package adapter

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/agentbellnorm/kjell/internal/classifier"
	"github.com/agentbellnorm/kjell/internal/database"
)

// PlainFormat formats a ClassifyResult as human-readable text.
func PlainFormat(result *classifier.ClassifyResult) string {
	var sb strings.Builder
	sb.WriteString(strings.ToUpper(string(result.Classification)))
	sb.WriteString("\n")

	for _, comp := range result.Components {
		if comp.Reason != "" {
			sb.WriteString(fmt.Sprintf("  %s\n", comp.Reason))
		}
	}

	return sb.String()
}

// JSONOutput is the structured JSON output format.
type JSONOutput struct {
	Input          string          `json:"input"`
	Classification string          `json:"classification"`
	Components     []JSONComponent `json:"components"`
}

// JSONComponent describes one command in the result.
type JSONComponent struct {
	Command        string `json:"command"`
	Classification string `json:"classification"`
	Reason         string `json:"reason,omitempty"`
}

// JSONFormat formats a ClassifyResult as JSON.
func JSONFormat(result *classifier.ClassifyResult) (string, error) {
	output := JSONOutput{
		Input:          result.Input,
		Classification: string(result.Classification),
	}

	for _, comp := range result.Components {
		output.Components = append(output.Components, JSONComponent{
			Command:        comp.Command,
			Classification: string(comp.Classification),
			Reason:         comp.Reason,
		})
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ClaudeCodeInput is the expected PreToolUse hook input from Claude Code.
type ClaudeCodeInput struct {
	ToolName  string                 `json:"tool_name"`
	ToolInput map[string]interface{} `json:"tool_input"`
}

// ClaudeCodeOutput is the hook response format for Claude Code.
type ClaudeCodeOutput struct {
	HookSpecificOutput ClaudeCodeHookOutput `json:"hookSpecificOutput"`
}

// ClaudeCodeHookOutput contains the hook event details.
type ClaudeCodeHookOutput struct {
	HookEventName           string `json:"hookEventName"`
	PermissionDecision      string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason,omitempty"`
}

// ClaudeCodeExtract reads Claude Code PreToolUse JSON from a reader and extracts the command.
func ClaudeCodeExtract(r io.Reader) (string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("reading input: %w", err)
	}

	var input ClaudeCodeInput
	if err := json.Unmarshal(data, &input); err != nil {
		return "", fmt.Errorf("parsing JSON: %w", err)
	}

	if input.ToolName != "Bash" {
		return "", fmt.Errorf("unexpected tool_name: %q (expected Bash)", input.ToolName)
	}

	command, ok := input.ToolInput["command"]
	if !ok {
		return "", fmt.Errorf("missing tool_input.command")
	}

	cmdStr, ok := command.(string)
	if !ok {
		return "", fmt.Errorf("tool_input.command is not a string")
	}

	return cmdStr, nil
}

// ClaudeCodeFormat formats a ClassifyResult as Claude Code hook output JSON.
// For safe commands, it returns "allow" to auto-approve.
// For write/unknown, it returns empty output so Claude Code's normal
// permission system handles it (including user's "always allow" rules).
func ClaudeCodeFormat(result *classifier.ClassifyResult) (string, error) {
	if result.Classification != database.Safe {
		// No output — let Claude Code's permission system decide.
		return "", nil
	}

	output := ClaudeCodeOutput{
		HookSpecificOutput: ClaudeCodeHookOutput{
			HookEventName:            "PreToolUse",
			PermissionDecision:       "allow",
			PermissionDecisionReason: "kjell: safe",
		},
	}

	data, err := json.Marshal(output)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func formatDecisionReason(result *classifier.ClassifyResult) string {
	if result.Classification == database.Safe {
		return "kjell: safe"
	}

	// For ask decisions, only show the components that caused it
	var flagged []string
	for _, comp := range result.Components {
		if comp.Classification != database.Safe && comp.Reason != "" {
			flagged = append(flagged, comp.Reason)
		}
	}

	if len(flagged) == 0 {
		return fmt.Sprintf("kjell: %s", result.Classification)
	}

	return fmt.Sprintf("kjell: %s", strings.Join(flagged, "; "))
}
