package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestNoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"kjell"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "Usage:") {
		t.Errorf("expected usage on stderr, got: %s", stderr.String())
	}
}

func TestCheckSafe(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"kjell", "check", "ls"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "SAFE") {
		t.Errorf("expected SAFE on stdout, got: %q", stdout.String())
	}
}

func TestCheckWrite(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"kjell", "check", "rm file"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "WRITE") {
		t.Errorf("expected WRITE on stdout, got: %q", stdout.String())
	}
}

func TestCheckJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"kjell", "check", "--json", "ls"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", code, stderr.String())
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &result); err != nil {
		t.Fatalf("invalid JSON output: %v; got: %q", err, stdout.String())
	}
	if result["classification"] != "safe" {
		t.Errorf("expected classification=safe, got %v", result["classification"])
	}
}

func TestCheckNoCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"kjell", "check"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "no command to classify") {
		t.Errorf("expected error about no command on stderr, got: %q", stderr.String())
	}
}

func TestCheckUnknownFormat(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"kjell", "check", "--format", "unknown-format", "ls"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "unknown format") {
		t.Errorf("expected 'unknown format' on stderr, got: %q", stderr.String())
	}
}

func TestDBStats(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"kjell", "db", "stats"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Commands in database:") {
		t.Errorf("expected command count on stdout, got: %q", stdout.String())
	}
}

func TestDBLookup(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"kjell", "db", "lookup", "grep"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Command: grep") {
		t.Errorf("expected 'Command: grep' on stdout, got: %q", stdout.String())
	}
}

func TestDBLookupNonexistent(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"kjell", "db", "lookup", "nonexistent"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(stdout.String(), "not in database") {
		t.Errorf("expected 'not in database' on stdout, got: %q", stdout.String())
	}
}

func TestDBNoSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"kjell", "db"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "usage:") {
		t.Errorf("expected usage on stderr, got: %q", stderr.String())
	}
}

func TestUnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"kjell", "foobar"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "unknown command: foobar") {
		t.Errorf("expected 'unknown command' on stderr, got: %q", stderr.String())
	}
}

func TestLogFlag(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	code := run([]string{"kjell", "--log", "check", "ls"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "SAFE") {
		t.Errorf("expected SAFE on stdout, got: %q", stdout.String())
	}
}

func TestLogFlagDebug(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	code := run([]string{"kjell", "--log", "debug", "check", "ls"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "SAFE") {
		t.Errorf("expected SAFE on stdout, got: %q", stdout.String())
	}
}

func TestLogFlagOnly(t *testing.T) {
	// --log as the only arg after program name → args becomes empty after removal
	var stdout, stderr bytes.Buffer
	code := run([]string{"kjell", "--log"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
}

func TestDBLookupRecursiveCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"kjell", "db", "lookup", "sudo"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Recursive: yes") {
		t.Errorf("expected 'Recursive: yes' on stdout, got: %q", stdout.String())
	}
}

func TestDBLookupWithSubcommands(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"kjell", "db", "lookup", "git"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Subcommands:") {
		t.Errorf("expected 'Subcommands:' on stdout, got: %q", out)
	}
}

func TestDBLookupWithFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"kjell", "db", "lookup", "sed"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Flags:") {
		t.Errorf("expected 'Flags:' on stdout, got: %q", out)
	}
	if !strings.Contains(out, "write") {
		t.Errorf("expected flag effect on stdout, got: %q", out)
	}
}

func TestDBLookupMissingArg(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"kjell", "db", "lookup"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
}

func TestDBValidate(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"kjell", "db", "validate"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Database valid:") {
		t.Errorf("expected 'Database valid:' on stdout, got: %q", stdout.String())
	}
}

func TestDBUnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"kjell", "db", "foobar"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "unknown db subcommand") {
		t.Errorf("expected error on stderr, got: %q", stderr.String())
	}
}

func TestCheckFormatMissingValue(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"kjell", "check", "--format"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "--format requires a value") {
		t.Errorf("expected format error on stderr, got: %q", stderr.String())
	}
}

func TestCheckClaudeCodeFormatWriteCommand(t *testing.T) {
	input := `{"tool_name":"Bash","tool_input":{"command":"rm -rf /"}}`
	stdin := strings.NewReader(input)
	var stdout, stderr bytes.Buffer
	code := run([]string{"kjell", "check", "--format", "claude-code"}, stdin, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", code, stderr.String())
	}
	// write commands produce empty output (passthrough)
	if strings.TrimSpace(stdout.String()) != "" {
		t.Errorf("expected empty output for write command in claude-code format, got: %q", stdout.String())
	}
}

func TestLogFlagSetupLogMkdirError(t *testing.T) {
	// Point HOME to a file (not a directory) so MkdirAll for ~/.kjell fails
	tmp := t.TempDir()
	fakeHome := tmp + "/fakehome"
	if err := os.WriteFile(fakeHome, []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", fakeHome)
	var stdout, stderr bytes.Buffer
	// setupLog will warn but still work (returns nil logger)
	code := run([]string{"kjell", "--log", "check", "ls"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", code, stderr.String())
	}
}

func TestLogFlagSetupLogOpenFileError(t *testing.T) {
	// Create ~/.kjell as a directory but make ~/.kjell/log a directory so OpenFile fails
	tmp := t.TempDir()
	kjellDir := tmp + "/.kjell"
	if err := os.MkdirAll(kjellDir+"/log", 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", tmp)
	var stdout, stderr bytes.Buffer
	code := run([]string{"kjell", "--log", "check", "ls"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "warning:") {
		t.Errorf("expected warning on stderr, got: %q", stderr.String())
	}
}

func TestCheckClaudeCodeFormatBadInput(t *testing.T) {
	stdin := strings.NewReader("not json")
	var stdout, stderr bytes.Buffer
	code := run([]string{"kjell", "check", "--format", "claude-code"}, stdin, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
}

func TestCheckClaudeCodeFormat(t *testing.T) {
	input := `{"tool_name":"Bash","tool_input":{"command":"ls -la"}}`
	stdin := strings.NewReader(input)
	var stdout, stderr bytes.Buffer
	code := run([]string{"kjell", "check", "--format", "claude-code"}, stdin, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", code, stderr.String())
	}
	// ls -la is safe, so claude-code format should output an allow response
	output := strings.TrimSpace(stdout.String())
	if output == "" {
		// safe commands produce allow JSON output
		t.Fatal("expected non-empty output for safe command in claude-code format")
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("invalid JSON output: %v; got: %q", err, output)
	}
}
