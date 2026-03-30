package database

import (
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
		t.Errorf("expected default 'read', got %q", def.Default)
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
		t.Errorf("expected subcommand 'log' with default 'read'")
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
		t.Errorf("expected GET -> read")
	}
}
