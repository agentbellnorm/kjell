package database

import (
	"embed"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"testing/fstest"
)

func testFS(files map[string]string) fstest.MapFS {
	m := fstest.MapFS{}
	for name, content := range files {
		m[name] = &fstest.MapFile{Data: []byte(content)}
	}
	return m
}

func TestLoadMinimalCommand(t *testing.T) {
	fs := testFS(map[string]string{
		"grep.toml": `command = "grep"
default = "safe"`,
	})

	db, err := LoadFromFS(fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	def := db.Lookup("grep")
	if def == nil {
		t.Fatal("expected to find grep, got nil")
	}
	if def.Command != "grep" {
		t.Errorf("expected command 'grep', got %q", def.Command)
	}
	if def.Default != Safe {
		t.Errorf("expected default 'safe', got %q", def.Default)
	}
}

func TestLookupNotFound(t *testing.T) {
	fs := testFS(map[string]string{
		"grep.toml": `command = "grep"
default = "safe"`,
	})

	db, err := LoadFromFS(fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if def := db.Lookup("nonexistent"); def != nil {
		t.Errorf("expected nil for nonexistent command, got %+v", def)
	}
}

func TestLoadCommandWithFlags(t *testing.T) {
	fs := testFS(map[string]string{
		"sed.toml": `command = "sed"
default = "safe"

[[flags]]
flag = ["-i", "--in-place"]
effect = "write"
reason = "Edits files in place"`,
	})

	db, err := LoadFromFS(fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	def := db.Lookup("sed")
	if def == nil {
		t.Fatal("expected to find sed")
	}
	if len(def.Flags) != 1 {
		t.Fatalf("expected 1 flag, got %d", len(def.Flags))
	}
	flag := def.Flags[0]
	if flag.Effect != "write" {
		t.Errorf("expected effect 'write', got %q", flag.Effect)
	}
	if flag.Reason != "Edits files in place" {
		t.Errorf("unexpected reason: %q", flag.Reason)
	}
	if len(flag.Flag) != 2 || flag.Flag[0] != "-i" || flag.Flag[1] != "--in-place" {
		t.Errorf("unexpected flag names: %v", flag.Flag)
	}
}

func TestLoadCommandWithSubcommands(t *testing.T) {
	fs := testFS(map[string]string{
		"git.toml": `command = "git"
default = "unknown"

[subcommands.log]
default = "safe"

[subcommands.push]
default = "write"`,
	})

	db, err := LoadFromFS(fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	def := db.Lookup("git")
	if def == nil {
		t.Fatal("expected to find git")
	}
	if len(def.Subcommands) != 2 {
		t.Fatalf("expected 2 subcommands, got %d", len(def.Subcommands))
	}
	if sub, ok := def.Subcommands["log"]; !ok || sub.Default != Safe {
		t.Errorf("expected subcommand 'log' with default 'safe'")
	}
	if sub, ok := def.Subcommands["push"]; !ok || sub.Default != Write {
		t.Errorf("expected subcommand 'push' with default 'write'")
	}
}

func TestLoadRecursiveCommand(t *testing.T) {
	fs := testFS(map[string]string{
		"sudo.toml": `command = "sudo"
default = "unknown"
recursive = true
inner_command_position = 1`,
	})

	db, err := LoadFromFS(fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	def := db.Lookup("sudo")
	if def == nil {
		t.Fatal("expected to find sudo")
	}
	if !def.Recursive {
		t.Error("expected recursive = true")
	}
}

func TestLoadRecursiveFlag(t *testing.T) {
	fs := testFS(map[string]string{
		"find.toml": `command = "find"
default = "safe"

[[flags]]
flag = ["-exec"]
effect = "recursive"
inner_command_terminators = [";", "+"]

[[flags]]
flag = ["-delete"]
effect = "write"`,
	})

	db, err := LoadFromFS(fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	def := db.Lookup("find")
	if def == nil {
		t.Fatal("expected to find find")
	}
	if len(def.Flags) != 2 {
		t.Fatalf("expected 2 flags, got %d", len(def.Flags))
	}
	if def.Flags[0].Effect != "recursive" {
		t.Errorf("expected first flag effect 'recursive', got %q", def.Flags[0].Effect)
	}
	if len(def.Flags[0].InnerCommandTerminator) != 2 {
		t.Errorf("expected 2 terminators, got %d", len(def.Flags[0].InnerCommandTerminator))
	}
}

func TestValidationMissingCommand(t *testing.T) {
	fs := testFS(map[string]string{
		"bad.toml": `default = "safe"`,
	})

	_, err := LoadFromFS(fs)
	if err == nil {
		t.Fatal("expected error for missing command field")
	}
}

func TestValidationInvalidClassification(t *testing.T) {
	fs := testFS(map[string]string{
		"bad.toml": `command = "foo"
default = "dangerous"`,
	})

	_, err := LoadFromFS(fs)
	if err == nil {
		t.Fatal("expected error for invalid classification")
	}
}

func TestValidationInvalidFlagEffect(t *testing.T) {
	fs := testFS(map[string]string{
		"bad.toml": `command = "foo"
default = "safe"

[[flags]]
flag = ["-x"]
effect = "destroy"`,
	})

	_, err := LoadFromFS(fs)
	if err == nil {
		t.Fatal("expected error for invalid flag effect")
	}
}

func TestValidationMissingSubcommandDefault(t *testing.T) {
	fs := testFS(map[string]string{
		"bad.toml": `command = "foo"
default = "unknown"

[subcommands.bar]
`,
	})

	_, err := LoadFromFS(fs)
	if err == nil {
		t.Fatal("expected error for subcommand missing default")
	}
}

func TestValidationFlagMissingNames(t *testing.T) {
	fs := testFS(map[string]string{
		"bad.toml": `command = "foo"
default = "safe"

[[flags]]
flag = []
effect = "write"`,
	})

	_, err := LoadFromFS(fs)
	if err == nil {
		t.Fatal("expected error for flag with no names")
	}
}

func TestLookupStripsPath(t *testing.T) {
	fs := testFS(map[string]string{
		"grep.toml": `command = "grep"
default = "safe"`,
	})

	db, err := LoadFromFS(fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if def := db.Lookup("/usr/bin/grep"); def == nil {
		t.Error("expected to find grep via /usr/bin/grep")
	}
}

func TestMultipleFiles(t *testing.T) {
	fs := testFS(map[string]string{
		"grep.toml": `command = "grep"
default = "safe"`,
		"rm.toml": `command = "rm"
default = "write"`,
		"readme.md": `this is not a toml file`,
	})

	db, err := LoadFromFS(fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if db.Commands() != 2 {
		t.Errorf("expected 2 commands, got %d", db.Commands())
	}
	if def := db.Lookup("grep"); def == nil {
		t.Error("expected to find grep")
	}
	if def := db.Lookup("rm"); def == nil {
		t.Error("expected to find rm")
	}
}

func TestValueDependentFlags(t *testing.T) {
	fs := testFS(map[string]string{
		"curl.toml": `command = "curl"
default = "safe"

[[flags]]
flag = ["-X", "--request"]
effect = "unknown"
reason = "Depends on HTTP method"

[flags.values]
GET = "safe"
HEAD = "safe"
POST = "write"
PUT = "write"
DELETE = "write"
PATCH = "write"`,
	})

	db, err := LoadFromFS(fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	def := db.Lookup("curl")
	if def == nil {
		t.Fatal("expected to find curl")
	}
	if len(def.Flags) != 1 {
		t.Fatalf("expected 1 flag, got %d", len(def.Flags))
	}
	if def.Flags[0].Values["POST"] != "write" {
		t.Errorf("expected POST -> write")
	}
	if def.Flags[0].Values["GET"] != "safe" {
		t.Errorf("expected GET -> safe")
	}
}

func TestMergeOverridesDefault(t *testing.T) {
	base := testFS(map[string]string{
		"grep.toml": `command = "grep"
default = "safe"`,
		"rm.toml": `command = "rm"
default = "write"`,
	})

	override := testFS(map[string]string{
		"grep.toml": `command = "grep"
default = "write"`,
	})

	db, err := LoadFromFS(base)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	overrideDB, err := LoadFromFS(override)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	db.Merge(overrideDB)

	def := db.Lookup("grep")
	if def == nil {
		t.Fatal("expected to find grep")
	}
	if def.Default != Write {
		t.Errorf("expected grep default 'write' after merge, got %q", def.Default)
	}

	// rm should remain unchanged
	def = db.Lookup("rm")
	if def == nil {
		t.Fatal("expected to find rm")
	}
	if def.Default != Write {
		t.Errorf("expected rm default 'write', got %q", def.Default)
	}
}

func TestMergeAddsNewCommand(t *testing.T) {
	base := testFS(map[string]string{
		"grep.toml": `command = "grep"
default = "safe"`,
	})

	override := testFS(map[string]string{
		"mycmd.toml": `command = "mycmd"
default = "write"`,
	})

	db, err := LoadFromFS(base)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	overrideDB, err := LoadFromFS(override)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	db.Merge(overrideDB)

	if db.Commands() != 2 {
		t.Errorf("expected 2 commands, got %d", db.Commands())
	}
	if def := db.Lookup("mycmd"); def == nil {
		t.Error("expected to find mycmd after merge")
	}
}

func TestMergeSubcommandGranular(t *testing.T) {
	base := testFS(map[string]string{
		"git.toml": `command = "git"
default = "unknown"

[subcommands.log]
default = "safe"

[subcommands.push]
default = "write"

[subcommands.status]
default = "safe"`,
	})

	// Override only changes push, leaves log and status alone
	override := testFS(map[string]string{
		"git.toml": `command = "git"

[subcommands.push]
default = "safe"`,
	})

	db, err := LoadFromFS(base)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	overrideDB, err := loadFromFS(override, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	db.Merge(overrideDB)

	def := db.Lookup("git")
	if def == nil {
		t.Fatal("expected to find git")
	}
	// default preserved
	if def.Default != Unknown {
		t.Errorf("expected default 'unknown', got %q", def.Default)
	}
	// push overridden
	if sub, ok := def.Subcommands["push"]; !ok || sub.Default != Safe {
		t.Errorf("expected push to be overridden to 'safe'")
	}
	// log preserved
	if sub, ok := def.Subcommands["log"]; !ok || sub.Default != Safe {
		t.Errorf("expected log to remain 'safe'")
	}
	// status preserved
	if sub, ok := def.Subcommands["status"]; !ok || sub.Default != Safe {
		t.Errorf("expected status to remain 'safe'")
	}
}

func TestMergeFlagGranular(t *testing.T) {
	base := testFS(map[string]string{
		"curl.toml": `command = "curl"
default = "safe"

[[flags]]
flag = ["-X", "--request"]
effect = "unknown"

[[flags]]
flag = ["-o", "--output"]
effect = "write"`,
	})

	// Override -X to be safe, leave -o alone
	override := testFS(map[string]string{
		"curl.toml": `command = "curl"

[[flags]]
flag = ["-X"]
effect = "safe"`,
	})

	db, err := LoadFromFS(base)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	overrideDB, err := loadFromFS(override, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	db.Merge(overrideDB)

	def := db.Lookup("curl")
	if def == nil {
		t.Fatal("expected to find curl")
	}
	if len(def.Flags) != 2 {
		t.Fatalf("expected 2 flags, got %d", len(def.Flags))
	}
	// -X overridden
	if def.Flags[0].Effect != "safe" {
		t.Errorf("expected -X effect 'safe', got %q", def.Flags[0].Effect)
	}
	// -o preserved
	if def.Flags[1].Effect != "write" {
		t.Errorf("expected -o effect 'write', got %q", def.Flags[1].Effect)
	}
}

func TestMergeAddsNewFlag(t *testing.T) {
	base := testFS(map[string]string{
		"foo.toml": `command = "foo"
default = "safe"

[[flags]]
flag = ["-v"]
effect = "safe"`,
	})

	override := testFS(map[string]string{
		"foo.toml": `command = "foo"

[[flags]]
flag = ["-w"]
effect = "write"`,
	})

	db, err := LoadFromFS(base)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	overrideDB, err := loadFromFS(override, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	db.Merge(overrideDB)

	def := db.Lookup("foo")
	if def == nil {
		t.Fatal("expected to find foo")
	}
	if len(def.Flags) != 2 {
		t.Fatalf("expected 2 flags after merge, got %d", len(def.Flags))
	}
}

func TestLoadDirPartialValidation(t *testing.T) {
	dir := t.TempDir()
	// Only command + subcommand, no default — valid as override
	err := os.WriteFile(filepath.Join(dir, "git.toml"), []byte(`command = "git"

[subcommands.push]
default = "safe"
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	db, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	def := db.Lookup("git")
	if def == nil {
		t.Fatal("expected to find git")
	}
	if sub, ok := def.Subcommands["push"]; !ok || sub.Default != Safe {
		t.Error("expected push subcommand with default 'safe'")
	}
}

func TestLoadDir(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "mycmd.toml"), []byte(`command = "mycmd"
default = "safe"
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	db, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	def := db.Lookup("mycmd")
	if def == nil {
		t.Fatal("expected to find mycmd")
	}
	if def.Default != Safe {
		t.Errorf("expected default 'safe', got %q", def.Default)
	}
}

func TestCommandNames(t *testing.T) {
	fs := testFS(map[string]string{
		"grep.toml": `command = "grep"
default = "safe"`,
		"rm.toml": `command = "rm"
default = "write"`,
		"ls.toml": `command = "ls"
default = "safe"`,
	})

	db, err := LoadFromFS(fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	names := db.CommandNames()
	sort.Strings(names)
	expected := []string{"grep", "ls", "rm"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d names, got %d", len(expected), len(names))
	}
	for i, name := range expected {
		if names[i] != name {
			t.Errorf("expected name %q at index %d, got %q", name, i, names[i])
		}
	}
}

func TestLoadEmbeddedErrorPath(t *testing.T) {
	var empty embed.FS
	_, err := LoadEmbedded(empty, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent embedded dir")
	}
}

func TestMergeCommandDefAllFields(t *testing.T) {
	base := CommandDef{
		Command:  "mycmd",
		Default:  Safe,
		Reason:   "",
		Recursive: false,
		Separator: "",
		InnerCommandPosition: nil,
	}

	override := CommandDef{
		Command:              "mycmd",
		Default:              Write,
		Reason:               "overridden reason",
		Recursive:            true,
		Separator:            ";",
		InnerCommandPosition: 2,
	}

	result := mergeCommandDef(base, override)

	if result.Default != Write {
		t.Errorf("expected default 'write', got %q", result.Default)
	}
	if result.Reason != "overridden reason" {
		t.Errorf("expected reason 'overridden reason', got %q", result.Reason)
	}
	if !result.Recursive {
		t.Error("expected recursive to be true")
	}
	if result.Separator != ";" {
		t.Errorf("expected separator ';', got %q", result.Separator)
	}
	if result.InnerCommandPosition != 2 {
		t.Errorf("expected inner_command_position 2, got %v", result.InnerCommandPosition)
	}
}

func TestLoadFromFSInvalidTOML(t *testing.T) {
	fs := testFS(map[string]string{
		"bad.toml": `this is not valid toml [[[`,
	})

	_, err := LoadFromFS(fs)
	if err == nil {
		t.Fatal("expected error for invalid TOML content")
	}
}

func TestLoadFromFSSkipsDirectories(t *testing.T) {
	m := fstest.MapFS{
		"subdir/placeholder.toml": &fstest.MapFile{Data: []byte(`command = "x"
default = "safe"`)},
		"grep.toml": &fstest.MapFile{Data: []byte(`command = "grep"
default = "safe"`)},
	}

	db, err := LoadFromFS(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should only have loaded grep.toml, not the subdir
	if db.Commands() != 1 {
		t.Errorf("expected 1 command (directories skipped), got %d", db.Commands())
	}
	if def := db.Lookup("grep"); def == nil {
		t.Error("expected to find grep")
	}
}

func TestNormalizeCommandDefFloat64(t *testing.T) {
	def := CommandDef{
		Command:              "test",
		Default:              Safe,
		InnerCommandPosition: float64(3.0),
	}

	normalizeCommandDef(&def)

	pos, ok := def.InnerCommandPosition.(int)
	if !ok {
		t.Fatalf("expected InnerCommandPosition to be int after normalization, got %T", def.InnerCommandPosition)
	}
	if pos != 3 {
		t.Errorf("expected InnerCommandPosition 3, got %d", pos)
	}
}

func TestValidationInvalidSubcommandClassification(t *testing.T) {
	fs := testFS(map[string]string{
		"bad.toml": `command = "foo"
default = "unknown"

[subcommands.bar]
default = "dangerous"`,
	})

	_, err := LoadFromFS(fs)
	if err == nil {
		t.Fatal("expected error for invalid subcommand classification")
	}
}

func TestMergeCommandDefNilBaseSubcommands(t *testing.T) {
	base := CommandDef{
		Command: "mycmd",
		Default: Safe,
	}
	override := CommandDef{
		Command: "mycmd",
		Subcommands: map[string]CommandDef{
			"sub1": {Default: Write},
		},
	}

	result := mergeCommandDef(base, override)

	if result.Subcommands == nil {
		t.Fatal("expected subcommands to be initialized")
	}
	if sub, ok := result.Subcommands["sub1"]; !ok || sub.Default != Write {
		t.Error("expected sub1 with default 'write'")
	}
}

func TestLoadFromFSReadFileError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unreadable.toml")
	if err := os.WriteFile(path, []byte(`command = "x"`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		os.Chmod(path, 0o644)
	})

	_, err := loadFromFS(os.DirFS(dir), false)
	if err == nil {
		t.Fatal("expected error for unreadable file")
	}
}

func TestValidationNoDefaultNoSubcommands(t *testing.T) {
	fs := testFS(map[string]string{
		"bad.toml": `command = "foo"`,
	})

	_, err := LoadFromFS(fs)
	if err == nil {
		t.Fatal("expected error for command with no default and no subcommands")
	}
}

func TestValidationSubcommandsOnly(t *testing.T) {
	fs := testFS(map[string]string{
		"git.toml": `command = "git"

[subcommands.log]
default = "safe"`,
	})

	db, err := LoadFromFS(fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	def := db.Lookup("git")
	if def == nil {
		t.Fatal("expected to find git")
	}
	if def.Default != "" {
		t.Errorf("expected empty default, got %q", def.Default)
	}
	if sub, ok := def.Subcommands["log"]; !ok || sub.Default != Safe {
		t.Error("expected log subcommand with default 'safe'")
	}
}
