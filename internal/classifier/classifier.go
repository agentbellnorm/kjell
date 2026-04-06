package classifier

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/agentbellnorm/kjell/internal/database"
	"github.com/agentbellnorm/kjell/internal/parser"
	"github.com/agentbellnorm/kjell/internal/pyanalyze"
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
	db     *database.Database
	logger *slog.Logger
}

// Option configures a Classifier.
type Option func(*Classifier)

// WithLogger sets a logger for debug tracing of classification decisions.
func WithLogger(l *slog.Logger) Option {
	return func(c *Classifier) { c.logger = l }
}

// New creates a new Classifier with the given database.
func New(db *database.Database, opts ...Option) *Classifier {
	c := &Classifier{db: db}
	for _, o := range opts {
		o(c)
	}
	return c
}

func (c *Classifier) debug(msg string, args ...any) {
	if c.logger != nil {
		c.logger.Debug(msg, args...)
	}
}

func (c *Classifier) info(msg string, args ...any) {
	if c.logger != nil {
		c.logger.Info(msg, args...)
	}
}

// Classify parses and classifies a shell command string.
func (c *Classifier) Classify(input string) (*ClassifyResult, error) {
	return c.classifyAtDepth(input, 0)
}

func (c *Classifier) classifyPipeline(pipeline parser.ParsedPipeline, result *ClassifyResult, depth int) database.Classification {
	pipeClass := database.Safe

	for _, cmd := range pipeline.Commands {
		if cmd.Command == "" {
			continue
		}
		cmdClass := c.classifyCommand(cmd, result, depth)
		pipeClass = worst(pipeClass, cmdClass)
	}

	c.debug("pipeline result", "classification", pipeClass)
	return pipeClass
}

func (c *Classifier) classifyCommand(cmd parser.ParsedCommand, result *ClassifyResult, depth int) database.Classification {
	def := c.db.Lookup(cmd.Command)

	// Unknown command
	if def == nil {
		c.debug("db miss", "command", cmd.Command)
		reason := fmt.Sprintf("%s: not in database", cmd.Command)
		c.info("classified", "command", cmd.Command, "input", result.Input, "classification", database.Unknown, "reason", reason)
		comp := ComponentResult{
			Command:        cmd.Command,
			Classification: database.Unknown,
			Reason:         reason,
		}
		result.Components = append(result.Components, comp)
		return database.Unknown
	}

	c.debug("db hit", "command", cmd.Command, "default", def.Default, "recursive", def.Recursive)

	// Handle recursive commands (sudo, env, etc.)
	if def.Recursive && depth < maxRecursionDepth {
		innerClass := c.resolveRecursive(def, cmd, result, depth)
		if innerClass != "" {
			c.debug("recursive resolved", "command", cmd.Command, "inner_classification", innerClass)
			return innerClass
		}
		c.debug("recursive extraction failed", "command", cmd.Command)
		// If extraction failed, fall through to default
	}

	// Check for subcommand
	cmdClass := def.Default
	reason := fmt.Sprintf("%s: default %s", cmd.Command, def.Default)
	subcommandMatched := false

	if len(def.Subcommands) > 0 && len(cmd.Args) > 0 {
		subName := findSubcommand(cmd.Args, def)
		if subDef, ok := def.Subcommands[subName]; ok {
			subcommandMatched = true
			cmdClass = subDef.Default
			reason = fmt.Sprintf("%s %s: %s", cmd.Command, subName, subDef.Default)
			c.debug("subcommand matched", "command", cmd.Command, "subcommand", subName, "classification", subDef.Default)

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
	// When no subcommand matched, flags replace the default
	// (e.g., tar is unknown by default, but -t makes it safe).
	// When a subcommand was matched, flags compose with worst-of
	// to avoid downgrading a write subcommand.
	if len(def.Flags) > 0 {
		flagClass, flagReason := c.checkFlags(cmd, def.Flags, result, depth)
		if flagClass != "" {
			if subcommandMatched {
				cmdClass = worst(cmdClass, flagClass)
			} else {
				cmdClass = flagClass
			}
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
	c.info("classified", "command", comp.Command, "input", result.Input, "classification", cmdClass, "reason", reason)
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

		c.debug("flag matched", "command", cmd.Command, "flag", matchedName, "effect", flagDef.Effect, "value", matchedValue)

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
	innerResult, err := c.classifyAtDepth(innerCmd, depth+1)
	if err != nil {
		return ""
	}

	reason := fmt.Sprintf("%s wraps: %s", cmd.Command, innerCmd)
	c.info("classified", "command", cmd.Command, "input", result.Input, "classification", innerResult.Classification, "reason", reason)
	result.Components = append(result.Components, ComponentResult{
		Command:        cmd.Command,
		Classification: innerResult.Classification,
		Reason:         reason,
	})

	return innerResult.Classification
}

func (c *Classifier) resolveRecursiveFlag(flagDef database.FlagDef, cmd parser.ParsedCommand, matchedFlag string, result *ClassifyResult, depth int) database.Classification {
	if flagDef.InnerCommandSource == "next_arg_as_shell" {
		// Find the arg after the flag and parse it as shell
		for i, arg := range cmd.Args {
			if arg == matchedFlag && i+1 < len(cmd.Args) {
				innerStr := cmd.Args[i+1]
				innerResult, err := c.classifyAtDepth(innerStr, depth+1)
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
				innerResult, err := c.classifyAtDepth(innerStr, depth+1)
				if err != nil {
					return ""
				}
				return innerResult.Classification
			}
		}
	} else if flagDef.InnerCommandSource == "next_arg_as_python" {
		// Analyze the next argument as Python source code
		for i, arg := range cmd.Args {
			if arg == matchedFlag && i+1 < len(cmd.Args) {
				shellClassifier := func(shellCmd string) database.Classification {
					r, err := c.classifyAtDepth(shellCmd, depth+1)
					if err != nil {
						return database.Unknown
					}
					return r.Classification
				}
				pyResult := pyanalyze.Analyze(cmd.Args[i+1], shellClassifier)
				return pyResult.Classification
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
				innerResult, err := c.classifyAtDepth(innerCmd, depth+1)
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
	case int:
		return findInnerByPosition(cmd.Args, pos)
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

func (c *Classifier) classifyAtDepth(input string, depth int) (*ClassifyResult, error) {
	c.debug("classify", "input", input, "depth", depth)

	if depth > maxRecursionDepth {
		c.debug("max recursion depth exceeded", "input", input, "depth", depth)
		return &ClassifyResult{
			Input:          input,
			Classification: database.Unknown,
		}, nil
	}

	expr, err := parser.Parse(input)
	if err != nil {
		c.debug("parse error", "input", input, "error", err)
		if depth == 0 {
			return nil, fmt.Errorf("parsing: %w", err)
		}
		return nil, err
	}

	result := &ClassifyResult{
		Input:          input,
		Classification: database.Safe,
	}

	for _, pipeline := range expr.Pipelines {
		pipeClass := c.classifyPipeline(pipeline, result, depth)
		result.Classification = worst(result.Classification, pipeClass)
	}

	for _, sub := range expr.Subshells {
		c.debug("command substitution", "subshell", sub, "depth", depth)
		subResult, err := c.classifyAtDepth(sub, depth+1)
		if err == nil {
			result.Classification = worst(result.Classification, subResult.Classification)
			result.Components = append(result.Components, ComponentResult{
				Command:        sub,
				Classification: subResult.Classification,
				Reason:         fmt.Sprintf("command substitution: %s", sub),
			})
		}
	}

	for _, redir := range expr.Redirects {
		if isWriteRedirect(redir) {
			c.debug("write redirect", "type", redir.Type, "target", redir.Target)
			result.Classification = database.Write
			result.Components = append(result.Components, ComponentResult{
				Command:        redir.Type + " " + redir.Target,
				Classification: database.Write,
				Reason:         fmt.Sprintf("redirect %s writes to %s", redir.Type, redir.Target),
			})
		}
	}

	c.debug("result", "input", input, "classification", result.Classification, "depth", depth)
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
	var firstNonFlag string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		// Check if this arg is a known subcommand
		if _, ok := def.Subcommands[arg]; ok {
			return arg
		}
		// Remember first non-flag arg in case no known subcommand is found
		if firstNonFlag == "" {
			firstNonFlag = arg
		}
	}
	return firstNonFlag
}

func isWriteRedirect(r parser.Redirect) bool {
	if r.Target == "/dev/null" {
		return false
	}
	switch r.Type {
	case ">", ">>", "2>", "2>>", "&>":
		return true
	}
	return false
}

var classificationRank = map[database.Classification]int{
	database.Safe:    0,
	database.Unknown: 1,
	database.Write:   2,
}

// worst returns the more dangerous classification.
// write > unknown > safe
func worst(a, b database.Classification) database.Classification {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	if classificationRank[b] > classificationRank[a] {
		return b
	}
	return a
}
