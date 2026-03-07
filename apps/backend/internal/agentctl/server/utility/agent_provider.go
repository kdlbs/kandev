package utility

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// modelNameRegex validates model names - alphanumeric, hyphens, underscores, dots, slashes, colons.
var modelNameRegex = regexp.MustCompile(`^[a-zA-Z0-9._:/-]*$`)

// validateModelName ensures the model name doesn't contain shell metacharacters.
func validateModelName(model string) error {
	if model == "" {
		return nil // Empty model is allowed (uses default)
	}
	if len(model) > 256 {
		return fmt.Errorf("model name too long")
	}
	if !modelNameRegex.MatchString(model) {
		return fmt.Errorf("invalid model name: contains disallowed characters")
	}
	return nil
}

// InferenceExecutor executes one-shot prompts using the agent's inference config.
type InferenceExecutor struct {
	workDir string
}

// NewInferenceExecutor creates a new inference executor.
func NewInferenceExecutor(workDir string) *InferenceExecutor {
	return &InferenceExecutor{workDir: workDir}
}

// Execute runs a one-shot prompt using the agent's inference configuration.
func (e *InferenceExecutor) Execute(ctx context.Context, req *PromptRequest) (*PromptResponse, error) {
	if req.InferenceConfig == nil {
		return &PromptResponse{Success: false, Error: "inference config is required"}, nil
	}

	cfg := req.InferenceConfig
	if len(cfg.Command) == 0 {
		return &PromptResponse{Success: false, Error: "inference command is empty"}, nil
	}

	// Validate model name to prevent command injection.
	// Model is user-provided and substituted into command arguments.
	if err := validateModelName(req.Model); err != nil {
		return &PromptResponse{Success: false, Error: err.Error()}, nil
	}

	startTime := time.Now()

	// Build command from inference config.
	// Note: Command and ModelFlag come from hardcoded agent definitions in the registry,
	// not from user input. Only the model name (validated above) and prompt are user-provided.
	args := e.buildCommand(cfg, req.Model)

	// Command from hardcoded agent registry; model validated with regex `^[a-zA-Z0-9._:/-]*$`;
	// prompt passed via stdin, not as command arg.
	cmd := exec.CommandContext(ctx, args[0], args[1:]...) // lgtm[go/command-injection] #nosec G204
	cmd.Dir = e.workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := e.startCommand(cmd, cfg.StdinInput, req.Prompt); err != nil {
		return &PromptResponse{Success: false, Error: err.Error()}, nil
	}

	if err := cmd.Wait(); err != nil {
		return &PromptResponse{
			Success:    false,
			Error:      fmt.Sprintf("process failed: %v, stderr: %s", err, stderr.String()),
			DurationMs: int(time.Since(startTime).Milliseconds()),
		}, nil
	}

	// Parse response based on output format
	response := e.parseResponse(stdout.String(), cfg.OutputFormat)

	return &PromptResponse{
		Success:    true,
		Response:   response,
		Model:      req.Model,
		DurationMs: int(time.Since(startTime).Milliseconds()),
	}, nil
}

// startCommand starts the command with optional stdin input.
func (e *InferenceExecutor) startCommand(cmd *exec.Cmd, stdinInput bool, prompt string) error {
	if !stdinInput {
		cmd.Args = append(cmd.Args, prompt)
		return cmd.Start()
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start: %w", err)
	}

	if _, err := stdin.Write([]byte(prompt)); err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("write: %w", err)
	}

	if err := stdin.Close(); err != nil {
		return fmt.Errorf("close stdin: %w", err)
	}

	return nil
}

// buildCommand builds the command arguments from inference config.
func (e *InferenceExecutor) buildCommand(cfg *InferenceConfigDTO, model string) []string {
	args := make([]string, len(cfg.Command))
	copy(args, cfg.Command)

	if model != "" && len(cfg.ModelFlag) > 0 {
		for _, part := range cfg.ModelFlag {
			args = append(args, strings.ReplaceAll(part, "{model}", model))
		}
	}

	return args
}

// parseResponse extracts text from the output based on format.
func (e *InferenceExecutor) parseResponse(output, format string) string {
	switch format {
	case "stream-json":
		return parseStreamJSON(output)
	case "auggie-json":
		return parseAuggieJSON(output)
	default:
		return strings.TrimSpace(output)
	}
}

// parseStreamJSON parses stream-json output format (used by Amp).
func parseStreamJSON(output string) string {
	var result strings.Builder

	scanner := bufio.NewScanner(strings.NewReader(output))
	// Increase buffer for large outputs (8MB max)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		text := extractAssistantText(line)
		if text != "" {
			result.WriteString(text)
		}
	}

	// Ignore scanner errors - best effort parsing
	_ = scanner.Err()

	return strings.TrimSpace(result.String())
}

// extractAssistantText extracts text content from a JSON line if it's an assistant message.
func extractAssistantText(line string) string {
	var msg map[string]interface{}
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return ""
	}

	msgType, ok := msg["type"].(string)
	if !ok || msgType != "assistant" {
		return ""
	}

	message, ok := msg["message"].(map[string]interface{})
	if !ok {
		return ""
	}

	content, ok := message["content"].([]interface{})
	if !ok {
		return ""
	}

	return extractTextFromContent(content)
}

// extractTextFromContent extracts text from content blocks.
func extractTextFromContent(content []interface{}) string {
	var result strings.Builder
	for _, block := range content {
		b, ok := block.(map[string]interface{})
		if !ok {
			continue
		}
		blockType, ok := b["type"].(string)
		if !ok || blockType != "text" {
			continue
		}
		if text, ok := b["text"].(string); ok {
			result.WriteString(text)
		}
	}
	return result.String()
}

// parseAuggieJSON extracts the result field from auggie JSON output.
func parseAuggieJSON(output string) string {
	var response struct {
		Result  string `json:"result"`
		IsError bool   `json:"is_error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &response); err != nil {
		return strings.TrimSpace(output)
	}
	return strings.TrimSpace(response.Result)
}
