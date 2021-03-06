package mybase

import (
	"reflect"
	"strings"
	"testing"
)

// simpleConfig returns a stub config based on a single map of key->value string
// pairs. All keys in the map will automatically be considered valid options.
func simpleConfig(values map[string]string) *Config {
	cmd := NewCommand("test", "1.0", "this is for testing", nil)
	for key := range values {
		cmd.AddOption(StringOption(key, 0, "", key))
	}
	cli := &CommandLine{
		Command: cmd,
	}
	return NewConfig(cli, SimpleSource(values))
}

// simpleCommand returns a standalone command for testing purposes
func simpleCommand() *Command {
	cmd := NewCommand("mycommand", "summary", "description", nil)
	cmd.AddOption(StringOption("visible", 0, "", "dummy description"))
	cmd.AddOption(StringOption("hidden", 0, "somedefault", "dummy description").Hidden())
	cmd.AddOption(StringOption("hasshort", 's', "", "dummy description"))
	cmd.AddOption(BoolOption("bool1", 'b', false, "dummy description"))
	cmd.AddOption(BoolOption("bool2", 'B', false, "dummy description"))
	cmd.AddOption(BoolOption("truthybool", 0, true, "dummy description"))
	cmd.AddArg("required", "", true)
	cmd.AddArg("optional", "hello", false)
	return cmd
}

// simpleCommandSuite returns a command suite for testing purposes
func simpleCommandSuite() *Command {
	suite := NewCommandSuite("mycommand", "summary", "description")
	suite.AddOption(StringOption("visible", 0, "", "dummy description"))
	suite.AddOption(StringOption("hidden", 0, "somedefault", "dummy description").Hidden())
	suite.AddOption(StringOption("hasshort", 's', "", "dummy description"))
	suite.AddOption(BoolOption("bool1", 'b', false, "dummy description"))
	suite.AddOption(BoolOption("bool2", 'B', false, "dummy description"))
	suite.AddOption(BoolOption("truthybool", 0, true, "dummy description"))

	cmd1 := NewCommand("one", "summary", "description", nil)
	cmd1.AddOption(StringOption("visible", 0, "newdefault", "dummy description")) // changed default
	cmd1.AddOption(StringOption("hidden", 0, "somedefault", "dummy description")) // no longer hidden
	cmd1.AddOption(StringOption("newopt", 'n', "", "dummy description"))
	suite.AddSubCommand(cmd1)

	cmd2 := NewCommand("two", "summary", "description", nil)
	cmd2.AddArg("optional", "hello", false)
	suite.AddSubCommand(cmd2)

	return suite
}

func TestOptionStatus(t *testing.T) {
	assertOptionStatus := func(cfg *Config, name string, expectChanged, expectSupplied, expectOnCLI bool) {
		t.Helper()
		if cfg.Changed(name) != expectChanged {
			t.Errorf("Expected cfg.Changed(%s)==%t, but instead returned %t", name, expectChanged, !expectChanged)
		}
		if cfg.Supplied(name) != expectSupplied {
			t.Errorf("Expected cfg.Supplied(%s)==%t, but instead returned %t", name, expectSupplied, !expectSupplied)
		}
		if cfg.OnCLI(name) != expectOnCLI {
			t.Errorf("Expected cfg.OnCLI(%s)==%t, but instead returned %t", name, expectOnCLI, !expectOnCLI)
		}
	}

	fakeFileOptions := SimpleSource(map[string]string{
		"hidden": "set off cli",
		"bool1":  "1",
	})
	cmd := simpleCommand()
	cfg := ParseFakeCLI(t, cmd, "mycommand -s 'hello world' --skip-truthybool --hidden=\"somedefault\" -B arg1", fakeFileOptions)
	assertOptionStatus(cfg, "visible", false, false, false)
	assertOptionStatus(cfg, "hidden", false, true, true)
	assertOptionStatus(cfg, "hasshort", true, true, true)
	assertOptionStatus(cfg, "bool1", true, true, false)
	assertOptionStatus(cfg, "bool2", true, true, true)
	assertOptionStatus(cfg, "truthybool", true, true, true)
	assertOptionStatus(cfg, "required", true, true, true)
	assertOptionStatus(cfg, "optional", false, false, false)

	// Among other things, confirm behavior of string option set to empty string
	cfg = ParseFakeCLI(t, cmd, "mycommand --skip-bool1 --hidden=\"\" --bool2 arg1", fakeFileOptions)
	assertOptionStatus(cfg, "bool1", false, true, true)
	assertOptionStatus(cfg, "hidden", true, true, true)
	assertOptionStatus(cfg, "bool2", true, true, true)
	if cfg.GetRaw("hidden") != "''" || cfg.Get("hidden") != "" {
		t.Errorf("Unexpected behavior of stringy options with empty value: GetRaw=%q, Get=%q", cfg.GetRaw("hidden"), cfg.Get("hidden"))
	}
}

func TestSuppliedWithValue(t *testing.T) {
	assertSuppliedWithValue := func(cfg *Config, name string, expected bool) {
		t.Helper()
		if cfg.SuppliedWithValue(name) != expected {
			t.Errorf("Unexpected return from SuppliedWithValue(%q): expected %t, found %t", name, expected, !expected)
		}
	}
	assertPanic := func(cfg *Config, name string) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("Expected SuppliedWithValue(%q) to panic, but it did not", name)
			}
		}()
		cfg.SuppliedWithValue(name)
	}

	cmd := simpleCommand()
	cmd.AddOption(StringOption("optional1", 'y', "", "dummy description").ValueOptional())
	cmd.AddOption(StringOption("optional2", 'z', "default", "dummy description").ValueOptional())

	cfg := ParseFakeCLI(t, cmd, "mycommand -s 'hello world' --skip-truthybool arg1")
	assertPanic(cfg, "doesntexist") // panics if option does not exist
	assertPanic(cfg, "truthybool")  // panics if option isn't string typed
	assertPanic(cfg, "hasshort")    // panics if option value isn't optional
	assertSuppliedWithValue(cfg, "optional1", false)
	assertSuppliedWithValue(cfg, "optional2", false)

	cfg = ParseFakeCLI(t, cmd, "mycommand -y -z arg1")
	assertSuppliedWithValue(cfg, "optional1", false)
	assertSuppliedWithValue(cfg, "optional2", false)

	cfg = ParseFakeCLI(t, cmd, "mycommand -yhello --optional2 arg1")
	assertSuppliedWithValue(cfg, "optional1", true)
	assertSuppliedWithValue(cfg, "optional2", false)

	cfg = ParseFakeCLI(t, cmd, "mycommand --optional2= --optional1='' arg1")
	assertSuppliedWithValue(cfg, "optional1", true)
	assertSuppliedWithValue(cfg, "optional2", true)
}

func TestGetRaw(t *testing.T) {
	optionValues := map[string]string{
		"basic":     "foo",
		"nothing":   "",
		"single":    "'quoted'",
		"double":    `"quoted"`,
		"backtick":  "`quoted`",
		"middle":    "something 'something' something",
		"beginning": `"something" something`,
		"end":       "something `something`",
	}
	cfg := simpleConfig(optionValues)

	for name, expected := range optionValues {
		if found := cfg.GetRaw(name); found != expected {
			t.Errorf("Expected GetRaw(%s) to be %s, instead found %s", name, expected, found)
		}
	}
}

func TestGet(t *testing.T) {
	assertBasicGet := func(name, value string) {
		optionValues := map[string]string{
			name: value,
		}
		cfg := simpleConfig(optionValues)
		if actual := cfg.Get(name); actual != value {
			t.Errorf("Expected Get(%s) to return %s, instead found %s", name, value, actual)
		}
	}
	assertQuotedGet := func(name, value, expected string) {
		optionValues := map[string]string{
			name: value,
		}
		cfg := simpleConfig(optionValues)
		if actual := cfg.Get(name); actual != expected {
			t.Errorf("Expected Get(%s) to return %s, instead found %s", name, expected, actual)
		}
	}

	basicValues := map[string]string{
		"basic":      "foo",
		"nothing":    "",
		"uni-start":  "☃snowperson",
		"uni-end":    "snowperson☃",
		"uni-both":   "☃snowperson☃",
		"middle":     "something 'something' something",
		"beginning":  `"something" something`,
		"end":        "something `something`",
		"no-escape1": `something\'s still backslashed`,
		"no-escape2": `'even this\'s still backslashed', they said`,
	}
	for name, value := range basicValues {
		assertBasicGet(name, value)
	}

	quotedValues := [][3]string{
		{"single", "'quoted'", "quoted"},
		{"double", `"quoted"`, "quoted"},
		{"empty", "''", ""},
		{"backtick", "`quoted`", "quoted"},
		{"uni-middle", `"yay ☃ snowpeople"`, `yay ☃ snowpeople`},
		{"esc-quote", `'something\'s escaped'`, `something's escaped`},
		{"esc-esc", `"c:\\tacotown"`, `c:\tacotown`},
		{"esc-rando", `'why\ whatevs'`, `why whatevs`},
		{"esc-uni", `'escaped snowpeople \☃ oh noes'`, `escaped snowpeople ☃ oh noes`},
	}
	for _, tuple := range quotedValues {
		assertQuotedGet(tuple[0], tuple[1], tuple[2])
	}
}

func TestGetSlice(t *testing.T) {
	assertGetSlice := func(optionValue string, delimiter rune, unwrapFull bool, expected ...string) {
		if expected == nil {
			expected = make([]string, 0)
		}
		cfg := simpleConfig(map[string]string{"option-name": optionValue})
		if actual := cfg.GetSlice("option-name", delimiter, unwrapFull); !reflect.DeepEqual(actual, expected) {
			t.Errorf("Expected GetSlice(\"...\", '%c', %t) on %#v to return %#v, instead found %#v", delimiter, unwrapFull, optionValue, expected, actual)
		}
	}

	assertGetSlice("hello", ',', false, "hello")
	assertGetSlice(`hello\`, ',', false, `hello\`)
	assertGetSlice("hello, world", ',', false, "hello", "world")
	assertGetSlice(`outside,"inside, ok?",   also outside`, ',', false, "outside", "inside, ok?", "also outside")
	assertGetSlice(`escaped\,delimiter doesn\'t split, ok?`, ',', false, `escaped\,delimiter doesn\'t split`, "ok?")
	assertGetSlice(`quoted "mid, value" doesn\'t split, either, duh`, ',', false, `quoted "mid, value" doesn\'t split`, "either", "duh")
	assertGetSlice(`'escaping\'s ok to prevent early quote end', yay," ok "`, ',', false, "escaping's ok to prevent early quote end", "yay", "ok")
	assertGetSlice(" space   delimiter", ' ', false, "space", "delimiter")
	assertGetSlice(`'fully wrapped in single quotes, commas still split tho, "nested\'s ok"'`, ',', false, "fully wrapped in single quotes, commas still split tho, \"nested's ok\"")
	assertGetSlice(`'fully wrapped in single quotes, commas still split tho, "nested\'s ok"'`, ',', true, "fully wrapped in single quotes", "commas still split tho", "nested's ok")
	assertGetSlice(`"'quotes',get \"tricky\", right, 'especially \\\' nested'"`, ',', true, "quotes", `get "tricky"`, "right", "especially ' nested")
	assertGetSlice("", ',', false)
	assertGetSlice("   ", ',', false)
	assertGetSlice("   ", ' ', false)
	assertGetSlice("``", ',', true)
	assertGetSlice(" `  `  ", ',', true)
	assertGetSlice(" `  `  ", ' ', true)
}

func TestGetEnum(t *testing.T) {
	optionValues := map[string]string{
		"foo":   "bar",
		"caps":  "SHOUTING",
		"blank": "",
	}
	cfg := simpleConfig(optionValues)

	value, err := cfg.GetEnum("foo", "baw", "bar", "bat")
	if value != "bar" || err != nil {
		t.Errorf("Expected bar,nil; found %s,%s", value, err)
	}
	value, err = cfg.GetEnum("foo", "BAW", "BaR", "baT")
	if value != "BaR" || err != nil {
		t.Errorf("Expected BaR,nil; found %s,%s", value, err)
	}
	value, err = cfg.GetEnum("foo", "nope", "dope")
	if value != "" || err == nil {
		t.Errorf("Expected error, found %s,%s", value, err)
	}
	value, err = cfg.GetEnum("caps", "yelling", "shouting")
	if value != "shouting" || err != nil {
		t.Errorf("Expected shouting,nil; found %s,%s", value, err)
	}
	value, err = cfg.GetEnum("blank", "nonblank1", "nonblank2")
	if value != "" || err != nil {
		t.Errorf("Expected empty string to be allowed since it is the default value, but instead found %s,%s", value, err)
	}
}

func TestGetBytes(t *testing.T) {
	optionValues := map[string]string{
		"simple-ok":     "1234",
		"negative-fail": "-3",
		"float-fail":    "4.5",
		"kilo1-ok":      "123k",
		"kilo2-ok":      "234K",
		"megs1-ok":      "12M",
		"megs2-ok":      "440mB",
		"gigs-ok":       "4GB",
		"tera-fail":     "55t",
		"blank-ok":      "",
	}
	cfg := simpleConfig(optionValues)

	assertBytes := func(name string, expect uint64) {
		value, err := cfg.GetBytes(name)
		if err == nil && strings.HasSuffix(name, "_bad") {
			t.Errorf("Expected error for GetBytes(%s) but didn't find one", name)
		} else if err != nil && strings.HasSuffix(name, "-ok") {
			t.Errorf("Unexpected error for GetBytes(%s): %s", name, err)
		}
		if value != expect {
			t.Errorf("Expected GetBytes(%s) to return %d, instead found %d", name, expect, value)
		}
	}

	expected := map[string]uint64{
		"simple-ok":     1234,
		"negative-fail": 0,
		"float-fail":    0,
		"kilo1-ok":      123 * 1024,
		"kilo2-ok":      234 * 1024,
		"megs1-ok":      12 * 1024 * 1024,
		"megs2-ok":      440 * 1024 * 1024,
		"gigs-ok":       4 * 1024 * 1024 * 1024,
		"tera-fail":     0,
		"blank-ok":      0,
	}
	for name, expect := range expected {
		assertBytes(name, expect)
	}
}

func TestGetRegexp(t *testing.T) {
	optionValues := map[string]string{
		"valid":   "^test",
		"invalid": "+++",
		"blank":   "",
	}
	cfg := simpleConfig(optionValues)

	re, err := cfg.GetRegexp("valid")
	if err != nil {
		t.Errorf("Unexpected error for GetRegexp(\"valid\"): %s", err)
	}
	if re == nil || !re.MatchString("testing") {
		t.Error("Regexp returned by GetRegexp(\"valid\") not working as expected")
	}

	re, err = cfg.GetRegexp("invalid")
	if re != nil || err == nil {
		t.Errorf("Expected invalid regexp to return nil and err, instead returned %v, %v", re, err)
	}

	re, err = cfg.GetRegexp("blank")
	if re != nil || err != nil {
		t.Errorf("Expected blank regexp to return nil, nil; instead returned %v, %v", re, err)
	}
}
