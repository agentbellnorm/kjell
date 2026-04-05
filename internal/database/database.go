package database

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Classification represents the safe/write/unknown classification of a command.
type Classification string

const (
	Safe    Classification = "safe"
	Write   Classification = "write"
	Unknown Classification = "unknown"
)

// FlagDef describes how a specific flag affects classification.
type FlagDef struct {
	Flag                   []string          `toml:"flag"`
	Effect                 string            `toml:"effect"` // "safe", "write", "unknown", "recursive"
	Reason                 string            `toml:"reason"`
	InnerCommandTerminator []string          `toml:"inner_command_terminators"`
	InnerCommandSource     string            `toml:"inner_command_source"` // "next_arg_as_shell", "trailing_args_as_shell"
	Values                 map[string]string `toml:"values"`              // value-dependent: e.g. {GET: "safe", POST: "write"}
}

// CommandDef defines the classification rules for a command.
type CommandDef struct {
	Command              string                `toml:"command"`
	Default              Classification        `toml:"default"`
	Flags                []FlagDef             `toml:"flags"`
	Subcommands          map[string]CommandDef  `toml:"subcommands"`
	Recursive            bool                  `toml:"recursive"`
	InnerCommandPosition interface{}           `toml:"inner_command_position"` // int or "after_vars"
	Separator            string                `toml:"separator"`
	Reason               string                `toml:"reason"`
}

// Database holds all command definitions and provides lookups.
type Database struct {
	commands map[string]CommandDef
}

// Lookup returns the CommandDef for a given command name, or nil if not found.
func (db *Database) Lookup(command string) *CommandDef {
	// Strip path prefix: /usr/bin/grep -> grep
	command = filepath.Base(command)
	if def, ok := db.commands[command]; ok {
		return &def
	}
	return nil
}

// Commands returns the number of commands in the database.
func (db *Database) Commands() int {
	return len(db.commands)
}

// CommandNames returns all command names in the database.
func (db *Database) CommandNames() []string {
	names := make([]string, 0, len(db.commands))
	for name := range db.commands {
		names = append(names, name)
	}
	return names
}

// Merge merges another database into this one. The override database takes
// precedence at every level: command default, individual subcommands, and
// individual flags (matched by any overlapping flag name).
func (db *Database) Merge(override *Database) {
	for name, overrideDef := range override.commands {
		if baseDef, ok := db.commands[name]; ok {
			db.commands[name] = mergeCommandDef(baseDef, overrideDef)
		} else {
			db.commands[name] = overrideDef
		}
	}
}

func mergeCommandDef(base, override CommandDef) CommandDef {
	result := base

	if override.Default != "" {
		result.Default = override.Default
	}
	if override.Reason != "" {
		result.Reason = override.Reason
	}
	if override.Recursive {
		result.Recursive = true
	}
	if override.Separator != "" {
		result.Separator = override.Separator
	}
	if override.InnerCommandPosition != nil {
		result.InnerCommandPosition = override.InnerCommandPosition
	}

	// Merge subcommands by name
	if len(override.Subcommands) > 0 {
		if result.Subcommands == nil {
			result.Subcommands = make(map[string]CommandDef)
		}
		for name, sub := range override.Subcommands {
			result.Subcommands[name] = sub
		}
	}

	// Merge flags — match by any overlapping flag name, replace if matched, append if new
	if len(override.Flags) > 0 {
		merged := make([]FlagDef, len(result.Flags))
		copy(merged, result.Flags)
		for _, oflag := range override.Flags {
			found := false
			for i, bflag := range merged {
				if flagsOverlap(bflag.Flag, oflag.Flag) {
					merged[i] = oflag
					found = true
					break
				}
			}
			if !found {
				merged = append(merged, oflag)
			}
		}
		result.Flags = merged
	}

	return result
}

func flagsOverlap(a, b []string) bool {
	for _, af := range a {
		for _, bf := range b {
			if af == bf {
				return true
			}
		}
	}
	return false
}

// LoadDir loads command definitions from TOML files in a directory on disk.
// It uses partial validation since these are override files that may only
// specify the fields they want to change.
func LoadDir(path string) (*Database, error) {
	return loadFromFS(os.DirFS(path), true)
}

// LoadFromFS loads command definitions from TOML files in a filesystem.
func LoadFromFS(fsys fs.FS) (*Database, error) {
	return loadFromFS(fsys, false)
}

func loadFromFS(fsys fs.FS, partial bool) (*Database, error) {
	db := &Database{commands: make(map[string]CommandDef)}

	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("reading directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}

		data, err := fs.ReadFile(fsys, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", entry.Name(), err)
		}

		var def CommandDef
		if err := toml.Unmarshal(data, &def); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", entry.Name(), err)
		}

		if err := validateCommandDef(entry.Name(), &def, partial); err != nil {
			return nil, err
		}

		normalizeCommandDef(&def)
		db.commands[def.Command] = def
	}

	return db, nil
}

// LoadEmbedded loads command definitions from an embedded filesystem.
func LoadEmbedded(embedded embed.FS, dir string) (*Database, error) {
	sub, err := fs.Sub(embedded, dir)
	if err != nil {
		return nil, fmt.Errorf("accessing embedded dir %s: %w", dir, err)
	}
	return LoadFromFS(sub)
}

func validateCommandDef(filename string, def *CommandDef, partial bool) error {
	if def.Command == "" {
		return fmt.Errorf("%s: missing required field 'command'", filename)
	}

	// Full definitions must have a default classification OR subcommands.
	// Partial (override) definitions only need the command name.
	if !partial && def.Default == "" && len(def.Subcommands) == 0 {
		return fmt.Errorf("%s: command %q must have 'default' or 'subcommands'", filename, def.Command)
	}

	if def.Default != "" {
		if err := validateClassification(filename, def.Command, string(def.Default)); err != nil {
			return err
		}
	}

	for i, flag := range def.Flags {
		if len(flag.Flag) == 0 {
			return fmt.Errorf("%s: command %q flag[%d] has no flag names", filename, def.Command, i)
		}
		validEffects := map[string]bool{"safe": true, "write": true, "unknown": true, "recursive": true}
		if !validEffects[flag.Effect] {
			return fmt.Errorf("%s: command %q flag %v has invalid effect %q", filename, def.Command, flag.Flag, flag.Effect)
		}
	}

	for name, sub := range def.Subcommands {
		if sub.Default == "" {
			return fmt.Errorf("%s: subcommand %q of %q missing 'default'", filename, name, def.Command)
		}
		if err := validateClassification(filename, def.Command+"."+name, string(sub.Default)); err != nil {
			return err
		}
	}

	return nil
}

func normalizeCommandDef(def *CommandDef) {
	// Normalize InnerCommandPosition to int for consistent type switching.
	// TOML decodes integers as int64; normalize to int.
	switch v := def.InnerCommandPosition.(type) {
	case int64:
		def.InnerCommandPosition = int(v)
	case float64:
		def.InnerCommandPosition = int(v)
	}
}

func validateClassification(filename, context, value string) error {
	switch Classification(value) {
	case Safe, Write, Unknown:
		return nil
	default:
		return fmt.Errorf("%s: %s has invalid classification %q (must be safe/write/unknown)", filename, context, value)
	}
}
