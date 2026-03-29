package parser

import (
	"testing"
)

func TestParseSimpleCommand(t *testing.T) {
	expr, err := Parse(`grep -r TODO src/`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(expr.Pipelines) != 1 {
		t.Fatalf("expected 1 pipeline, got %d", len(expr.Pipelines))
	}
	cmd := expr.Pipelines[0].Commands[0]
	if cmd.Command != "grep" {
		t.Errorf("expected command 'grep', got %q", cmd.Command)
	}
	if len(cmd.Args) != 3 {
		t.Errorf("expected 3 args, got %d: %v", len(cmd.Args), cmd.Args)
	}
}

func TestParseFlags(t *testing.T) {
	expr, err := Parse(`grep -r --include="*.go" TODO`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cmd := expr.Pipelines[0].Commands[0]
	foundR := false
	for _, f := range cmd.Flags {
		if f.Name == "-r" {
			foundR = true
		}
	}
	if !foundR {
		t.Error("expected to find -r flag")
	}
}

func TestParsePipeline(t *testing.T) {
	expr, err := Parse(`cat file | grep error | sort`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(expr.Pipelines) != 1 {
		t.Fatalf("expected 1 pipeline, got %d", len(expr.Pipelines))
	}
	if len(expr.Pipelines[0].Commands) != 3 {
		t.Fatalf("expected 3 commands in pipeline, got %d", len(expr.Pipelines[0].Commands))
	}

	names := []string{"cat", "grep", "sort"}
	for i, name := range names {
		if expr.Pipelines[0].Commands[i].Command != name {
			t.Errorf("command %d: expected %q, got %q", i, name, expr.Pipelines[0].Commands[i].Command)
		}
	}
}

func TestParseCompound(t *testing.T) {
	expr, err := Parse(`cmd1 && cmd2 ; cmd3`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(expr.Pipelines) < 3 {
		t.Fatalf("expected 3 pipelines, got %d", len(expr.Pipelines))
	}
	if len(expr.Operators) < 2 {
		t.Fatalf("expected 2 operators, got %d: %v", len(expr.Operators), expr.Operators)
	}
}

func TestParseAndOr(t *testing.T) {
	expr, err := Parse(`ls && pwd`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(expr.Pipelines) != 2 {
		t.Fatalf("expected 2 pipelines, got %d", len(expr.Pipelines))
	}
	if len(expr.Operators) != 1 || expr.Operators[0] != "&&" {
		t.Errorf("expected operator '&&', got %v", expr.Operators)
	}
	if expr.Pipelines[0].Commands[0].Command != "ls" {
		t.Errorf("expected 'ls', got %q", expr.Pipelines[0].Commands[0].Command)
	}
	if expr.Pipelines[1].Commands[0].Command != "pwd" {
		t.Errorf("expected 'pwd', got %q", expr.Pipelines[1].Commands[0].Command)
	}
}

func TestParseRedirectOut(t *testing.T) {
	expr, err := Parse(`echo hello > file.txt`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(expr.Redirects) == 0 {
		t.Fatal("expected at least one redirect")
	}
	found := false
	for _, r := range expr.Redirects {
		if r.Type == ">" && r.Target == "file.txt" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected redirect > file.txt, got %+v", expr.Redirects)
	}
}

func TestParseRedirectAppend(t *testing.T) {
	expr, err := Parse(`echo msg >> log.txt`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(expr.Redirects) == 0 {
		t.Fatal("expected at least one redirect")
	}
	if expr.Redirects[0].Type != ">>" {
		t.Errorf("expected '>>', got %q", expr.Redirects[0].Type)
	}
}

func TestParseCommandSubstitution(t *testing.T) {
	expr, err := Parse(`echo $(ls)`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(expr.Subshells) == 0 {
		t.Fatal("expected command substitution to be detected")
	}
}

func TestParseStringArgs(t *testing.T) {
	expr, err := Parse(`sh -c 'grep foo bar'`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cmd := expr.Pipelines[0].Commands[0]
	if cmd.Command != "sh" {
		t.Errorf("expected 'sh', got %q", cmd.Command)
	}
	// The string argument should be preserved
	found := false
	for _, arg := range cmd.Args {
		if arg == "grep foo bar" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected to find 'grep foo bar' in args, got %v", cmd.Args)
	}
}

func TestParseEmpty(t *testing.T) {
	_, err := Parse("")
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestParseWhitespace(t *testing.T) {
	_, err := Parse("   ")
	if err == nil {
		t.Error("expected error for whitespace-only input")
	}
}

func TestParseGitSubcommand(t *testing.T) {
	expr, err := Parse(`git log --oneline`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cmd := expr.Pipelines[0].Commands[0]
	if cmd.Command != "git" {
		t.Errorf("expected 'git', got %q", cmd.Command)
	}
	if len(cmd.Args) < 1 || cmd.Args[0] != "log" {
		t.Errorf("expected first arg 'log', got %v", cmd.Args)
	}
}

func TestParseSudoCommand(t *testing.T) {
	expr, err := Parse(`sudo rm -rf /`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cmd := expr.Pipelines[0].Commands[0]
	if cmd.Command != "sudo" {
		t.Errorf("expected 'sudo', got %q", cmd.Command)
	}
	if len(cmd.Args) < 1 || cmd.Args[0] != "rm" {
		t.Errorf("expected first arg 'rm', got %v", cmd.Args)
	}
}

func TestParseEnvVarPrefix(t *testing.T) {
	expr, err := Parse(`FOO=bar grep TODO`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cmd := expr.Pipelines[0].Commands[0]
	if cmd.Command != "grep" {
		t.Errorf("expected 'grep' (after assignment), got %q", cmd.Command)
	}
	if len(cmd.Assignments) == 0 {
		t.Error("expected assignments to be captured")
	}
}

func TestParseRedirectStderr(t *testing.T) {
	expr, err := Parse(`cmd 2> /dev/null`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(expr.Redirects) == 0 {
		t.Fatal("expected redirect")
	}
	if expr.Redirects[0].Type != "2>" {
		t.Errorf("expected '2>', got %q", expr.Redirects[0].Type)
	}
}

func TestParseFindExec(t *testing.T) {
	expr, err := Parse(`find . -exec cat {} \;`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cmd := expr.Pipelines[0].Commands[0]
	if cmd.Command != "find" {
		t.Errorf("expected 'find', got %q", cmd.Command)
	}
	// -exec should be in the flags
	foundExec := false
	for _, f := range cmd.Flags {
		if f.Name == "-exec" {
			foundExec = true
		}
	}
	if !foundExec {
		t.Errorf("expected to find -exec flag, got flags: %+v", cmd.Flags)
	}
}

func TestParseOrOperator(t *testing.T) {
	expr, err := Parse(`cmd1 || cmd2`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(expr.Pipelines) != 2 {
		t.Fatalf("expected 2 pipelines, got %d", len(expr.Pipelines))
	}
	if len(expr.Operators) != 1 || expr.Operators[0] != "||" {
		t.Errorf("expected operator '||', got %v", expr.Operators)
	}
}

func TestParseDoubleDashSeparator(t *testing.T) {
	expr, err := Parse(`kubectl exec pod -- ls -la`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cmd := expr.Pipelines[0].Commands[0]
	if cmd.Command != "kubectl" {
		t.Errorf("expected 'kubectl', got %q", cmd.Command)
	}
	// Args should include everything: exec, pod, --, ls, -la
	foundDash := false
	for _, arg := range cmd.Args {
		if arg == "--" {
			foundDash = true
		}
	}
	if !foundDash {
		t.Errorf("expected '--' in args, got %v", cmd.Args)
	}
}
