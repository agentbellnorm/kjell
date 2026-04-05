package classifier

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/agentbellnorm/kjell/internal/database"
)

func testDB(t *testing.T) *database.Database {
	t.Helper()
	fs := fstest.MapFS{
		"cat.toml":  {Data: []byte(`command = "cat"` + "\n" + `default = "safe"`)},
		"grep.toml": {Data: []byte(`command = "grep"` + "\n" + `default = "safe"`)},
		"ls.toml":   {Data: []byte(`command = "ls"` + "\n" + `default = "safe"`)},
		"pwd.toml":  {Data: []byte(`command = "pwd"` + "\n" + `default = "safe"`)},
		"sort.toml": {Data: []byte(`command = "sort"` + "\n" + `default = "safe"`)},
		"head.toml": {Data: []byte(`command = "head"` + "\n" + `default = "safe"`)},
		"echo.toml": {Data: []byte(`command = "echo"` + "\n" + `default = "safe"`)},
		"wc.toml":   {Data: []byte(`command = "wc"` + "\n" + `default = "safe"`)},
		"rm.toml":   {Data: []byte(`command = "rm"` + "\n" + `default = "write"`)},
		"cp.toml":   {Data: []byte(`command = "cp"` + "\n" + `default = "write"`)},
		"mv.toml":   {Data: []byte(`command = "mv"` + "\n" + `default = "write"`)},
		"mkdir.toml": {Data: []byte(`command = "mkdir"` + "\n" + `default = "write"`)},
		"touch.toml": {Data: []byte(`command = "touch"` + "\n" + `default = "write"`)},
		"tee.toml":  {Data: []byte(`command = "tee"` + "\n" + `default = "write"`)},
		"sed.toml": {Data: []byte(`command = "sed"
default = "safe"

[[flags]]
flag = ["-i", "--in-place"]
effect = "write"
reason = "Edits files in place"
`)},
		"git.toml": {Data: []byte(`command = "git"
default = "unknown"

[subcommands.log]
default = "safe"

[subcommands.diff]
default = "safe"

[subcommands.status]
default = "safe"

[subcommands.commit]
default = "write"

[subcommands.push]
default = "write"

[subcommands.add]
default = "write"

[subcommands.stash]
default = "write"
`)},
		"find.toml": {Data: []byte(`command = "find"
default = "safe"

[[flags]]
flag = ["-exec"]
effect = "recursive"
inner_command_terminators = [";", "+"]

[[flags]]
flag = ["-execdir"]
effect = "recursive"
inner_command_terminators = [";", "+"]

[[flags]]
flag = ["-delete"]
effect = "write"
reason = "Deletes matching files"
`)},
		"sudo.toml": {Data: []byte(`command = "sudo"
default = "unknown"
recursive = true
inner_command_position = 1
`)},
		"env.toml": {Data: []byte(`command = "env"
default = "safe"
recursive = true
inner_command_position = "after_vars"
`)},
		"xargs.toml": {Data: []byte(`command = "xargs"
default = "unknown"
recursive = true
inner_command_position = 1
`)},
		"time.toml": {Data: []byte(`command = "time"
default = "safe"
recursive = true
inner_command_position = 1
`)},
		"nice.toml": {Data: []byte(`command = "nice"
default = "safe"
recursive = true
inner_command_position = 1
`)},
		"watch.toml": {Data: []byte(`command = "watch"
default = "safe"
recursive = true
inner_command_position = 1
`)},
		"sh.toml": {Data: []byte(`command = "sh"
default = "unknown"
recursive = true

[[flags]]
flag = ["-c"]
effect = "recursive"
inner_command_source = "next_arg_as_shell"
`)},
		"bash.toml": {Data: []byte(`command = "bash"
default = "unknown"
recursive = true

[[flags]]
flag = ["-c"]
effect = "recursive"
inner_command_source = "next_arg_as_shell"
`)},
		"kubectl.toml": {Data: []byte(`command = "kubectl"
default = "unknown"

[subcommands.get]
default = "safe"

[subcommands.apply]
default = "write"

[subcommands.exec]
default = "unknown"
recursive = true
separator = "--"
`)},
		"curl.toml": {Data: []byte(`command = "curl"
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
PATCH = "write"

[[flags]]
flag = ["-d", "--data", "--data-raw", "--data-binary"]
effect = "write"
reason = "Sends data (implies POST)"
`)},
	}

	db, err := database.LoadFromFS(fs)
	if err != nil {
		t.Fatalf("failed to load test DB: %v", err)
	}
	return db
}

func classify(t *testing.T, input string) *ClassifyResult {
	t.Helper()
	db := testDB(t)
	c := New(db)
	result, err := c.Classify(input)
	if err != nil {
		t.Fatalf("classify %q: %v", input, err)
	}
	return result
}

func assertClassification(t *testing.T, input string, expected database.Classification) {
	t.Helper()
	result := classify(t, input)
	if result.Classification != expected {
		t.Errorf("classify(%q) = %s, want %s\n  components: %+v",
			input, result.Classification, expected, result.Components)
	}
}

// === Step 3: Basic Classification ===

func TestBasicSafeCommand(t *testing.T) {
	assertClassification(t, "grep -r TODO", database.Safe)
}

func TestBasicWriteCommand(t *testing.T) {
	assertClassification(t, "rm file.txt", database.Write)
}

func TestUnknownCommand(t *testing.T) {
	assertClassification(t, "some-tool --flag", database.Unknown)
}

func TestFlagOverrideToWrite(t *testing.T) {
	assertClassification(t, "sed 's/foo/bar/' file.txt", database.Safe)
	assertClassification(t, "sed -i 's/foo/bar/' file.txt", database.Write)
	assertClassification(t, "sed --in-place 's/foo/bar/' file.txt", database.Write)
}

func TestSedInPlaceWithBackup(t *testing.T) {
	assertClassification(t, "sed -i.bak 's/foo/bar/' file.txt", database.Write)
}

func TestSubcommandSafe(t *testing.T) {
	assertClassification(t, "git log", database.Safe)
	assertClassification(t, "git log --oneline", database.Safe)
	assertClassification(t, "git diff", database.Safe)
	assertClassification(t, "git status", database.Safe)
}

func TestSubcommandWrite(t *testing.T) {
	assertClassification(t, "git push", database.Write)
	assertClassification(t, "git commit -m 'test'", database.Write)
	assertClassification(t, "git add .", database.Write)
}

func TestGitUnknownSubcommand(t *testing.T) {
	// git with no recognized subcommand falls to default "unknown"
	assertClassification(t, "git", database.Unknown)
}

// === Step 4: Composition ===

func TestPipelineAllSafe(t *testing.T) {
	assertClassification(t, "cat file | grep error | sort", database.Safe)
}

func TestPipelineOneWrite(t *testing.T) {
	assertClassification(t, "cat file | tee output.log", database.Write)
}

func TestCompoundAllRead(t *testing.T) {
	assertClassification(t, "ls && pwd", database.Safe)
}

func TestCompoundOneWrite(t *testing.T) {
	assertClassification(t, "ls && rm file", database.Write)
}

func TestRedirectWrite(t *testing.T) {
	assertClassification(t, "grep error file > output.txt", database.Write)
}

func TestRedirectAppend(t *testing.T) {
	assertClassification(t, "echo msg >> log.txt", database.Write)
}

func TestCommandSubstitutionWrite(t *testing.T) {
	assertClassification(t, "echo $(rm file)", database.Write)
}

func TestCommandSubstitutionSafe(t *testing.T) {
	assertClassification(t, "echo $(ls)", database.Safe)
}

func TestPipelineWithRedirect(t *testing.T) {
	assertClassification(t, "grep error log.txt | sort > output.txt", database.Write)
}

func TestComplexPipelineSafe(t *testing.T) {
	assertClassification(t, "grep error log.txt | sort | head -20", database.Safe)
}

// === Step 5: Recursive Evaluation ===

func TestSudoSafe(t *testing.T) {
	assertClassification(t, "sudo ls -la", database.Safe)
}

func TestSudoWrite(t *testing.T) {
	assertClassification(t, "sudo rm -rf /", database.Write)
}

func TestEnvSafe(t *testing.T) {
	assertClassification(t, "env FOO=bar grep TODO", database.Safe)
}

func TestFindExecSafe(t *testing.T) {
	assertClassification(t, `find . -exec cat {} \;`, database.Safe)
}

func TestFindExecWrite(t *testing.T) {
	assertClassification(t, `find . -exec rm {} \;`, database.Write)
}

func TestFindDelete(t *testing.T) {
	assertClassification(t, "find . -name '*.tmp' -delete", database.Write)
}

func TestKubectlExecSafe(t *testing.T) {
	assertClassification(t, "kubectl exec pod -- ls", database.Safe)
}

func TestShCSafe(t *testing.T) {
	assertClassification(t, `sh -c 'grep foo bar'`, database.Safe)
}

func TestShCWrite(t *testing.T) {
	assertClassification(t, `sh -c 'rm -rf /'`, database.Write)
}

func TestBashCSafe(t *testing.T) {
	assertClassification(t, `bash -c 'ls -la'`, database.Safe)
}

func TestChainedRecursive(t *testing.T) {
	assertClassification(t, "sudo env FOO=bar xargs grep TODO", database.Safe)
}

func TestXargsWrite(t *testing.T) {
	assertClassification(t, "xargs rm", database.Write)
}

func TestXargsRead(t *testing.T) {
	assertClassification(t, "xargs grep TODO", database.Safe)
}

func TestTimeRead(t *testing.T) {
	assertClassification(t, "time ls -la", database.Safe)
}

func TestNiceRead(t *testing.T) {
	assertClassification(t, "nice -n 10 grep -r TODO .", database.Safe)
}

func TestXargsNoInnerCommand(t *testing.T) {
	// xargs with no inner command falls back to default (unknown)
	assertClassification(t, "xargs", database.Unknown)
}

func TestWatchSafe(t *testing.T) {
	assertClassification(t, "watch cat /var/log/syslog", database.Safe)
}

// === Value-dependent flags ===

func TestCurlDefault(t *testing.T) {
	assertClassification(t, "curl https://example.com", database.Safe)
}

func TestCurlPostFlag(t *testing.T) {
	assertClassification(t, "curl -X POST https://example.com/api", database.Write)
}

func TestCurlGetFlag(t *testing.T) {
	assertClassification(t, "curl -X GET https://example.com/api", database.Safe)
}

func TestCurlDataFlag(t *testing.T) {
	assertClassification(t, "curl -d 'data' https://example.com/api", database.Write)
}

// === Edge cases ===

func TestEmptyInput(t *testing.T) {
	db := testDB(t)
	c := New(db)
	_, err := c.Classify("")
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestMultipleWriteComponents(t *testing.T) {
	result := classify(t, "rm a && rm b")
	if result.Classification != database.Write {
		t.Errorf("expected write, got %s", result.Classification)
	}
}

func TestOrOperator(t *testing.T) {
	assertClassification(t, "ls || rm file", database.Write)
}

func TestSemicolon(t *testing.T) {
	assertClassification(t, "ls ; rm file", database.Write)
}

func TestCommandLevelFlagDoesNotDowngradeSubcommand(t *testing.T) {
	// A command-level flag classified as "safe" must not downgrade
	// a subcommand that is classified as "write".
	fs := fstest.MapFS{
		"tool.toml": {Data: []byte(`command = "tool"
default = "unknown"

[subcommands.deploy]
default = "write"

[[flags]]
flag = ["--verbose", "-v"]
effect = "safe"
reason = "Verbose output only"
`)},
	}
	db, err := database.LoadFromFS(fs)
	if err != nil {
		t.Fatalf("failed to load test DB: %v", err)
	}
	c := New(db)
	result, err := c.Classify("tool deploy --verbose")
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if result.Classification != database.Write {
		t.Errorf("tool deploy --verbose = %s, want write (command-level flag should not downgrade subcommand)",
			result.Classification)
	}
}

// === WithLogger / debug / info coverage ===

func TestWithLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	db := testDB(t)
	c := New(db, WithLogger(logger))

	result, err := c.Classify("ls")
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if result.Classification != database.Safe {
		t.Errorf("expected safe, got %s", result.Classification)
	}
	// Logger should have captured some output (debug and info calls)
	if buf.Len() == 0 {
		t.Error("expected logger output, got none")
	}
}

// === resolveRecursiveFlag: trailing_args_as_shell ===

func TestTrailingArgsAsShell(t *testing.T) {
	fs := fstest.MapFS{
		"ls.toml":  {Data: []byte(`command = "ls"` + "\n" + `default = "safe"`)},
		"rm.toml":  {Data: []byte(`command = "rm"` + "\n" + `default = "write"`)},
		"mytool.toml": {Data: []byte(`command = "mytool"
default = "unknown"

[[flags]]
flag = ["--run"]
effect = "recursive"
inner_command_source = "trailing_args_as_shell"
`)},
	}
	db, err := database.LoadFromFS(fs)
	if err != nil {
		t.Fatalf("failed to load test DB: %v", err)
	}
	c := New(db)

	// safe inner command
	result, err := c.Classify("mytool --run ls")
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if result.Classification != database.Safe {
		t.Errorf("mytool --run ls = %s, want safe", result.Classification)
	}

	// write inner command
	result, err = c.Classify("mytool --run rm foo")
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if result.Classification != database.Write {
		t.Errorf("mytool --run rm foo = %s, want write", result.Classification)
	}

	// trailing_args_as_shell with no args after the flag
	result, err = c.Classify("mytool --run")
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if result.Classification != database.Unknown {
		t.Errorf("mytool --run (no trailing args) = %s, want unknown", result.Classification)
	}
}

// === resolveRecursiveFlag: next_arg_as_shell with missing arg ===

func TestNextArgAsShellMissingArg(t *testing.T) {
	// sh -c with no argument after -c: the recursive flag cannot extract inner command
	// so it should fall back to unknown
	db := testDB(t)
	c := New(db)

	result, err := c.Classify("sh -c")
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if result.Classification != database.Unknown {
		t.Errorf("sh -c (no arg) = %s, want unknown", result.Classification)
	}
}

// === resolveRecursiveFlag: InnerCommandTerminator with only {} args ===

func TestFindExecOnlyPlaceholders(t *testing.T) {
	// find -exec {} \; — after filtering out {}, cleanArgs is empty,
	// so resolveRecursiveFlag returns "", and checkFlags treats it as unknown
	// ("recursive flag, could not extract inner command")
	db := testDB(t)
	c := New(db)

	result, err := c.Classify(`find . -exec {} \;`)
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if result.Classification != database.Unknown {
		t.Errorf("find . -exec {} = %s, want unknown", result.Classification)
	}
}

// === checkFlags: recursive flag where extraction fails => unknown ===

func TestRecursiveFlagExtractionFails(t *testing.T) {
	// This tests the path in checkFlags where a recursive flag resolves to ""
	// and the classifier falls back to unknown.
	// bash -c (no arg) triggers this: -c is a recursive flag with next_arg_as_shell
	// but there's no argument after -c.
	db := testDB(t)
	c := New(db)

	result, err := c.Classify("bash -c")
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if result.Classification != database.Unknown {
		t.Errorf("bash -c (no arg) = %s, want unknown", result.Classification)
	}
}

// === classifyAtDepth: max recursion depth ===

func TestMaxRecursionDepth(t *testing.T) {
	db := testDB(t)
	c := New(db)

	// Build a command that chains sudo 12 levels deep (exceeds maxRecursionDepth=10)
	cmd := "ls"
	for i := 0; i < 12; i++ {
		cmd = "sudo " + cmd
	}

	result, err := c.Classify(cmd)
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if result.Classification != database.Unknown {
		t.Errorf("deeply nested sudo = %s, want unknown", result.Classification)
	}
}

// === argsAfterSubcommand: subcommand is last arg ===

func TestArgsAfterSubcommandLastArg(t *testing.T) {
	// kubectl exec with no args after exec — triggers nil return
	db := testDB(t)
	c := New(db)

	result, err := c.Classify("kubectl exec")
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	// kubectl exec with no separator/inner command falls to subcommand default (unknown)
	if result.Classification != database.Unknown {
		t.Errorf("kubectl exec (no args) = %s, want unknown", result.Classification)
	}
}

// === worst: first arg empty ===

func TestWorstEmptyFirst(t *testing.T) {
	got := worst("", database.Write)
	if got != database.Write {
		t.Errorf("worst('', write) = %s, want write", got)
	}

	got = worst("", database.Safe)
	if got != database.Safe {
		t.Errorf("worst('', safe) = %s, want safe", got)
	}

	got = worst("", database.Unknown)
	if got != database.Unknown {
		t.Errorf("worst('', unknown) = %s, want unknown", got)
	}
}

// === matchFlag: combined short flags ===

func TestCombinedShortFlags(t *testing.T) {
	// tar -tf where -t is matched inside the combined -tf
	fs := fstest.MapFS{
		"tar.toml": {Data: []byte(`command = "tar"
default = "unknown"

[[flags]]
flag = ["-t", "--list"]
effect = "safe"
reason = "List archive contents"
`)},
	}
	db, err := database.LoadFromFS(fs)
	if err != nil {
		t.Fatalf("failed to load test DB: %v", err)
	}
	c := New(db)

	result, err := c.Classify("tar -tf archive.tar")
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if result.Classification != database.Safe {
		t.Errorf("tar -tf = %s, want safe", result.Classification)
	}
}

// === Additional edge cases for full coverage ===

func TestWithLoggerRecursiveCommand(t *testing.T) {
	// Exercise debug/info logging through recursive and pipeline paths
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	db := testDB(t)
	c := New(db, WithLogger(logger))

	result, err := c.Classify("sudo cat /etc/hosts | grep foo")
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if result.Classification != database.Safe {
		t.Errorf("expected safe, got %s", result.Classification)
	}

	output := buf.String()
	if !strings.Contains(output, "recursive") {
		t.Error("expected log output to contain 'recursive' trace")
	}
}

func TestWithLoggerUnknownCommand(t *testing.T) {
	// Exercise the info logging path for unknown commands (db miss)
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	db := testDB(t)
	c := New(db, WithLogger(logger))

	result, err := c.Classify("nonexistent-tool --flag")
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if result.Classification != database.Unknown {
		t.Errorf("expected unknown, got %s", result.Classification)
	}

	output := buf.String()
	if !strings.Contains(output, "db miss") {
		t.Error("expected log output to contain 'db miss'")
	}
}

// === resolveRecursiveFlag: next_arg_as_shell success path ===

func TestNextArgAsShellSuccess(t *testing.T) {
	// A command that is recursive with a separator (so top-level extractInnerCommand
	// fails when separator is absent), but has a recursive flag with next_arg_as_shell.
	// This ensures the flag-level recursive path is reached and succeeds.
	fs := fstest.MapFS{
		"ls.toml": {Data: []byte(`command = "ls"` + "\n" + `default = "safe"`)},
		"rm.toml": {Data: []byte(`command = "rm"` + "\n" + `default = "write"`)},
		"runner.toml": {Data: []byte(`command = "runner"
default = "unknown"
recursive = true
separator = "--"

[[flags]]
flag = ["-c"]
effect = "recursive"
inner_command_source = "next_arg_as_shell"
`)},
	}
	db, err := database.LoadFromFS(fs)
	if err != nil {
		t.Fatalf("failed to load test DB: %v", err)
	}
	c := New(db)

	// No "--" separator, so top-level recursive fails. Then -c flag triggers
	// next_arg_as_shell path which successfully classifies "ls".
	result, err := c.Classify("runner -c ls")
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if result.Classification != database.Safe {
		t.Errorf("runner -c ls = %s, want safe", result.Classification)
	}

	// Same but with a write command
	result, err = c.Classify("runner -c rm")
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if result.Classification != database.Write {
		t.Errorf("runner -c rm = %s, want write", result.Classification)
	}
}

// === resolveRecursiveFlag: next_arg_as_shell classifyAtDepth error ===

func TestNextArgAsShellParseError(t *testing.T) {
	// A recursive flag with next_arg_as_shell where the inner command fails to parse.
	// This covers the err != nil path at line 292.
	fs := fstest.MapFS{
		"runner.toml": {Data: []byte(`command = "runner"
default = "unknown"
recursive = true
separator = "--"

[[flags]]
flag = ["-c"]
effect = "recursive"
inner_command_source = "next_arg_as_shell"
`)},
	}
	db, err := database.LoadFromFS(fs)
	if err != nil {
		t.Fatalf("failed to load test DB: %v", err)
	}
	c := New(db)

	// Pass malformed shell as the next arg — should fail to parse
	result, err := c.Classify(`runner -c "if then"`)
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	// When parse fails, resolveRecursiveFlag returns "", checkFlags falls back to unknown
	if result.Classification != database.Unknown {
		t.Errorf("runner -c 'if then' = %s, want unknown", result.Classification)
	}
}

// === resolveRecursiveFlag: trailing_args_as_shell classifyAtDepth error ===

func TestTrailingArgsAsShellParseError(t *testing.T) {
	fs := fstest.MapFS{
		"mytool.toml": {Data: []byte(`command = "mytool"
default = "unknown"

[[flags]]
flag = ["--run"]
effect = "recursive"
inner_command_source = "trailing_args_as_shell"
`)},
	}
	db, err := database.LoadFromFS(fs)
	if err != nil {
		t.Fatalf("failed to load test DB: %v", err)
	}
	c := New(db)

	// Malformed shell after --run
	result, err := c.Classify(`mytool --run "if then"`)
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if result.Classification != database.Unknown {
		t.Errorf("mytool --run 'if then' = %s, want unknown", result.Classification)
	}
}

// === resolveRecursiveFlag: InnerCommandTerminator classifyAtDepth error ===

func TestExecTerminatorParseError(t *testing.T) {
	// find -exec with a malformed command that triggers a parse error
	// in the inner classifyAtDepth call
	fs := fstest.MapFS{
		"myfind.toml": {Data: []byte(`command = "myfind"
default = "safe"

[[flags]]
flag = ["-exec"]
effect = "recursive"
inner_command_terminators = [";", "+"]
`)},
	}
	db, err := database.LoadFromFS(fs)
	if err != nil {
		t.Fatalf("failed to load test DB: %v", err)
	}
	c := New(db)

	// Inner command "if then" is a parse error
	result, err := c.Classify(`myfind . -exec "if then" \;`)
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	// Parse error in inner command: resolveRecursiveFlag returns "", fall back to unknown
	if result.Classification != database.Unknown {
		t.Errorf("myfind -exec parse_error = %s, want unknown", result.Classification)
	}
}

// === Subcommand with flags ===

func TestSubcommandFlags(t *testing.T) {
	// A subcommand that has its own flags. This covers lines 142-147.
	fs := fstest.MapFS{
		"tool.toml": {Data: []byte(`command = "tool"
default = "unknown"

[subcommands.query]
default = "safe"

[[subcommands.query.flags]]
flag = ["--write"]
effect = "write"
reason = "writes data"
`)},
	}
	db, err := database.LoadFromFS(fs)
	if err != nil {
		t.Fatalf("failed to load test DB: %v", err)
	}
	c := New(db)

	// Subcommand without the flag: safe
	result, err := c.Classify("tool query")
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if result.Classification != database.Safe {
		t.Errorf("tool query = %s, want safe", result.Classification)
	}

	// Subcommand with the flag: write
	result, err = c.Classify("tool query --write")
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if result.Classification != database.Write {
		t.Errorf("tool query --write = %s, want write", result.Classification)
	}
}

// === extractInnerCommand: separator present but nothing after it ===

func TestSeparatorAtEnd(t *testing.T) {
	// kubectl exec pod -- (nothing after --)
	db := testDB(t)
	c := New(db)

	result, err := c.Classify("kubectl exec pod --")
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	// Separator found but no args after it — resolveRecursive returns ""
	// Falls back to subcommand default (unknown)
	if result.Classification != database.Unknown {
		t.Errorf("kubectl exec pod -- = %s, want unknown", result.Classification)
	}
}

// === extractBetweenFlagAndTerminator: no terminator found (returns inner at EOF) ===

func TestExecNoTerminator(t *testing.T) {
	// find -exec cat {} without a terminating \; or +
	// extractBetweenFlagAndTerminator collects args after -exec but never
	// finds a terminator, so it returns inner (line 351: return inner)
	db := testDB(t)
	c := New(db)

	result, err := c.Classify("find . -exec cat file")
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	// "cat file" is extracted (no terminator found, falls through to line 351),
	// "cat" is safe, so "find . -exec cat file" resolves to safe
	if result.Classification != database.Safe {
		t.Errorf("find . -exec cat file (no terminator) = %s, want safe", result.Classification)
	}
}

// === extractInnerCommand: separator not found returns nil ===

func TestSeparatorNotPresent(t *testing.T) {
	// kubectl exec pod (no -- separator at all)
	db := testDB(t)
	c := New(db)

	result, err := c.Classify("kubectl exec pod")
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	// No separator found, resolveRecursive returns "", falls back to subcommand default (unknown)
	if result.Classification != database.Unknown {
		t.Errorf("kubectl exec pod = %s, want unknown", result.Classification)
	}
}

// === classifyAtDepth: max recursion depth exceeded ===

func TestMaxRecursionDepthViaSubshell(t *testing.T) {
	// Use sudo chaining to get to depth 10, then a command substitution
	// pushes to depth 11 which triggers line 429 (depth > maxRecursionDepth).
	// sudo^10 echo $(ls) -> at depth 10: echo $(ls) -> subshell calls
	// classifyAtDepth("ls", 11) which hits the max recursion guard.
	db := testDB(t)
	c := New(db)

	// Build: sudo sudo ... (10 times) echo $(ls)
	cmd := "echo $(ls)"
	for i := 0; i < 10; i++ {
		cmd = "sudo " + cmd
	}
	result, err := c.Classify(cmd)
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	// The $(ls) at depth 11 returns unknown due to max recursion,
	// which is worst-cased with echo (safe), resulting in unknown
	if result.Classification != database.Unknown {
		t.Errorf("deeply nested with subshell = %s, want unknown", result.Classification)
	}
}

// === worst: empty first argument ===

func TestWorstWithEmptyClassifications(t *testing.T) {
	// Test worst("", x) returns x
	tests := []struct {
		a, b database.Classification
		want database.Classification
	}{
		{"", database.Write, database.Write},
		{"", database.Safe, database.Safe},
		{"", database.Unknown, database.Unknown},
		{database.Write, "", database.Write},
		{database.Safe, "", database.Safe},
		{database.Unknown, "", database.Unknown},
		{"", "", ""},
	}
	for _, tt := range tests {
		got := worst(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("worst(%q, %q) = %q, want %q", tt.a, tt.b, got, tt.want)
		}
	}
}

// === argsAfterSubcommand: subcommand not found at all ===

func TestArgsAfterSubcommandNotFound(t *testing.T) {
	// This is different from "subcommand is last arg" — the subcommand
	// name doesn't appear in args at all (returns nil at line 494).
	got := argsAfterSubcommand([]string{"get", "pods"}, "exec")
	if got != nil {
		t.Errorf("argsAfterSubcommand (not found) = %v, want nil", got)
	}
}

// === matchFlag: combined short flags ===

func TestCombinedShortFlagsDirectly(t *testing.T) {
	// Ensure combined short flags like -tf match -t
	fs := fstest.MapFS{
		"tar.toml": {Data: []byte(`command = "tar"
default = "unknown"

[[flags]]
flag = ["-t"]
effect = "safe"
reason = "List archive contents"

[[flags]]
flag = ["-x"]
effect = "write"
reason = "Extract files"
`)},
	}
	db, err := database.LoadFromFS(fs)
	if err != nil {
		t.Fatalf("failed to load test DB: %v", err)
	}
	c := New(db)

	// -ft contains -t (not at start, so combined short flags path is used)
	result, err := c.Classify("tar -ft archive.tar")
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if result.Classification != database.Safe {
		t.Errorf("tar -ft = %s, want safe", result.Classification)
	}

	// -fx contains -x (not at start), should classify as write
	result, err = c.Classify("tar -fx archive.tar")
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if result.Classification != database.Write {
		t.Errorf("tar -fx = %s, want write", result.Classification)
	}
}

// === resolveRecursive: inner classifyAtDepth error ===

func TestResolveRecursiveInnerError(t *testing.T) {
	// When resolveRecursive extracts an inner command that fails to parse,
	// it returns "" (line 270-272)
	fs := fstest.MapFS{
		"wrap.toml": {Data: []byte(`command = "wrap"
default = "unknown"
recursive = true
inner_command_position = 1
`)},
	}
	db, err := database.LoadFromFS(fs)
	if err != nil {
		t.Fatalf("failed to load test DB: %v", err)
	}
	c := New(db)

	// The inner command "if" alone is invalid shell, triggers parse error
	// at depth > 0, causing resolveRecursive to return ""
	result, err := c.Classify(`wrap "if then"`)
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	// resolveRecursive fails, falls through to default (unknown)
	if result.Classification != database.Unknown {
		t.Errorf("wrap parse-error = %s, want unknown", result.Classification)
	}
}
