package kjell_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agentbellnorm/kjell"
	"github.com/agentbellnorm/kjell/internal/database"
)

func TestLoadDatabase_HappyPath(t *testing.T) {
	// Point HOME to an empty temp dir so no custom overrides interfere.
	t.Setenv("HOME", t.TempDir())

	db, err := kjell.LoadDatabase()
	if err != nil {
		t.Fatalf("LoadDatabase() returned error: %v", err)
	}
	if db == nil {
		t.Fatal("LoadDatabase() returned nil database")
	}
	if db.Commands() <= 0 {
		t.Fatalf("expected a positive number of commands, got %d", db.Commands())
	}

	// Spot-check a known embedded command.
	def := db.Lookup("grep")
	if def == nil {
		t.Fatal("expected 'grep' to be in the database")
	}
	if def.Default != database.Safe {
		t.Fatalf("expected grep default to be safe, got %q", def.Default)
	}
}

func TestLoadDatabase_WithCustomOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	customDB := filepath.Join(tmpDir, ".kjell", "db")
	if err := os.MkdirAll(customDB, 0o755); err != nil {
		t.Fatalf("failed to create custom db dir: %v", err)
	}

	// Override grep from safe to write.
	override := []byte("command = \"grep\"\ndefault = \"write\"\n")
	if err := os.WriteFile(filepath.Join(customDB, "grep.toml"), override, 0o644); err != nil {
		t.Fatalf("failed to write override file: %v", err)
	}

	db, err := kjell.LoadDatabase()
	if err != nil {
		t.Fatalf("LoadDatabase() returned error: %v", err)
	}
	if db == nil {
		t.Fatal("LoadDatabase() returned nil database")
	}

	def := db.Lookup("grep")
	if def == nil {
		t.Fatal("expected 'grep' to be in the database after merge")
	}
	if def.Default != database.Write {
		t.Fatalf("expected grep default to be overridden to write, got %q", def.Default)
	}
}

func TestLoadDatabase_InvalidCustomOverride(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	customDB := filepath.Join(tmpDir, ".kjell", "db")
	if err := os.MkdirAll(customDB, 0o755); err != nil {
		t.Fatalf("failed to create custom db dir: %v", err)
	}

	// Write invalid TOML.
	invalid := []byte("this is not valid [[[ toml content !!!")
	if err := os.WriteFile(filepath.Join(customDB, "bad.toml"), invalid, 0o644); err != nil {
		t.Fatalf("failed to write invalid override file: %v", err)
	}

	_, err := kjell.LoadDatabase()
	if err == nil {
		t.Fatal("expected LoadDatabase() to return an error for invalid TOML, got nil")
	}
}

func TestLoadDatabase_NoCustomDir(t *testing.T) {
	// HOME exists but has no .kjell/db/ directory.
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	db, err := kjell.LoadDatabase()
	if err != nil {
		t.Fatalf("LoadDatabase() returned error: %v", err)
	}
	if db == nil {
		t.Fatal("LoadDatabase() returned nil database")
	}
	if db.Commands() <= 0 {
		t.Fatalf("expected a positive number of commands from embedded DB, got %d", db.Commands())
	}
}
