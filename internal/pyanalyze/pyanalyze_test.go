package pyanalyze

import (
	"testing"

	"github.com/agentbellnorm/kjell/internal/database"
)

// mockShellClassifier simulates kjell's shell classifier for testing.
func mockShellClassifier(cmd string) database.Classification {
	safe := map[string]bool{"ls": true, "ls -la": true, "echo hi": true, "cat /etc/hosts": true, "grep foo bar": true}
	write := map[string]bool{"rm -rf /": true, "rm foo": true}
	if safe[cmd] {
		return database.Safe
	}
	if write[cmd] {
		return database.Write
	}
	return database.Unknown
}

func TestAnalyze(t *testing.T) {
	tests := []struct {
		name   string
		code   string
		expect database.Classification
	}{
		// Safe: pure computation
		{"print", `print(1+1)`, database.Safe},
		{"json loads", `import json; json.loads('{"a":1}')`, database.Safe},
		{"list comprehension", `[str(x) for x in range(10)]`, database.Safe},
		{"sys version", `import sys; print(sys.version)`, database.Safe},
		{"os getcwd", `import os; print(os.getcwd())`, database.Safe},
		{"os getenv", `import os; os.getenv("HOME")`, database.Safe},
		{"os listdir", `import os; os.listdir(".")`, database.Safe},
		{"re match", `import re; re.match(r"\d+", "123")`, database.Safe},
		{"math sqrt", `import math; math.sqrt(4)`, database.Safe},
		{"method on variable", `data = {"a": 1}; data.keys()`, database.Safe},
		{"open read default", `open("file.txt").read()`, database.Safe},
		{"open read explicit", `open("file.txt", "r").read()`, database.Safe},
		{"open read binary", `open("file.txt", "rb").read()`, database.Safe},

		// Write: filesystem
		{"os remove", `import os; os.remove("foo")`, database.Write},
		{"os unlink", `import os; os.unlink("foo")`, database.Write},
		{"os mkdir", `import os; os.mkdir("/tmp/dir")`, database.Write},
		{"os rename", `import os; os.rename("old", "new")`, database.Write},
		{"shutil rmtree", `import shutil; shutil.rmtree("/tmp/dir")`, database.Write},
		{"shutil copy", `import shutil; shutil.copy("a", "b")`, database.Write},
		{"open write", `open("file.txt", "w").write("data")`, database.Write},
		{"open append", `open("file.txt", "a").write("data")`, database.Write},
		{"open mode keyword", `open("file.txt", mode="w")`, database.Write},

		// Shell recursion: os.system with shell classifier
		{"os.system safe", `import os; os.system("ls -la")`, database.Safe},
		{"os.system write", `import os; os.system("rm -rf /")`, database.Write},
		{"os.popen safe", `import os; os.popen("ls")`, database.Safe},

		// Shell recursion: subprocess with shell classifier
		{"subprocess.run list safe", `import subprocess; subprocess.run(["ls", "-la"])`, database.Safe},
		{"subprocess.run list write", `import subprocess; subprocess.run(["rm", "foo"])`, database.Write},
		{"subprocess.run string safe", `import subprocess; subprocess.run("ls")`, database.Safe},
		{"subprocess.call safe", `import subprocess; subprocess.call(["echo", "hi"])`, database.Safe},
		{"subprocess.Popen safe", `import subprocess; subprocess.Popen(["grep", "foo", "bar"])`, database.Safe},

		// Shell recursion: from-imported
		{"from os import system", `from os import system; system("ls")`, database.Safe},
		{"from subprocess import run", `from subprocess import run; run(["ls"])`, database.Safe},

		// Shell recursion: variable arg falls back to table
		{"subprocess.run variable", `import subprocess; subprocess.run(cmd)`, database.Write},
		{"os.system variable", `import os; os.system(cmd)`, database.Write},

		// Python recursion: exec/eval with extractable code
		{"exec safe", `exec("print(1)")`, database.Safe},
		{"exec write", `exec("import os; os.remove('foo')")`, database.Write},
		{"eval safe", `eval("len([1,2,3])")`, database.Safe},

		// exec/eval with non-extractable code falls back to unknown
		{"exec variable", `exec(code)`, database.Unknown},
		{"eval variable", `eval(expr)`, database.Unknown},

		// Unknown: unrecognized module
		{"unknown module", `import foo; foo.bar()`, database.Unknown},

		// From imports: safe
		{"from json import loads", `from json import loads; loads("{}")`, database.Safe},

		// From imports: write (non-shell, no recursion needed)
		{"from os import remove", `from os import remove; remove("foo")`, database.Write},

		// Complex call expressions should be unknown, not safe
		{"chained call", `get_func()("arg")`, database.Unknown},
		{"subscript call", `funcs[0]("arg")`, database.Unknown},

		// open() with non-static mode should be unknown
		{"open variable mode", `open("file.txt", mode)`, database.Unknown},
		{"open concat mode", `open("file.txt", "r" + "w")`, database.Unknown},
		{"open fstring mode", `open("file.txt", f"{'w'}")`, database.Unknown},

		// Aliased imports
		{"import as", `import os as o; o.remove("foo")`, database.Write},
		{"import as safe", `import json as j; j.loads("{}")`, database.Safe},
		{"from import as", `from os import remove as rm; rm("foo")`, database.Write},

		// String types: triple-quoted, raw, bytes prefixes
		{"triple quoted exec", `exec("""print(1)""")`, database.Safe},
		{"raw string re", `import re; re.match(r"pattern", "text")`, database.Safe},
		{"bytes prefix", `import re; re.match(b"pattern", b"text")`, database.Safe},

		// Method on non-identifier object (e.g. string literal)
		{"string literal method", `"hello".upper()`, database.Safe},

		// Subprocess with empty list
		{"subprocess empty list", `import subprocess; subprocess.run([])`, database.Write},

		// Subprocess with non-string list element
		{"subprocess mixed list", `import subprocess; subprocess.run([cmd])`, database.Write},

		// Subprocess with f-string in list (nodeStringContent returns "" for f-strings)
		{"subprocess fstring list", `import subprocess; subprocess.run([f"cmd"])`, database.Write},

		// open() edge cases
		{"open no args", `open()`, database.Safe},
		{"open only filename", `open("file.txt")`, database.Safe},
		{"open mode keyword safe", `open("file.txt", mode="r")`, database.Safe},
		{"open x mode", `open("file.txt", "x")`, database.Write},
		{"open r+ mode", `open("file.txt", "r+")`, database.Write},
		{"open with non-mode kwarg", `open("file.txt", encoding="utf-8")`, database.Safe},

		// Module used without import (direct name matches moduleTable)
		{"os without import", `os.getcwd()`, database.Safe},
		{"os without import write", `os.remove("foo")`, database.Write},

		// Relative from-import
		{"relative from import", `from . import foo; foo()`, database.Unknown},

		// Long shell command triggers truncate
		{"os.system long cmd", `import os; os.system("echo aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")`, database.Unknown},

		// Prefixed strings in extractable positions (tests nodeStringContent prefix stripping)
		{"exec bytes prefix", `exec(b"print(1)")`, database.Safe},
		{"exec raw prefix", `exec(r"print(1)")`, database.Safe},
		{"exec uppercase prefix", `exec(B"print(1)")`, database.Safe},
		{"subprocess bytes list", `import subprocess; subprocess.run([b"ls", b"-la"])`, database.Safe},
		{"os.system bytes arg", `import os; os.system(b"ls")`, database.Safe},

		// Triple-quoted string in shell-recursive position
		{"os.system triple quoted", `import os; os.system("""ls""")`, database.Safe},

	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Analyze(tt.code, mockShellClassifier)
			if result.Classification != tt.expect {
				t.Errorf("Analyze(%q) = %s, want %s (reasons: %v)", tt.code, result.Classification, tt.expect, result.Reasons)
			}
		})
	}
}

func TestAnalyzeMaxDepth(t *testing.T) {
	// Call analyzeAtDepth directly at the limit to test the depth guard
	result := analyzeAtDepth(`print(1)`, mockShellClassifier, maxPyDepth)
	if result.Classification != database.Unknown {
		t.Errorf("at max depth = %s, want unknown", result.Classification)
	}
}

func TestAnalyzeNilClassifierPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic with nil classifyShell")
		}
	}()
	Analyze("print(1)", nil)
}

