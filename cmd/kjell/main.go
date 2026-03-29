package main

import (
	"fmt"
	"os"
	"strings"

	kjell "github.com/agentbellnorm/kjell"
	"github.com/agentbellnorm/kjell/internal/adapter"
	"github.com/agentbellnorm/kjell/internal/classifier"
	"github.com/agentbellnorm/kjell/internal/database"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	db, err := kjell.LoadDatabase()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading database: %v\n", err)
		os.Exit(1)
	}

	c := classifier.New(db)

	switch os.Args[1] {
	case "check":
		runCheck(c, os.Args[2:])
	case "db":
		runDB(db, os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func runCheck(c *classifier.Classifier, args []string) {
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
				fmt.Fprintln(os.Stderr, "error: --format requires a value")
				os.Exit(1)
			}
			format = args[i+1]
			i += 2
		default:
			command = args[i]
			i++
		}
	}

	if format == "claude-code" {
		cmd, err := adapter.ClaudeCodeExtract(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(2)
		}
		command = cmd
	}

	if command == "" {
		fmt.Fprintln(os.Stderr, "error: no command to classify")
		os.Exit(1)
	}

	result, err := c.Classify(command)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	switch format {
	case "plain":
		fmt.Print(adapter.PlainFormat(result))
	case "json":
		output, err := adapter.JSONFormat(result)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(output)
	case "claude-code":
		output, err := adapter.ClaudeCodeFormat(result)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if output != "" {
			fmt.Println(output)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown format: %s\n", format)
		os.Exit(1)
	}
}

func runDB(db *database.Database, args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: kjell db <subcommand>")
		os.Exit(1)
	}

	switch args[0] {
	case "stats":
		fmt.Printf("Commands in database: %d\n", db.Commands())
	case "lookup":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: kjell db lookup <command>")
			os.Exit(1)
		}
		def := db.Lookup(args[1])
		if def == nil {
			fmt.Printf("%s: not in database\n", args[1])
			os.Exit(1)
		}
		fmt.Printf("Command: %s\n", def.Command)
		fmt.Printf("Default: %s\n", def.Default)
		if def.Recursive {
			fmt.Println("Recursive: yes")
		}
		if len(def.Subcommands) > 0 {
			fmt.Println("Subcommands:")
			for name, sub := range def.Subcommands {
				fmt.Printf("  %s: %s\n", name, sub.Default)
			}
		}
		if len(def.Flags) > 0 {
			fmt.Println("Flags:")
			for _, f := range def.Flags {
				fmt.Printf("  %s: %s", strings.Join(f.Flag, ", "), f.Effect)
				if f.Reason != "" {
					fmt.Printf(" (%s)", f.Reason)
				}
				fmt.Println()
			}
		}
	case "validate":
		fmt.Printf("Database valid: %d commands loaded\n", db.Commands())
	default:
		fmt.Fprintf(os.Stderr, "unknown db subcommand: %s\n", args[0])
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `Usage: kjell <command> [options]

Commands:
  check [--json] [--format <format>] <command>
    Classify a shell command as read/write/unknown.
    Formats: plain (default), json, claude-code

  db stats       Show database statistics
  db lookup <cmd> Show database entry for a command
  db validate    Validate the database`)
}
