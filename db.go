package kjell

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/agentbellnorm/kjell/internal/database"
)

//go:embed db/*.toml
var EmbeddedDB embed.FS

// LoadDatabase loads the built-in command database, then merges any custom
// definitions from ~/.kjell/db/ on top (overriding by command name).
func LoadDatabase() (*database.Database, error) {
	db, err := database.LoadEmbedded(EmbeddedDB, "db")
	if err != nil {
		return nil, err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return db, nil
	}

	customDir := filepath.Join(home, ".kjell", "db")
	if info, err := os.Stat(customDir); err == nil && info.IsDir() {
		custom, err := database.LoadDir(customDir)
		if err != nil {
			return nil, fmt.Errorf("loading custom database from %s: %w", customDir, err)
		}
		db.Merge(custom)
	}

	return db, nil
}
