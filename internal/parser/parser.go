package parser

import (
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// ParsedFlag represents a single parsed flag.
type ParsedFlag struct {
	Name  string
	Value string // empty if flag has no value
}

// ParsedCommand represents a single command invocation.
type ParsedCommand struct {
	Command     string
	Args        []string // all tokens after the command name
	Flags       []ParsedFlag
	Subcommand  string   // first non-flag argument for commands like git
	Assignments []string // VAR=val pairs before the command (e.g. env FOO=bar)
}

// ParsedPipeline represents commands connected by pipes.
type ParsedPipeline struct {
	Commands []ParsedCommand
	Negated  bool
}

// Redirect represents an I/O redirection.
type Redirect struct {
	Type   string // ">", ">>", "<", "2>", "2>>", "&>"
	Target string
}

// ParsedExpression is the top-level parse result.
type ParsedExpression struct {
	Pipelines []ParsedPipeline
	Operators []string // "&&", ";", "||" between pipelines
	Redirects []Redirect
	Subshells []string // command substitutions found
}

// ParseError represents a parsing failure.
type ParseError struct {
	Message string
	Pos     int
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("parse error: %s", e.Message)
}

// Parse parses a shell command string into a ParsedExpression.
func Parse(input string) (*ParsedExpression, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, &ParseError{Message: "empty input"}
	}

	reader := strings.NewReader(input)
	p := syntax.NewParser(syntax.Variant(syntax.LangBash))
	file, err := p.Parse(reader, "")
	if err != nil {
		return nil, &ParseError{Message: err.Error()}
	}

	expr := &ParsedExpression{}
	if len(file.Stmts) == 0 {
		return nil, &ParseError{Message: "no statements found"}
	}

	// Process each top-level statement
	for i, stmt := range file.Stmts {
		walkStmt(stmt, expr)

		// Multiple top-level statements are separated by ";" (or newlines)
		if i < len(file.Stmts)-1 {
			expr.Operators = append(expr.Operators, ";")
		}
	}

	return expr, nil
}

// walkStmt recursively walks a statement and appends pipelines/operators to expr.
func walkStmt(stmt *syntax.Stmt, expr *ParsedExpression) {
	// Collect redirects from the statement itself
	for _, redir := range stmt.Redirs {
		expr.Redirects = append(expr.Redirects, extractRedirect(redir))
	}

	switch cmd := stmt.Cmd.(type) {
	case *syntax.BinaryCmd:
		if cmd.Op == syntax.Pipe || cmd.Op == syntax.PipeAll {
			// Pipeline: collect all piped commands into one ParsedPipeline
			pipeline := ParsedPipeline{Negated: stmt.Negated}
			collectPipeCommands(cmd, &pipeline, expr)
			expr.Pipelines = append(expr.Pipelines, pipeline)
		} else {
			// &&, || — flatten into separate pipelines with operators
			walkStmt(cmd.X, expr)
			switch cmd.Op {
			case syntax.AndStmt:
				expr.Operators = append(expr.Operators, "&&")
			case syntax.OrStmt:
				expr.Operators = append(expr.Operators, "||")
			default:
				expr.Operators = append(expr.Operators, ";")
			}
			walkStmt(cmd.Y, expr)
		}

	case *syntax.CallExpr:
		pipeline := ParsedPipeline{
			Commands: []ParsedCommand{extractCallExpr(cmd, expr)},
			Negated:  stmt.Negated,
		}
		expr.Pipelines = append(expr.Pipelines, pipeline)

	case *syntax.Subshell:
		for _, sub := range cmd.Stmts {
			walkStmt(sub, expr)
		}

	case *syntax.ForClause:
		for _, sub := range cmd.Do {
			walkStmt(sub, expr)
		}

	case *syntax.WhileClause:
		for _, sub := range cmd.Do {
			walkStmt(sub, expr)
		}

	case *syntax.IfClause:
		for _, sub := range cmd.Then {
			walkStmt(sub, expr)
		}
		if cmd.Else != nil {
			walkStmt(&syntax.Stmt{Cmd: cmd.Else}, expr)
		}

	case *syntax.Block:
		for _, sub := range cmd.Stmts {
			walkStmt(sub, expr)
		}

	default:
		if cmd != nil {
			expr.Pipelines = append(expr.Pipelines, ParsedPipeline{
				Commands: []ParsedCommand{{}},
			})
		}
	}
}

func collectPipeCommands(cmd *syntax.BinaryCmd, pipeline *ParsedPipeline, expr *ParsedExpression) {
	// Left side
	collectPipeSide(cmd.X, pipeline, expr)
	// Right side
	collectPipeSide(cmd.Y, pipeline, expr)
}

func collectPipeSide(stmt *syntax.Stmt, pipeline *ParsedPipeline, expr *ParsedExpression) {
	// Collect redirects from this side
	for _, redir := range stmt.Redirs {
		expr.Redirects = append(expr.Redirects, extractRedirect(redir))
	}

	switch c := stmt.Cmd.(type) {
	case *syntax.BinaryCmd:
		if c.Op == syntax.Pipe || c.Op == syntax.PipeAll {
			collectPipeCommands(c, pipeline, expr)
		} else {
			pipeline.Commands = append(pipeline.Commands, ParsedCommand{})
		}
	case *syntax.CallExpr:
		pipeline.Commands = append(pipeline.Commands, extractCallExpr(c, expr))
	default:
		pipeline.Commands = append(pipeline.Commands, ParsedCommand{})
	}
}

func extractCallExpr(call *syntax.CallExpr, expr *ParsedExpression) ParsedCommand {
	if call == nil {
		return ParsedCommand{}
	}

	cmd := ParsedCommand{}

	// Extract assignments (VAR=val) and collect any command substitutions in values
	for _, assign := range call.Assigns {
		val := nodeToString(assign.Value)
		cmd.Assignments = append(cmd.Assignments, assign.Name.Value+"="+val)
		if assign.Value != nil {
			collectSubstitutions(assign.Value, expr)
		}
	}

	// If there are only assignments and no args, it's just variable setting
	if len(call.Args) == 0 {
		return cmd
	}

	// Walk all words, collecting subshells
	for _, word := range call.Args {
		collectSubstitutions(word, expr)
	}

	// Extract command name and arguments
	words := make([]string, 0, len(call.Args))
	for _, word := range call.Args {
		words = append(words, wordToString(word))
	}

	if len(words) == 0 {
		return cmd
	}

	cmd.Command = words[0]
	if len(words) > 1 {
		cmd.Args = words[1:]
	}

	// Parse flags from args
	cmd.Flags = extractFlags(cmd.Args)

	return cmd
}

func collectSubstitutions(word *syntax.Word, expr *ParsedExpression) {
	for _, part := range word.Parts {
		switch p := part.(type) {
		case *syntax.CmdSubst:
			var sb strings.Builder
			syntax.NewPrinter().Print(&sb, p)
			inner := sb.String()
			if strings.HasPrefix(inner, "$(") && strings.HasSuffix(inner, ")") {
				inner = inner[2 : len(inner)-1]
			} else if strings.HasPrefix(inner, "`") && strings.HasSuffix(inner, "`") {
				inner = inner[1 : len(inner)-1]
			}
			expr.Subshells = append(expr.Subshells, inner)
		case *syntax.DblQuoted:
			for _, qp := range p.Parts {
				if cs, ok := qp.(*syntax.CmdSubst); ok {
					var sb strings.Builder
					syntax.NewPrinter().Print(&sb, cs)
					inner := sb.String()
					if strings.HasPrefix(inner, "$(") && strings.HasSuffix(inner, ")") {
						inner = inner[2 : len(inner)-1]
					}
					expr.Subshells = append(expr.Subshells, inner)
				}
			}
		}
	}
}

// ExtractFlags parses flags from a list of arguments.
func ExtractFlags(args []string) []ParsedFlag {
	return extractFlags(args)
}

func extractFlags(args []string) []ParsedFlag {
	var flags []ParsedFlag
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break // end of flags
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			flag := ParsedFlag{Name: arg}
			if strings.Contains(arg, "=") {
				parts := strings.SplitN(arg, "=", 2)
				flag.Name = parts[0]
				flag.Value = parts[1]
			} else if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				flag.Value = args[i+1]
			}
			flags = append(flags, flag)
		}
	}
	return flags
}

func wordToString(word *syntax.Word) string {
	var sb strings.Builder
	syntax.NewPrinter().Print(&sb, word)
	s := sb.String()
	// Unquote simple strings
	if len(s) >= 2 {
		if (s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func nodeToString(node syntax.Node) string {
	if node == nil {
		return ""
	}
	var sb strings.Builder
	syntax.NewPrinter().Print(&sb, node)
	return sb.String()
}

func extractRedirect(redir *syntax.Redirect) Redirect {
	r := Redirect{}

	switch redir.Op {
	case syntax.RdrOut:
		r.Type = ">"
	case syntax.AppOut:
		r.Type = ">>"
	case syntax.RdrIn:
		r.Type = "<"
	case syntax.DplOut:
		r.Type = ">&"
	case syntax.RdrAll:
		r.Type = "&>"
	default:
		r.Type = ">"
	}

	if redir.N != nil && redir.N.Value == "2" {
		if redir.Op == syntax.RdrOut {
			r.Type = "2>"
		} else if redir.Op == syntax.AppOut {
			r.Type = "2>>"
		}
	}

	if redir.Word != nil {
		r.Target = wordToString(redir.Word)
	}

	return r
}
