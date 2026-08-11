// Package auggieusage reads privacy-safe token usage from Augment/Auggie
// on-disk session files (~/.augment/sessions/<acpSessionId>.json).
//
// It never returns prompts, tool args, or response text. Callers publish
// deltas through the existing session_prompt_usage bus (see docs/specs/office/costs.md).
package auggieusage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ExchangeUsage is one completed chatHistory exchange with token totals only.
type ExchangeUsage struct {
	SequenceID int
	ModelID    string
	FinishedAt time.Time
	Input      int64
	Output     int64
	CacheRead  int64
	CacheWrite int64
}

// SessionPath returns ~/.augment/sessions/<acpSessionID>.json for homeDir.
// homeDir should be the user home (empty uses os.UserHomeDir).
func SessionPath(homeDir, acpSessionID string) (string, error) {
	if acpSessionID == "" {
		return "", errors.New("auggieusage: empty ACP session id")
	}
	if homeDir == "" {
		var err error
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("auggieusage: home dir: %w", err)
		}
	}
	return filepath.Join(homeDir, ".augment", "sessions", acpSessionID+".json"), nil
}

// ReadNewExchangeUsages loads sessionFile and returns completed exchanges with
// sequenceId > afterSequence. Content fields are decoded only to reach token
// nodes and are discarded from the returned values.
func ReadNewExchangeUsages(sessionFile string, afterSequence int) ([]ExchangeUsage, error) {
	raw, err := os.ReadFile(sessionFile)
	if err != nil {
		return nil, err
	}
	var file sessionFileDoc
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("auggieusage: decode %s: %w", filepath.Base(sessionFile), err)
	}
	fallbackModel := file.AgentState.ModelID
	out := make([]ExchangeUsage, 0)
	for _, item := range file.ChatHistory {
		if !item.Completed || item.SequenceID <= afterSequence {
			continue
		}
		eu := ExchangeUsage{
			SequenceID: item.SequenceID,
			ModelID:    item.Exchange.ModelID,
			FinishedAt: parseFinishedAt(item.FinishedAt),
		}
		if eu.ModelID == "" {
			eu.ModelID = fallbackModel
		}
		for _, node := range item.Exchange.ResponseNodes {
			if node.TokenUsage == nil {
				continue
			}
			tu := node.TokenUsage
			eu.Input += tu.InputTokens
			eu.Output += tu.OutputTokens
			eu.CacheRead += tu.CacheReadInputTokens
			eu.CacheWrite += tu.CacheCreationInputTokens
		}
		out = append(out, eu)
	}
	return out, nil
}

// SumExchanges aggregates exchange token counts and returns the last model id.
func SumExchanges(exchanges []ExchangeUsage) (input, output, cacheRead, cacheWrite int64, model string, maxSeq int) {
	for _, ex := range exchanges {
		input += ex.Input
		output += ex.Output
		cacheRead += ex.CacheRead
		cacheWrite += ex.CacheWrite
		if ex.SequenceID > maxSeq {
			maxSeq = ex.SequenceID
		}
		if ex.ModelID != "" {
			model = ex.ModelID
		}
	}
	return input, output, cacheRead, cacheWrite, model, maxSeq
}

type sessionFileDoc struct {
	AgentState  agentStateDoc `json:"agentState"`
	ChatHistory []historyItem `json:"chatHistory"`
}

type agentStateDoc struct {
	ModelID string `json:"modelId"`
}

type historyItem struct {
	SequenceID int         `json:"sequenceId"`
	Completed  bool        `json:"completed"`
	FinishedAt string      `json:"finishedAt"`
	Exchange   exchangeDoc `json:"exchange"`
}

type exchangeDoc struct {
	ModelID       string         `json:"model_id"`
	ResponseNodes []responseNode `json:"response_nodes"`
}

type responseNode struct {
	TokenUsage *tokenUsageDoc `json:"token_usage"`
}

type tokenUsageDoc struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
}

func parseFinishedAt(raw string) time.Time {
	if raw == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC()
	}
	return time.Time{}
}
