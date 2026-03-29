package classifier

import (
	"fmt"
	"strings"

	"github.com/agentbellnorm/kjell/internal/database"
	"github.com/agentbellnorm/kjell/internal/parser"
)

const maxRecursionDepth = 10

// ComponentResult describes the classification of a single command in a pipeline.
type ComponentResult struct {
	Command        string
	Classification database.Classification
	Reason         string
}

// ClassifyResult is the overall result of classifying a shell expression.
type ClassifyResult struct {
	Input          string
	Classification database.Classification
	Components     []ComponentResult
}

// Classifier classifies shell commands against a database.
type Classifier struct {
	db *database.Database
}

// New creates a new Classifier with the given database.
func New(db *database.Database) *Classifier {
	return &Classifier{db: db}
}

// Classify parses and classifies a shell command string.
func (c *Classifier) Classify(input string) (*ClassifyResult, error) {
	expr, err := parser.Parse(input)
	if err != nil {
		return nil, fmt.Errorf("parsing: %w", err)
	}

	result := &ClassifyResult{
		Input:          input,
		Classification: database.Read, // start optimistic
	}

	// Classify all pipelines
	for _, pipeline := range expr.Pipelines {
		pipeClass := c.classifyPipeline(pipeline, result, 0)
		result.Classification = worst(result.Classification, pipeClass)
	}

	// Classify command substitutions
	for _, sub := range expr.Subshells {
		subResult, err := c.Classify(sub)
		if err == nil {
			result.Classification = worst(result.Classification, subResult.Classification)
			result.Components = append(result.Components, ComponentResult{
				Command:        sub,
				Classification: subResult.Classification,
				Reason:         fmt.Sprintf("command substitution: %s", sub),
			})
		}
	}

	// Any output redirect makes it a write
	for _, redir := range expr.Redirects {
		if isWriteRedirect(redir) {
			result.Classification = database.Write
			result.Components = append(result.Components, ComponentResult{
				Command:        redir.Type + " " + redir.Target,
				Classification: database.Write,
				Reason:         fmt.Sprintf("redirect %s writes to %s", redir.Type, redir.Target),
			})
		}
	}

	return result, nil
}

func (c *Classifier) classifyPipeline(pipeline parser.ParsedPipeline, result *ClassifyResult, depth int) database.Classification {
	pipeClass := database.Read

	for _, cmd := range pipeline.Commands {
		if cmd.Command == "" {
			continue
		}
		cmdClass := c.classifyCommand(cmd, result, depth)
		pipeClass = worst(pipeClass, cmdClass)
	}

	return pipeClass
}

func (c *Classifier) classifyCommand(cmd parser.ParsedCommand, result *ClassifyResult, depth int) database.Classification {
	def := c.db.Lookup(cmd.Command)

	// Unknown command
	if def == nil {
		comp := ComponentResult{
			Command:        cmd.Command,
			Classification: database.Unknown,
			Reason:         fmt.Sprintf("%s: not in database", cmd.Command),
		}
		result.Components = append(result.Components, comp)
		return database.Unknown
	}

	// Handle recursive commands (sudo, env, etc.)
	if def.Recursive && depth < maxRecursionDepth {
		innerClass := c.resolveRecursive(def, cmd, result, depth)
		if innerClass != "" {
			return innerClass
		}
		// If extraction failed, fall through to default
	}

	// Check for subcommand
	cmdClass := def.Default
	reason := fmt.Sprintf("%s: default %s", cmd.Command, def.Default)

	if len(def.Subcommands) > 0 && len(cmd.Args) > 0 {
		subName := findSubcommand(cmd.Args, def)
		if subDef, ok := def.Subcommands[subName]; ok {
			cmdClass = subDef.Default
			reason = fmt.Sprintf("%s %s: %s", cmd.Command, subName, subDef.Default)

			// Handle recursive subcommands (e.g. kubectl exec pod -- ls)
			if subDef.Recursive && depth < maxRecursionDepth {
				// Build a synthetic command from the subcommand's args
				subArgs := argsAfterSubcommand(cmd.Args, subName)
				subCmd := parser.ParsedCommand{
					Command: subName,
					Args:    subArgs,
					Flags:   parser.ExtractFlags(subArgs),
				}
				innerClass := c.resolveRecursive(&subDef, subCmd, result, depth)
				if innerClass != "" {
					return innerClass
				}
			}

			// Check subcommand flags too
			if len(subDef.Flags) > 0 {
				flagClass, flagReason := c.checkFlags(cmd, subDef.Flags, result, depth)
				if flagClass != "" {
					cmdClass = worst(cmdClass, flagClass)
					reason = flagReason
				}
			}
		}
	}

	// Check flags against the command-level flags.
	// When a flag explicitly sets a classification, it replaces the default
	// (e.g., tar is unknown by default, but -t makes it read).
	if len(def.Flags) > 0 {
		flagClass, flagReason := c.checkFlags(cmd, def.Flags, result, depth)
		if flagClass != "" {
			cmdClass = flagClass
			reason = flagReason
		}
	}

	comp := ComponentResult{
		Command:        cmd.Command,
		Classification: cmdClass,
		Reason:         reason,
	}
	if len(cmd.Args) > 0 {
		// Include subcommand in the displayed command for clarity
		comp.Command = cmd.Command
		if sub := findSubcommand(cmd.Args, def); sub != "" {
			comp.Command = cmd.Command + " " + sub
		}
	}
	result.Components = append(result.Components, comp)

	return cmdClass
}

func (c *Classifier) checkFlags(cmd parser.ParsedCommand, flagDefs []database.FlagDef, result *ClassifyResult, depth int) (database.Classification, string) {
	var flagClass database.Classification
	var flagReason string

	for _, flagDef := range flagDefs {
		matched, matchedName, matchedValue := matchFlag(cmd, flagDef)
		if !matched {
			continue
		}

		switch flagDef.Effect {
		case "recursive":
			if depth < maxRecursionDepth {
				innerClass := c.resolveRecursiveFlag(flagDef, cmd, matchedName, result, depth)
				if innerClass != "" {
					flagClass = worst(flagClass, innerClass)
					flagReason = fmt.Sprintf("%s %s: recursive into inner command", cmd.Command, matchedName)
				} else {
					// Can't extract inner command — treat as write to be safe
					flagClass = worst(flagClass, database.Unknown)
					flagReason = fmt.Sprintf("%s %s: recursive flag, could not extract inner command", cmd.Command, matchedName)
				}
			}
		default:
			effect := database.Classification(flagDef.Effect)
			// Check value-dependent flags
			if len(flagDef.Values) > 0 && matchedValue != "" {
				if valEffect, ok := flagDef.Values[matchedValue]; ok {
					effect = database.Classification(valEffect)
				}
			}
			if effect != "" {
				flagClass = worst(flagClass, effect)
				if flagDef.Reason != "" {
					flagReason = fmt.Sprintf("%s %s: %s", cmd.Command, matchedName, flagDef.Reason)
				} else {
					flagReason = fmt.Sprintf("%s %s: %s", cmd.Command, matchedName, effect)
				}
			}
		}
	}

	return flagClass, flagReason
}

func matchFlag(cmd parser.ParsedCommand, flagDef database.FlagDef) (bool, string, string) {
	for _, arg := range cmd.Args {
		for _, name := range flagDef.Flag {
			if arg == name {
				// Find the value: look at parsed flags
				for _, pf := range cmd.Flags {
					if pf.Name == name {
						return true, name, pf.Value
					}
				}
				return true, name, ""
			}
			// Handle -i.bak style (flag prefix match for flags like -i)
			if len(name) == 2 && strings.HasPrefix(arg, name) && len(arg) > 2 {
				return true, name, ""
			}
			// Handle combined short flags: -tf matches -t (single-letter flags bundled)
			if len(name) == 2 && len(arg) > 2 && arg[0] == '-' && arg[1] != '-' {
				if strings.ContainsRune(arg[1:], rune(name[1])) {
					return true, name, ""
				}
			}
		}
	}
	return false, "", ""
}

func (c *Classifier) resolveRecursive(def *database.CommandDef, cmd parser.ParsedCommand, result *ClassifyResult, depth int) database.Classification {
	innerArgs := extractInnerCommand(def, cmd)
	if len(innerArgs) == 0 {
		return ""
	}

	// Reconstruct inner command string
	innerCmd := strings.Join(innerArgs, " ")
	innerResult, err := c.classifyString(innerCmd, depth+1)
	if err != nil {
		return ""
	}

	result.Components = append(result.Components, ComponentResult{
		Command:        cmd.Command,
		Classification: innerResult.Classification,
		Reason:         fmt.Sprintf("%s wraps: %s", cmd.Command, innerCmd),
	})

	return innerResult.Classification
}

func (c *Classifier) resolveRecursiveFlag(flagDef database.FlagDef, cmd parser.ParsedCommand, matchedFlag string, result *ClassifyResult, depth int) database.Classification {
	if flagDef.InnerCommandSource == "next_arg_as_shell" {
		// Find the arg after the flag and parse it as shell
		for i, arg := range cmd.Args {
			if arg == matchedFlag && i+1 < len(cmd.Args) {
				innerStr := cmd.Args[i+1]
				innerResult, err := c.classifyString(innerStr, depth+1)
				if err != nil {
					return ""
				}
				return innerResult.Classification
			}
		}
	} else if flagDef.InnerCommandSource == "trailing_args_as_shell" {
		// Everything after the flag position is the inner command
		for i, arg := range cmd.Args {
			if arg == matchedFlag && i+1 < len(cmd.Args) {
				innerStr := strings.Join(cmd.Args[i+1:], " ")
				innerResult, err := c.classifyString(innerStr, depth+1)
				if err != nil {
					return ""
				}
				return innerResult.Classification
			}
		}
	} else if len(flagDef.InnerCommandTerminator) > 0 {
		// Extract command between flag and terminator (e.g. -exec cat {} \;)
		innerArgs := extractBetweenFlagAndTerminator(cmd.Args, matchedFlag, flagDef.InnerCommandTerminator)
		if len(innerArgs) > 0 {
			// Filter out {} placeholders
			var cleanArgs []string
			for _, a := range innerArgs {
				if a != "{}" {
					cleanArgs = append(cleanArgs, a)
				}
			}
			if len(cleanArgs) > 0 {
				innerCmd := strings.Join(cleanArgs, " ")
				innerResult, err := c.classifyString(innerCmd, depth+1)
				if err != nil {
					return ""
				}
				return innerResult.Classification
			}
		}
	}
	return ""
}

func extractBetweenFlagAndTerminator(args []string, flag string, terminators []string) []string {
	collecting := false
	var inner []string
	for _, arg := range args {
		if arg == flag {
			collecting = true
			continue
		}
		if collecting {
			for _, term := range terminators {
				if arg == term || arg == `\`+term {
					return inner
				}
			}
			inner = append(inner, arg)
		}
	}
	return inner
}

func extractInnerCommand(def *database.CommandDef, cmd parser.ParsedCommand) []string {
	if def.Separator != "" {
		for i, arg := range cmd.Args {
			if arg == def.Separator {
				if i+1 < len(cmd.Args) {
					return cmd.Args[i+1:]
				}
				return nil
			}
		}
		return nil
	}

	switch pos := def.InnerCommandPosition.(type) {
	case float64:
		return findInnerByPosition(cmd.Args, int(pos))
	case int64:
		return findInnerByPosition(cmd.Args, int(pos))
	case string:
		if pos == "after_vars" {
			for i, arg := range cmd.Args {
				if !strings.Contains(arg, "=") || strings.HasPrefix(arg, "-") {
					return cmd.Args[i:]
				}
			}
		}
	}

	// Default: find first arg that looks like a command name
	return findInnerByPosition(cmd.Args, 1)
}

// findInnerByPosition finds the nth non-flag, non-flag-value arg.
// It handles cases like "nice -n 10 grep" where 10 is a flag value, not the inner command.
func findInnerByPosition(args []string, pos int) []string {
	nonFlagIdx := 0
	skipNext := false
	for i, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			// This is a flag. If it doesn't contain "=", the next arg might be its value.
			if !strings.Contains(arg, "=") && i+1 < len(args) {
				next := args[i+1]
				// Heuristic: if the next arg doesn't start with "-" and doesn't look like
				// a command (contains only digits, or is a short value), it's likely a flag value.
				if !strings.HasPrefix(next, "-") && looksLikeFlagValue(next) {
					skipNext = true
				}
			}
			continue
		}
		nonFlagIdx++
		if nonFlagIdx >= pos {
			return args[i:]
		}
	}
	return nil
}

// looksLikeFlagValue returns true if the string looks like it could be a flag value
// rather than a command name.
func looksLikeFlagValue(s string) bool {
	// Pure numbers are almost certainly flag values
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (c *Classifier) classifyString(input string, depth int) (*ClassifyResult, error) {
	if depth > maxRecursionDepth {
		return &ClassifyResult{
			Input:          input,
			Classification: database.Unknown,
		}, nil
	}

	expr, err := parser.Parse(input)
	if err != nil {
		return nil, err
	}

	result := &ClassifyResult{
		Input:          input,
		Classification: database.Read,
	}

	for _, pipeline := range expr.Pipelines {
		pipeClass := c.classifyPipeline(pipeline, result, depth)
		result.Classification = worst(result.Classification, pipeClass)
	}

	for _, sub := range expr.Subshells {
		subResult, err := c.classifyString(sub, depth+1)
		if err == nil {
			result.Classification = worst(result.Classification, subResult.Classification)
		}
	}

	for _, redir := range expr.Redirects {
		if isWriteRedirect(redir) {
			result.Classification = database.Write
		}
	}

	return result, nil
}

func argsAfterSubcommand(args []string, subName string) []string {
	for i, arg := range args {
		if arg == subName {
			if i+1 < len(args) {
				return args[i+1:]
			}
			return nil
		}
	}
	return nil
}

func findSubcommand(args []string, def *database.CommandDef) string {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		// Check if this arg is a known subcommand
		if _, ok := def.Subcommands[arg]; ok {
			return arg
		}
		// First non-flag arg that's not a known subcommand — stop looking
		return arg
	}
	return ""
}

func isWriteRedirect(r parser.Redirect) bool {
	switch r.Type {
	case ">", ">>", "2>", "2>>", "&>":
		return true
	}
	return false
}

// worst returns the more dangerous classification.
// write > unknown > read
func worst(a, b database.Classification) database.Classification {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	rank := map[database.Classification]int{
		database.Read:    0,
		database.Unknown: 1,
		database.Write:   2,
	}
	if rank[b] > rank[a] {
		return b
	}
	return a
}
