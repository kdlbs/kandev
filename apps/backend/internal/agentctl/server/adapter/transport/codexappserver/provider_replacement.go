package codexappserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"runtime"
	"sort"
	"time"

	protocol "github.com/kandev/kandev/pkg/codexappserver"
)

var replacementProviderVersion = regexp.MustCompile(`(?:^|[/ ])0\.(166|169)\.0(?:[ /(;]|$)`)

// ProviderMCPReplacementReady requires a persisted, settled native conversation
// and fresh background evidence. A missing observation cannot authorize a stop.
func (a *Adapter) ProviderMCPReplacementReady(ctx context.Context) error {
	a.mu.RLock()
	threadID, version := a.threadID, a.providerVersion
	busy := a.closed || a.client == nil || a.promptPending || a.turnID != ""
	for childID := range a.children {
		if !isTerminalChildStatus(a.childStatuses[childID]) {
			busy = true
		}
	}
	a.mu.RUnlock()
	if busy || threadID == "" || !supportedReplacementProvider(version) {
		return errors.New("provider replacement is unsupported or provider work is active")
	}
	if err := a.verifyReplacementHistory(ctx, threadID); err != nil {
		return err
	}
	terminals, err := a.FetchAllBackgroundTerminals(ctx, threadID)
	if err != nil {
		return fmt.Errorf("read provider background work: %w", err)
	}
	if len(terminals) != 0 {
		return errors.New("provider background work is active")
	}
	return nil
}

func (a *Adapter) verifyReplacementHistory(ctx context.Context, threadID string) error {
	client := a.getClient()
	if client == nil {
		return errors.New("provider is disconnected")
	}
	var response protocol.ThreadReadResponse
	if err := client.Call(ctx, protocol.MethodThreadRead, protocol.ThreadReadParams{ThreadID: threadID, IncludeTurns: true}, &response); err != nil {
		return err
	}
	if response.Thread.ID != threadID || len(response.Thread.Turns) == 0 {
		return errors.New("provider conversation is not durably resumable")
	}
	latest, err := replacementTurnBoundary(response.Thread.Turns)
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.threadID != threadID {
		return errors.New("provider native conversation changed")
	}
	if latest.After(a.providerTurnStart) {
		a.providerTurnStart = latest
	}

	return nil
}

// ProviderMCPCatalog reads the resumed native thread's complete live catalog.
// It does not run a turn or infer readiness from the requested configuration.
func (a *Adapter) ProviderMCPCatalog(ctx context.Context, threadID string) (map[string][]string, error) {
	if threadID == "" || a.GetSessionID() != threadID {
		return nil, errors.New("provider native conversation changed")
	}
	if err := a.verifyReplacementHistory(ctx, threadID); err != nil {
		return nil, err
	}
	client := a.getClient()
	if client == nil {
		return nil, errors.New("provider is disconnected")
	}
	var response struct {
		Data       []json.RawMessage `json:"data"`
		NextCursor *string           `json:"nextCursor"`
	}
	if err := client.Call(ctx, protocol.MethodMCPStatusList, protocol.MCPServerStatusListParams{ThreadID: &threadID, Detail: "full"}, &response); err != nil {
		return nil, err
	}
	if response.NextCursor != nil && *response.NextCursor != "" {
		return nil, errors.New("provider MCP catalog is incomplete")
	}
	return replacementCatalog(response.Data)
}

func replacementCatalog(data []json.RawMessage) (map[string][]string, error) {
	catalog := make(map[string][]string, len(data))
	for _, raw := range data {
		var entry struct {
			Name          string                     `json:"name"`
			RuntimeStatus string                     `json:"runtimeStatus"`
			ToolsError    *string                    `json:"toolsError"`
			Tools         map[string]json.RawMessage `json:"tools"`
		}
		if err := json.Unmarshal(raw, &entry); err != nil {
			return nil, err
		}
		if entry.Name == "" || entry.RuntimeStatus != "connected" || entry.ToolsError != nil || entry.Tools == nil {
			return nil, errors.New("provider MCP server catalog is unavailable")
		}
		if _, exists := catalog[entry.Name]; exists {
			return nil, errors.New("provider MCP server catalog is ambiguous")
		}
		names := make([]string, 0, len(entry.Tools))
		for name := range entry.Tools {
			names = append(names, name)
		}
		sort.Strings(names)
		catalog[entry.Name] = names
	}
	return catalog, nil
}

// RecordedTurnStart retains the native turn boundary through provider resume.
func (a *Adapter) RecordedTurnStart(threadID string) (time.Time, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.providerTurnStart, threadID != "" && threadID == a.threadID && !a.providerTurnStart.IsZero()
}

func replacementTurnBoundary(turns []protocol.Turn) (time.Time, error) {
	for _, turn := range turns {
		switch turn.Status {
		case "completed", "interrupted", "failed":
		default:
			return time.Time{}, errors.New("provider conversation has unsettled work")
		}
	}
	latest := time.Time{}
	for _, turn := range turns {
		if turn.StartedAt != nil && *turn.StartedAt > 0 {
			started := time.Unix(*turn.StartedAt, 0)
			if started.After(latest) {
				latest = started
			}
		}
	}
	if latest.IsZero() || latest.After(time.Now().Add(time.Minute)) {
		return time.Time{}, errors.New("provider native turn timestamp is unavailable")
	}

	return latest, nil
}

func (a *Adapter) ProviderMCPReplacementSupported() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return supportedReplacementProvider(a.providerVersion)
}
func supportedReplacementProvider(version string) bool {
	return runtime.GOOS == "linux" && replacementProviderVersion.MatchString(version)
}
