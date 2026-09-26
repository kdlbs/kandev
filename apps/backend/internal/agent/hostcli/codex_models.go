package hostcli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ModelListTimeout bounds one app-server model listing.
const ModelListTimeout = 15 * time.Second

// Typed listing failures. Callers map them to user-facing statuses.
var (
	ErrNotLoggedIn = errors.New("host cli is not logged in")
	ErrTimeout     = errors.New("host cli model listing timed out")
)

// codexAppServerArgs starts the documented Codex app-server protocol on
// stdio. See https://github.com/openai/codex (codex-rs/app-server).
var codexAppServerArgs = []string{"app-server"}

const codexModelListLimit = 200

type jsonRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      *int   `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type jsonRPCMessage struct {
	ID     *int            `json:"id"`
	Method string          `json:"method"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type codexModel struct {
	ID                        string `json:"id"`
	Model                     string `json:"model"`
	DisplayName               string `json:"displayName"`
	Description               string `json:"description"`
	Hidden                    bool   `json:"hidden"`
	IsDefault                 bool   `json:"isDefault"`
	DefaultReasoningEffort    string `json:"defaultReasoningEffort"`
	SupportedReasoningEfforts []struct {
		ReasoningEffort string `json:"reasoningEffort"`
		Description     string `json:"description"`
	} `json:"supportedReasoningEfforts"`
}

type codexModelListResult struct {
	Data []codexModel `json:"data"`
}

// ListCodexModels asks the installed Codex CLI for its model catalogue through
// `codex app-server` (initialize, initialized, model/list). The process is
// terminated once the response arrives or the deadline passes.
func ListCodexModels(ctx context.Context, runner Runner, path string) ([]Model, error) {
	if strings.TrimSpace(path) == "" {
		return nil, ErrNotInstalled
	}
	ctx, cancel := context.WithTimeout(ctx, ModelListTimeout)
	defer cancel()

	proc, err := runner.Start(ctx, append([]string{path}, codexAppServerArgs...))
	if err != nil {
		return nil, err
	}
	defer func() { _ = proc.Kill() }()

	initID, listID := 1, 2
	requests := []jsonRPCRequest{
		{JSONRPC: "2.0", ID: &initID, Method: "initialize", Params: map[string]any{
			"clientInfo": map[string]any{"name": "kandev", "title": "Kandev", "version": "0"},
		}},
		{JSONRPC: "2.0", Method: "initialized"},
		{JSONRPC: "2.0", ID: &listID, Method: "model/list", Params: map[string]any{
			"limit": codexModelListLimit,
		}},
	}

	// Read before writing. The peer answers `initialize` before we finish
	// writing `model/list`, so a reader started afterwards deadlocks: it would
	// block on stdin while the peer blocks on stdout.
	done := make(chan responseOutcome, 1)
	go func() { done <- readResponse(proc, listID) }()

	for _, request := range requests {
		if err := writeJSONLine(proc, request); err != nil {
			return nil, fmt.Errorf("write %s: %w", request.Method, err)
		}
	}

	result, err := awaitResponse(ctx, done)
	if err != nil {
		return nil, err
	}
	return convertCodexModels(result), nil
}

func writeJSONLine(proc Process, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = proc.Stdin().Write(append(data, '\n'))
	return err
}

type responseOutcome struct {
	result codexModelListResult
	err    error
}

func awaitResponse(ctx context.Context, done <-chan responseOutcome) (codexModelListResult, error) {
	select {
	case outcome := <-done:
		return outcome.result, outcome.err
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return codexModelListResult{}, ErrTimeout
		}
		return codexModelListResult{}, ctx.Err()
	}
}

func readResponse(proc Process, wantID int) responseOutcome {
	scanner := bufio.NewScanner(proc.Stdout())
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var msg jsonRPCMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if msg.ID == nil || *msg.ID != wantID {
			continue
		}
		if msg.Error != nil {
			return responseOutcome{err: classifyRPCError(msg.Error.Message)}
		}
		var result codexModelListResult
		if err := json.Unmarshal(msg.Result, &result); err != nil {
			return responseOutcome{err: fmt.Errorf("parse model list: %w", err)}
		}
		return responseOutcome{result: result}
	}
	if err := scanner.Err(); err != nil {
		return responseOutcome{err: fmt.Errorf("read app-server output: %w", err)}
	}
	return responseOutcome{err: errors.New("app-server exited before answering model/list")}
}

func containsAny(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}

func classifyRPCError(message string) error {
	lower := strings.ToLower(message)
	if containsAny(lower, "auth", "login", "logged in", "unauthorized", "401", "sign in") {
		return fmt.Errorf("%w: %s", ErrNotLoggedIn, message)
	}
	return fmt.Errorf("model/list failed: %s", message)
}

func convertCodexModels(result codexModelListResult) []Model {
	models := make([]Model, 0, len(result.Data))
	for _, entry := range result.Data {
		if entry.Hidden {
			continue
		}
		id := strings.TrimSpace(entry.Model)
		if id == "" {
			id = strings.TrimSpace(entry.ID)
		}
		if id == "" {
			continue
		}
		name := strings.TrimSpace(entry.DisplayName)
		if name == "" {
			name = id
		}
		model := Model{ID: id, Name: name, Description: entry.Description, IsDefault: entry.IsDefault}
		if len(entry.SupportedReasoningEfforts) > 0 || entry.DefaultReasoningEffort != "" {
			efforts := make([]string, 0, len(entry.SupportedReasoningEfforts))
			for _, effort := range entry.SupportedReasoningEfforts {
				efforts = append(efforts, effort.ReasoningEffort)
			}
			model.Meta = map[string]any{
				"reasoningEfforts":       efforts,
				"defaultReasoningEffort": entry.DefaultReasoningEffort,
			}
		}
		models = append(models, model)
	}
	return models
}
