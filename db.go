package kjell

import (
	"embed"

	"github.com/agentbellnorm/kjell/internal/database"
)

//go:embed db/*.toml
var EmbeddedDB embed.FS

// LoadDatabase loads the built-in command database.
func LoadDatabase() (*database.Database, error) {
	return database.LoadEmbedded(EmbeddedDB, "db")
}
