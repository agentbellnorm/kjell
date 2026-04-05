package adapter

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agentbellnorm/kjell/internal/classifier"
	"github.com/agentbellnorm/kjell/internal/database"
)

type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, errors.New("read error")
}

func TestPlainFormatSafe(t *testing.T) {
	result := &classifier.ClassifyResult{
		Input:          "grep -r TODO",
		Classification: database.Safe,
		Components: []classifier.ComponentResult{
			{Command: "grep", Classification: database.Safe, Reason: "grep: default safe"},
		},
	}

	output := PlainFormat(result)
	if !strings.HasPrefix(output, "SAFE") {
		t.Errorf("expected output to start with SAFE, got %q", output)
	}
}

func TestPlainFormatWrite(t *testing.T) {
	result := &classifier.ClassifyResult{
		Input:          "rm file",
		Classification: database.Write,
		Components: []classifier.ComponentResult{
			{Command: "rm", Classification: database.Write, Reason: "rm: default write"},
		},
	}

	output := PlainFormat(result)
	if !strings.HasPrefix(output, "WRITE") {
		t.Errorf("expected output to start with WRITE, got %q", output)
	}
}

func TestJSONFormat(t *testing.T) {
	result := &classifier.ClassifyResult{
		Input:          "grep -r TODO",
		Classification: database.Safe,
		Components: []classifier.ComponentResult{
			{Command: "grep", Classification: database.Safe, Reason: "grep: default safe"},
		},
	}

	output, err := JSONFormat(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed JSONOutput
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if parsed.Classification != "safe" {
		t.Errorf("expected classification 'safe', got %q", parsed.Classification)
	}
	if parsed.Input != "grep -r TODO" {
		t.Errorf("expected input 'grep -r TODO', got %q", parsed.Input)
	}
}

func TestClaudeCodeExtract(t *testing.T) {
	input := `{"tool_name":"Bash","tool_input":{"command":"grep -r TODO src/"}}`
	cmd, err := ClaudeCodeExtract(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd != "grep -r TODO src/" {
		t.Errorf("expected 'grep -r TODO src/', got %q", cmd)
	}
}

func TestClaudeCodeExtractNonBash(t *testing.T) {
	input := `{"tool_name":"Write","tool_input":{"path":"/tmp/foo"}}`
	_, err := ClaudeCodeExtract(strings.NewReader(input))
	if err == nil {
		t.Error("expected error for non-Bash tool")
	}
}

func TestClaudeCodeExtractMalformed(t *testing.T) {
	_, err := ClaudeCodeExtract(strings.NewReader("not json"))
	if err == nil {
		t.Error("expected error for malformed input")
	}
}

func TestClaudeCodeExtractMissingCommand(t *testing.T) {
	input := `{"tool_name":"Bash","tool_input":{}}`
	_, err := ClaudeCodeExtract(strings.NewReader(input))
	if err == nil {
		t.Error("expected error for missing command")
	}
}

func TestClaudeCodeFormatAllow(t *testing.T) {
	result := &classifier.ClassifyResult{
		Input:          "ls -la",
		Classification: database.Safe,
		Components: []classifier.ComponentResult{
			{Command: "ls", Classification: database.Safe, Reason: "ls: default safe"},
		},
	}

	output, err := ClaudeCodeFormat(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed ClaudeCodeOutput
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if parsed.HookSpecificOutput.PermissionDecision != "allow" {
		t.Errorf("expected 'allow', got %q", parsed.HookSpecificOutput.PermissionDecision)
	}
}

func TestClaudeCodeFormatWritePassthrough(t *testing.T) {
	result := &classifier.ClassifyResult{
		Input:          "rm file",
		Classification: database.Write,
		Components: []classifier.ComponentResult{
			{Command: "rm", Classification: database.Write, Reason: "rm: default write"},
		},
	}

	output, err := ClaudeCodeFormat(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output != "" {
		t.Errorf("expected empty output for write (passthrough), got %q", output)
	}
}

func TestClaudeCodeFormatUnknownPassthrough(t *testing.T) {
	result := &classifier.ClassifyResult{
		Input:          "mystery-cmd",
		Classification: database.Unknown,
		Components: []classifier.ComponentResult{
			{Command: "mystery-cmd", Classification: database.Unknown, Reason: "mystery-cmd: not in database"},
		},
	}

	output, err := ClaudeCodeFormat(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output != "" {
		t.Errorf("expected empty output for unknown (passthrough), got %q", output)
	}
}

func TestFormatDecisionReasonSafe(t *testing.T) {
	result := &classifier.ClassifyResult{
		Input:          "ls -la",
		Classification: database.Safe,
		Components: []classifier.ComponentResult{
			{Command: "ls", Classification: database.Safe, Reason: "ls: default safe"},
		},
	}

	reason := formatDecisionReason(result)
	if reason != "kjell: safe" {
		t.Errorf("expected 'kjell: safe', got %q", reason)
	}
}

func TestFormatDecisionReasonWriteWithFlagged(t *testing.T) {
	result := &classifier.ClassifyResult{
		Input:          "rm file && curl -X POST http://example.com",
		Classification: database.Write,
		Components: []classifier.ComponentResult{
			{Command: "rm", Classification: database.Write, Reason: "rm: default write"},
			{Command: "curl", Classification: database.Write, Reason: "curl -X POST: write"},
		},
	}

	reason := formatDecisionReason(result)
	expected := "kjell: rm: default write; curl -X POST: write"
	if reason != expected {
		t.Errorf("expected %q, got %q", expected, reason)
	}
}

func TestFormatDecisionReasonWriteNoFlagged(t *testing.T) {
	result := &classifier.ClassifyResult{
		Input:          "rm file",
		Classification: database.Write,
		Components: []classifier.ComponentResult{
			{Command: "rm", Classification: database.Write, Reason: ""},
		},
	}

	reason := formatDecisionReason(result)
	if reason != "kjell: write" {
		t.Errorf("expected 'kjell: write', got %q", reason)
	}
}

func TestFormatDecisionReasonUnknownWithFlagged(t *testing.T) {
	result := &classifier.ClassifyResult{
		Input:          "mystery-cmd",
		Classification: database.Unknown,
		Components: []classifier.ComponentResult{
			{Command: "mystery-cmd", Classification: database.Unknown, Reason: "mystery-cmd: not in database"},
		},
	}

	reason := formatDecisionReason(result)
	expected := "kjell: mystery-cmd: not in database"
	if reason != expected {
		t.Errorf("expected %q, got %q", expected, reason)
	}
}

func TestJSONFormatEmptyComponents(t *testing.T) {
	result := &classifier.ClassifyResult{
		Input:          "ls",
		Classification: database.Safe,
		Components:     nil,
	}

	output, err := JSONFormat(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed JSONOutput
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if parsed.Components != nil {
		t.Errorf("expected nil components, got %v", parsed.Components)
	}
}

func TestClaudeCodeExtractCommandNotString(t *testing.T) {
	input := `{"tool_name":"Bash","tool_input":{"command":123}}`
	_, err := ClaudeCodeExtract(strings.NewReader(input))
	if err == nil {
		t.Error("expected error for non-string command")
	}
	if !strings.Contains(err.Error(), "not a string") {
		t.Errorf("expected 'not a string' in error, got %q", err.Error())
	}
}

func TestClaudeCodeExtractReadError(t *testing.T) {
	_, err := ClaudeCodeExtract(errReader{})
	if err == nil {
		t.Error("expected error for read failure")
	}
	if !strings.Contains(err.Error(), "reading input") {
		t.Errorf("expected 'reading input' in error, got %q", err.Error())
	}
}

func TestPlainFormatComponentEmptyReason(t *testing.T) {
	result := &classifier.ClassifyResult{
		Input:          "rm file",
		Classification: database.Write,
		Components: []classifier.ComponentResult{
			{Command: "rm", Classification: database.Write, Reason: ""},
		},
	}

	output := PlainFormat(result)
	if !strings.HasPrefix(output, "WRITE") {
		t.Errorf("expected output to start with WRITE, got %q", output)
	}
	// With empty reason, there should be no indented reason line
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 line (just classification), got %d: %v", len(lines), lines)
	}
}
