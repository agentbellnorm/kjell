package classifier

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/agentbellnorm/kjell/internal/database"
)

type complianceTestFile struct {
	Tests []complianceTest `toml:"tests"`
}

type complianceTest struct {
	Input  string `toml:"input"`
	Expect string `toml:"expect"`
	Note   string `toml:"note"`
}

func loadComplianceTests(t *testing.T, dir string) []complianceTest {
	t.Helper()
	var allTests []complianceTest

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("compliance test dir %s does not exist", dir)
		}
		t.Fatalf("reading dir %s: %v", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".toml" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}

		var file complianceTestFile
		if err := toml.Unmarshal(data, &file); err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}

		for i := range file.Tests {
			file.Tests[i].Note = fmt.Sprintf("[%s] %s", entry.Name(), file.Tests[i].Note)
		}
		allTests = append(allTests, file.Tests...)
	}

	return allTests
}

func loadDB(t *testing.T) *database.Database {
	t.Helper()
	dbDir := filepath.Join(findRepoRoot(t), "db")
	db, err := database.LoadFromFS(os.DirFS(dbDir))
	if err != nil {
		t.Fatalf("loading DB: %v", err)
	}
	return db
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	// Walk up from the test file location to find the repo root
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root")
		}
		dir = parent
	}
}

func TestComplianceCommands(t *testing.T) {
	db := loadDB(t)
	c := New(db)

	testsDir := filepath.Join(findRepoRoot(t), "tests", "commands")
	tests := loadComplianceTests(t, testsDir)

	for _, tc := range tests {
		t.Run(tc.Input, func(t *testing.T) {
			result, err := c.Classify(tc.Input)
			if err != nil {
				t.Fatalf("classify error: %v", err)
			}
			if string(result.Classification) != tc.Expect {
				t.Errorf("classify(%q) = %s, want %s  %s",
					tc.Input, result.Classification, tc.Expect, tc.Note)
			}
		})
	}
}

func TestComplianceComposition(t *testing.T) {
	db := loadDB(t)
	c := New(db)

	testsDir := filepath.Join(findRepoRoot(t), "tests", "composition")
	tests := loadComplianceTests(t, testsDir)

	for _, tc := range tests {
		t.Run(tc.Input, func(t *testing.T) {
			result, err := c.Classify(tc.Input)
			if err != nil {
				t.Fatalf("classify error: %v", err)
			}
			if string(result.Classification) != tc.Expect {
				t.Errorf("classify(%q) = %s, want %s  %s",
					tc.Input, result.Classification, tc.Expect, tc.Note)
			}
		})
	}
}

func TestComplianceEdgeCases(t *testing.T) {
	db := loadDB(t)
	c := New(db)

	testsDir := filepath.Join(findRepoRoot(t), "tests", "edge_cases")
	tests := loadComplianceTests(t, testsDir)

	for _, tc := range tests {
		t.Run(tc.Input, func(t *testing.T) {
			result, err := c.Classify(tc.Input)
			if err != nil {
				if tc.Expect == "error" {
					return // expected error
				}
				t.Fatalf("classify error: %v", err)
			}
			if tc.Expect == "error" {
				t.Error("expected error, got none")
				return
			}
			if string(result.Classification) != tc.Expect {
				t.Errorf("classify(%q) = %s, want %s  %s",
					tc.Input, result.Classification, tc.Expect, tc.Note)
			}
		})
	}
}
