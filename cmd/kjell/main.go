package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	kjell "github.com/agentbellnorm/kjell"
	"github.com/agentbellnorm/kjell/internal/adapter"
	"github.com/agentbellnorm/kjell/internal/classifier"
	"github.com/agentbellnorm/kjell/internal/database"
)

func main() {
	os.Exit(run(os.Args, os.Stdin, os.Stdout, os.Stderr))
}

func run(osArgs []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	if len(osArgs) < 2 {
		printUsage(stderr)
		return 1
	}

	// Parse global flags before subcommand
	args := make([]string, len(osArgs)-1)
	copy(args, osArgs[1:])
	logLevel := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--log" {
			logLevel = "info"
			if i+1 < len(args) && (args[i+1] == "debug" || args[i+1] == "info") {
				logLevel = args[i+1]
				args = append(args[:i], args[i+2:]...)
			} else {
				args = append(args[:i], args[i+1:]...)
			}
			break
		}
	}

	if len(args) == 0 {
		printUsage(stderr)
		return 1
	}

	db, err := kjell.LoadDatabase()
	if err != nil {
		fmt.Fprintf(stderr, "error loading database: %v\n", err)
		return 1
	}

	var opts []classifier.Option
	if logLevel != "" {
		logger, closeLog := setupLog(logLevel, stderr)
		if closeLog != nil {
			defer closeLog()
		}
		if logger != nil {
			opts = append(opts, classifier.WithLogger(logger))
		}
	}

	c := classifier.New(db, opts...)

	switch args[0] {
	case "check":
		return runCheck(c, args[1:], stdin, stdout, stderr)
	case "db":
		return runDB(db, args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n", args[0])
		printUsage(stderr)
		return 1
	}
}

func setupLog(level string, stderr io.Writer) (*slog.Logger, func()) {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(stderr, "warning: could not determine home directory: %v\n", err)
		return nil, nil
	}

	dir := filepath.Join(home, ".kjell")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(stderr, "warning: could not create %s: %v\n", dir, err)
		return nil, nil
	}

	logPath := filepath.Join(dir, "log")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(stderr, "warning: could not open log file %s: %v\n", logPath, err)
		return nil, nil
	}

	slogLevel := slog.LevelInfo
	if level == "debug" {
		slogLevel = slog.LevelDebug
	}

	logger := slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: slogLevel}))
	return logger, func() { f.Close() }
}

func runCheck(c *classifier.Classifier, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	format := "plain"
	var command string

	i := 0
	for i < len(args) {
		switch args[i] {
		case "--json":
			format = "json"
			i++
		case "--format":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "error: --format requires a value")
				return 1
			}
			format = args[i+1]
			i += 2
		default:
			command = args[i]
			i++
		}
	}

	if format == "claude-code" {
		cmd, err := adapter.ClaudeCodeExtract(stdin)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 2
		}
		command = cmd
	}

	if command == "" {
		fmt.Fprintln(stderr, "error: no command to classify")
		return 1
	}

	result, err := c.Classify(command)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	switch format {
	case "plain":
		fmt.Fprint(stdout, adapter.PlainFormat(result))
	case "json":
		output, err := adapter.JSONFormat(result)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, output)
	case "claude-code":
		output, err := adapter.ClaudeCodeFormat(result)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		if output != "" {
			fmt.Fprintln(stdout, output)
		}
	default:
		fmt.Fprintf(stderr, "unknown format: %s\n", format)
		return 1
	}

	return 0
}

func runDB(db *database.Database, args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: kjell db <subcommand>")
		return 1
	}

	switch args[0] {
	case "stats":
		fmt.Fprintf(stdout, "Commands in database: %d\n", db.Commands())
	case "lookup":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "usage: kjell db lookup <command>")
			return 1
		}
		def := db.Lookup(args[1])
		if def == nil {
			fmt.Fprintf(stdout, "%s: not in database\n", args[1])
			return 1
		}
		fmt.Fprintf(stdout, "Command: %s\n", def.Command)
		fmt.Fprintf(stdout, "Default: %s\n", def.Default)
		if def.Recursive {
			fmt.Fprintln(stdout, "Recursive: yes")
		}
		if len(def.Subcommands) > 0 {
			fmt.Fprintln(stdout, "Subcommands:")
			for name, sub := range def.Subcommands {
				fmt.Fprintf(stdout, "  %s: %s\n", name, sub.Default)
			}
		}
		if len(def.Flags) > 0 {
			fmt.Fprintln(stdout, "Flags:")
			for _, f := range def.Flags {
				fmt.Fprintf(stdout, "  %s: %s", strings.Join(f.Flag, ", "), f.Effect)
				if f.Reason != "" {
					fmt.Fprintf(stdout, " (%s)", f.Reason)
				}
				fmt.Fprintln(stdout)
			}
		}
	case "validate":
		fmt.Fprintf(stdout, "Database valid: %d commands loaded\n", db.Commands())
	default:
		fmt.Fprintf(stderr, "unknown db subcommand: %s\n", args[0])
		return 1
	}

	return 0
}

func printUsage(stderr io.Writer) {
	fmt.Fprintln(stderr, `Usage: kjell [--log [debug|info]] <command> [options]

Global options:
  --log [level]  Write classification trace to ~/.kjell/log (default: info)

Commands:
  check [--json] [--format <format>] <command>
    Classify a shell command as safe/write/unknown.
    Formats: plain (default), json, claude-code

  db stats       Show database statistics
  db lookup <cmd> Show database entry for a command
  db validate    Validate the database`)
}
