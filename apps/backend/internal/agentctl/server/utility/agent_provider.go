package utility

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

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

	startTime := time.Now()

	// Build command from inference config
	args := e.buildCommand(cfg, req.Model)

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = e.workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if cfg.StdinInput {
		// Send prompt via stdin
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return &PromptResponse{Success: false, Error: fmt.Sprintf("stdin pipe: %v", err)}, nil
		}

		if err := cmd.Start(); err != nil {
			return &PromptResponse{Success: false, Error: fmt.Sprintf("start: %v", err)}, nil
		}

		if _, err := stdin.Write([]byte(req.Prompt)); err != nil {
			_ = cmd.Process.Kill()
			return &PromptResponse{Success: false, Error: fmt.Sprintf("write: %v", err)}, nil
		}
		stdin.Close()
	} else {
		// Add prompt as positional argument
		cmd.Args = append(cmd.Args, req.Prompt)
		if err := cmd.Start(); err != nil {
			return &PromptResponse{Success: false, Error: fmt.Sprintf("start: %v", err)}, nil
		}
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

// buildCommand builds the command arguments from inference config.
func (e *InferenceExecutor) buildCommand(cfg *InferenceConfigDTO, model string) []string {
	args := make([]string, len(cfg.Command))
	copy(args, cfg.Command)

	// Add model flag if specified
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
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var msg map[string]interface{}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}

		// Extract text from assistant messages
		if msgType, ok := msg["type"].(string); ok && msgType == "assistant" {
			if message, ok := msg["message"].(map[string]interface{}); ok {
				if content, ok := message["content"].([]interface{}); ok {
					for _, block := range content {
						if b, ok := block.(map[string]interface{}); ok {
							if blockType, ok := b["type"].(string); ok && blockType == "text" {
								if text, ok := b["text"].(string); ok {
									result.WriteString(text)
								}
							}
						}
					}
				}
			}
		}
	}

	return strings.TrimSpace(result.String())
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
