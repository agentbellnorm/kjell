package classifier

import (
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

func TestBasicReadCommand(t *testing.T) {
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

func TestSubcommandRead(t *testing.T) {
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

func TestPipelineAllRead(t *testing.T) {
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

func TestCommandSubstitutionRead(t *testing.T) {
	assertClassification(t, "echo $(ls)", database.Safe)
}

func TestPipelineWithRedirect(t *testing.T) {
	assertClassification(t, "grep error log.txt | sort > output.txt", database.Write)
}

func TestComplexPipelineRead(t *testing.T) {
	assertClassification(t, "grep error log.txt | sort | head -20", database.Safe)
}

// === Step 5: Recursive Evaluation ===

func TestSudoRead(t *testing.T) {
	assertClassification(t, "sudo ls -la", database.Safe)
}

func TestSudoWrite(t *testing.T) {
	assertClassification(t, "sudo rm -rf /", database.Write)
}

func TestEnvRead(t *testing.T) {
	assertClassification(t, "env FOO=bar grep TODO", database.Safe)
}

func TestFindExecRead(t *testing.T) {
	assertClassification(t, `find . -exec cat {} \;`, database.Safe)
}

func TestFindExecWrite(t *testing.T) {
	assertClassification(t, `find . -exec rm {} \;`, database.Write)
}

func TestFindDelete(t *testing.T) {
	assertClassification(t, "find . -name '*.tmp' -delete", database.Write)
}

func TestKubectlExecRead(t *testing.T) {
	assertClassification(t, "kubectl exec pod -- ls", database.Safe)
}

func TestShCRead(t *testing.T) {
	assertClassification(t, `sh -c 'grep foo bar'`, database.Safe)
}

func TestShCWrite(t *testing.T) {
	assertClassification(t, `sh -c 'rm -rf /'`, database.Write)
}

func TestBashCRead(t *testing.T) {
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

func TestWatchRead(t *testing.T) {
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
