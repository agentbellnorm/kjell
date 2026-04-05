package parser

import (
	"strings"
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

// --- Coverage-expanding tests below ---

func TestParseErrorMethod(t *testing.T) {
	pe := &ParseError{Message: "something broke", Pos: 5}
	got := pe.Error()
	if got != "parse error: something broke" {
		t.Errorf("expected 'parse error: something broke', got %q", got)
	}
}

func TestParseForClause(t *testing.T) {
	expr, err := Parse(`for i in 1 2 3; do echo $i; done`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The body command (echo) should be extracted
	found := false
	for _, p := range expr.Pipelines {
		for _, c := range p.Commands {
			if c.Command == "echo" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected to find 'echo' command inside for loop body")
	}
}

func TestParseWhileClause(t *testing.T) {
	expr, err := Parse(`while true; do echo x; done`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, p := range expr.Pipelines {
		for _, c := range p.Commands {
			if c.Command == "echo" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected to find 'echo' command inside while loop body")
	}
}

func TestParseIfClause(t *testing.T) {
	expr, err := Parse(`if true; then echo yes; else echo no; fi`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Both then and else branches should produce pipelines
	echoCount := 0
	for _, p := range expr.Pipelines {
		for _, c := range p.Commands {
			if c.Command == "echo" {
				echoCount++
			}
		}
	}
	if echoCount < 2 {
		t.Errorf("expected at least 2 echo commands (then + else), got %d", echoCount)
	}
}

func TestParseBlock(t *testing.T) {
	expr, err := Parse(`{ echo a; echo b; }`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	echoCount := 0
	for _, p := range expr.Pipelines {
		for _, c := range p.Commands {
			if c.Command == "echo" {
				echoCount++
			}
		}
	}
	if echoCount != 2 {
		t.Errorf("expected 2 echo commands in block, got %d", echoCount)
	}
}

func TestParseCaseClause(t *testing.T) {
	// case statement triggers the default branch of walkStmt
	expr, err := Parse(`case $x in a) echo a;; esac`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(expr.Pipelines) == 0 {
		t.Error("expected at least one pipeline from case statement")
	}
}

func TestCollectPipeSideNonPipeBinaryCmd(t *testing.T) {
	// (cmd1 && cmd2) | cmd3 — the left side of the pipe is a subshell
	// containing a BinaryCmd(&&), which triggers the default case in collectPipeSide
	expr, err := Parse(`(cmd1 && cmd2) | cmd3`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(expr.Pipelines) == 0 {
		t.Fatal("expected at least one pipeline")
	}
}

func TestCollectSubstitutionsDblQuoted(t *testing.T) {
	expr, err := Parse(`echo "result: $(ls)"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(expr.Subshells) == 0 {
		t.Fatal("expected command substitution inside double quotes to be detected")
	}
	if expr.Subshells[0] != "ls" {
		t.Errorf("expected subshell 'ls', got %q", expr.Subshells[0])
	}
}

func TestCollectSubstitutionsBacktick(t *testing.T) {
	expr, err := Parse("echo `ls`")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(expr.Subshells) == 0 {
		t.Fatal("expected backtick command substitution to be detected")
	}
	if expr.Subshells[0] != "ls" {
		t.Errorf("expected subshell 'ls', got %q", expr.Subshells[0])
	}
}

func TestExtractFlagsPublic(t *testing.T) {
	flags := ExtractFlags([]string{"-v", "--output", "file.txt", "--name=foo", "--", "-ignored"})

	if len(flags) != 3 {
		t.Fatalf("expected 3 flags, got %d: %+v", len(flags), flags)
	}
	if flags[0].Name != "-v" {
		t.Errorf("expected flag '-v', got %q", flags[0].Name)
	}
	if flags[1].Name != "--output" || flags[1].Value != "file.txt" {
		t.Errorf("expected flag '--output' with value 'file.txt', got %+v", flags[1])
	}
	if flags[2].Name != "--name" || flags[2].Value != "foo" {
		t.Errorf("expected flag '--name' with value 'foo', got %+v", flags[2])
	}
}

func TestParseRedirectDplOut(t *testing.T) {
	expr, err := Parse(`cmd >&2`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(expr.Redirects) == 0 {
		t.Fatal("expected redirect")
	}
	if expr.Redirects[0].Type != ">&" {
		t.Errorf("expected '>&', got %q", expr.Redirects[0].Type)
	}
}

func TestParseRedirectRdrAll(t *testing.T) {
	expr, err := Parse(`cmd &> file.txt`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(expr.Redirects) == 0 {
		t.Fatal("expected redirect")
	}
	if expr.Redirects[0].Type != "&>" {
		t.Errorf("expected '&>', got %q", expr.Redirects[0].Type)
	}
	if expr.Redirects[0].Target != "file.txt" {
		t.Errorf("expected target 'file.txt', got %q", expr.Redirects[0].Target)
	}
}

func TestParseRedirectStderrAppend(t *testing.T) {
	expr, err := Parse(`cmd 2>> file.txt`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(expr.Redirects) == 0 {
		t.Fatal("expected redirect")
	}
	if expr.Redirects[0].Type != "2>>" {
		t.Errorf("expected '2>>', got %q", expr.Redirects[0].Type)
	}
}

func TestParseRedirectDefaultOp(t *testing.T) {
	// Heredoc (<<) triggers the default case in extractRedirect
	expr, err := Parse(`cat <<EOF
hello
EOF`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(expr.Redirects) == 0 {
		t.Fatal("expected redirect for heredoc")
	}
	if !strings.HasPrefix(expr.Redirects[0].Type, "redirect_op_") {
		t.Errorf("expected default redirect type 'redirect_op_*', got %q", expr.Redirects[0].Type)
	}
}

func TestParseRedirectInput(t *testing.T) {
	expr, err := Parse(`cmd < input.txt`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(expr.Redirects) == 0 {
		t.Fatal("expected redirect")
	}
	if expr.Redirects[0].Type != "<" {
		t.Errorf("expected '<', got %q", expr.Redirects[0].Type)
	}
}

func TestParseNegatedPipeline(t *testing.T) {
	expr, err := Parse(`! cmd1 | cmd2`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(expr.Pipelines) == 0 {
		t.Fatal("expected at least one pipeline")
	}
	if !expr.Pipelines[0].Negated {
		t.Error("expected pipeline to be negated")
	}
}

func TestParseSubshell(t *testing.T) {
	expr, err := Parse(`(echo hello; echo world)`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	echoCount := 0
	for _, p := range expr.Pipelines {
		for _, c := range p.Commands {
			if c.Command == "echo" {
				echoCount++
			}
		}
	}
	if echoCount != 2 {
		t.Errorf("expected 2 echo commands in subshell, got %d", echoCount)
	}
}

func TestParseIfWithoutElse(t *testing.T) {
	expr, err := Parse(`if true; then echo yes; fi`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, p := range expr.Pipelines {
		for _, c := range p.Commands {
			if c.Command == "echo" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected to find 'echo' command inside if-then body")
	}
}

func TestParseAssignmentOnly(t *testing.T) {
	expr, err := Parse(`FOO=bar`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(expr.Pipelines) != 1 {
		t.Fatalf("expected 1 pipeline, got %d", len(expr.Pipelines))
	}
	cmd := expr.Pipelines[0].Commands[0]
	if cmd.Command != "" {
		t.Errorf("expected empty command for assignment-only, got %q", cmd.Command)
	}
	if len(cmd.Assignments) == 0 {
		t.Error("expected assignment to be captured")
	}
}

func TestParseSyntaxError(t *testing.T) {
	_, err := Parse(`if then fi`)
	if err == nil {
		t.Error("expected error for invalid syntax")
	}
	pe, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("expected *ParseError, got %T", err)
	}
	if pe.Message == "" {
		t.Error("expected non-empty error message")
	}
}

func TestParseRedirectOnPipeSide(t *testing.T) {
	// Triggers redirect collection inside collectPipeSide (lines 168-170)
	expr, err := Parse(`cmd1 2>/dev/null | cmd2`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(expr.Pipelines) == 0 {
		t.Fatal("expected at least one pipeline")
	}
	foundStderrRedirect := false
	for _, r := range expr.Redirects {
		if r.Type == "2>" {
			foundStderrRedirect = true
		}
	}
	if !foundStderrRedirect {
		t.Errorf("expected 2> redirect from pipe side, got %+v", expr.Redirects)
	}
}

func TestCollectPipeSideNonPipeBinary(t *testing.T) {
	// When a non-pipe BinaryCmd appears inside a pipe context, it hits the
	// else branch at line 177. This can happen with: { cmd1 && cmd2; } | cmd3
	// The "&&" BinaryCmd ends up as a stmt.Cmd inside the pipe.
	expr, err := Parse(`{ cmd1 && cmd2; } | cmd3`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(expr.Pipelines) == 0 {
		t.Fatal("expected at least one pipeline")
	}
	// cmd3 should be in the pipeline
	found := false
	for _, p := range expr.Pipelines {
		for _, c := range p.Commands {
			if c.Command == "cmd3" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected to find 'cmd3' in pipeline")
	}
}

func TestParseAssignmentWithSubstitution(t *testing.T) {
	// Tests that command substitutions in assignment values are collected
	expr, err := Parse(`FOO=$(whoami) echo hello`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(expr.Subshells) == 0 {
		t.Error("expected command substitution in assignment value to be detected")
	}
}

func TestParseDblQuotedBacktickSubstitution(t *testing.T) {
	// Backticks inside double quotes: echo "result: `ls`"
	// The printer normalizes backticks to $(), so the backtick branch in
	// collectSubstitutions is unreachable. But this tests the DblQuoted path
	// with a backtick-style substitution.
	expr, err := Parse("echo \"result: `ls`\"")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(expr.Subshells) == 0 {
		t.Fatal("expected command substitution inside double-quoted backtick to be detected")
	}
}

func TestParseIfElseIf(t *testing.T) {
	expr, err := Parse(`if true; then echo a; elif false; then echo b; else echo c; fi`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	echoCount := 0
	for _, p := range expr.Pipelines {
		for _, c := range p.Commands {
			if c.Command == "echo" {
				echoCount++
			}
		}
	}
	if echoCount < 3 {
		t.Errorf("expected at least 3 echo commands, got %d", echoCount)
	}
}

func TestParseNestedSubshellInPipe(t *testing.T) {
	// Subshell as pipe side — triggers the default case in collectPipeSide
	expr, err := Parse(`(echo hello) | grep hello`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(expr.Pipelines) == 0 {
		t.Fatal("expected at least one pipeline")
	}
}

func TestParseFlagEqualsValue(t *testing.T) {
	expr, err := Parse(`curl --header=Content-Type`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cmd := expr.Pipelines[0].Commands[0]
	found := false
	for _, f := range cmd.Flags {
		if f.Name == "--header" && f.Value == "Content-Type" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected flag --header=Content-Type, got %+v", cmd.Flags)
	}
}

func TestParseMultipleRedirectsInPipeline(t *testing.T) {
	expr, err := Parse(`cmd1 < input.txt | cmd2 > output.txt 2>&1`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	types := make(map[string]bool)
	for _, r := range expr.Redirects {
		types[r.Type] = true
	}
	if !types["<"] {
		t.Error("expected '<' redirect")
	}
	if !types[">"] {
		t.Error("expected '>' redirect")
	}
	if !types[">&"] {
		t.Error("expected '>&' redirect")
	}
}

func TestParseHereString(t *testing.T) {
	// <<< is a here-string, should trigger default redirect case
	expr, err := Parse(`cat <<< "hello world"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(expr.Redirects) == 0 {
		t.Fatal("expected redirect for here-string")
	}
	if !strings.HasPrefix(expr.Redirects[0].Type, "redirect_op_") {
		t.Errorf("expected default redirect type for here-string, got %q", expr.Redirects[0].Type)
	}
}

func TestParseCommentOnly(t *testing.T) {
	// A comment-only input is non-empty after trim but produces 0 statements
	_, err := Parse("# just a comment")
	if err == nil {
		t.Error("expected error for comment-only input")
	}
	pe, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("expected *ParseError, got %T", err)
	}
	if pe.Message != "no statements found" {
		t.Errorf("expected 'no statements found', got %q", pe.Message)
	}
}
