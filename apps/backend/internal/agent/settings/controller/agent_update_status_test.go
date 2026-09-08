package controller

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

type statusSelectionStore struct {
	selection map[string]managedruntime.Selection
}

func (s *statusSelectionStore) Get(_ context.Context, agentName, packageName string) (managedruntime.Selection, bool, error) {
	selection, ok := s.selection[agentName+"\x00"+packageName]
	return selection, ok, nil
}

func (s *statusSelectionStore) Save(context.Context, string, string, string) error {
	return nil
}

func (s *statusSelectionStore) Delete(context.Context, string, string) error {
	return nil
}

func TestListAgentUpdateStatusesComparesLatestAndCachesFailures(t *testing.T) {
	controller := newTestController(map[string]agents.Agent{
		"claude-acp":  agents.NewClaudeACP(),
		"codex-acp":   agents.NewCodexACP(),
		"copilot-acp": agents.NewCopilotACP(),
		"gemini":      agents.NewGemini(),
	})
	controller.SetManagedRuntimeSelectionStore(&statusSelectionStore{selection: map[string]managedruntime.Selection{
		"claude-acp\x00@agentclientprotocol/claude-agent-acp": {
			Package: "@agentclientprotocol/claude-agent-acp",
			Version: "0.70.1",
		},
	}})

	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	controller.SetRuntimeUpdateStatusClock(func() time.Time { return now })
	var mu sync.Mutex
	calls := make(map[string]int)
	controller.SetRuntimeUpdateStatusResolver(func(_ context.Context, packageName string) (string, error) {
		mu.Lock()
		calls[packageName]++
		mu.Unlock()
		switch packageName {
		case "@agentclientprotocol/claude-agent-acp":
			return "0.71.0", nil
		case "@agentclientprotocol/codex-acp":
			return agents.NewCodexACP().ManagedNPMRuntime().DefaultVersionOrPinned(), nil
		case "@github/copilot":
			return "1.0.74", nil
		case "@google/gemini-cli":
			return "", errors.New("registry unavailable")
		default:
			return "", errors.New("unexpected package")
		}
	})

	statuses, err := controller.ListAgentUpdateStatuses(context.Background())
	if err != nil {
		t.Fatalf("ListAgentUpdateStatuses: %v", err)
	}
	if len(statuses.Statuses) != 4 {
		t.Fatalf("status count = %d, want 4", len(statuses.Statuses))
	}
	byPackage := make(map[string]dto.AgentUpdateStatusDTO, len(statuses.Statuses))
	for _, status := range statuses.Statuses {
		byPackage[status.Package] = status
		if status.Package != "@google/gemini-cli" && status.CheckedAt == nil {
			t.Fatalf("%s has no checked_at", status.Package)
		}
	}
	if got := byPackage["@agentclientprotocol/claude-agent-acp"].CheckState; got != dto.AgentUpdateCheckStateUpdateAvailable {
		t.Fatalf("claude state = %q, want update_available", got)
	}
	if got := byPackage["@agentclientprotocol/claude-agent-acp"].EffectiveVersion; got != "0.70.1" {
		t.Fatalf("claude effective = %q, want selected version", got)
	}
	if got := byPackage["@agentclientprotocol/codex-acp"].CheckState; got != dto.AgentUpdateCheckStateUpToDate {
		t.Fatalf("codex state = %q, want up_to_date", got)
	}
	if got := byPackage["@github/copilot"].CheckState; got != dto.AgentUpdateCheckStateUpToDate {
		t.Fatalf("copilot state = %q, want up_to_date for older latest", got)
	}
	if got := byPackage["@google/gemini-cli"].CheckState; got != dto.AgentUpdateCheckStateUnknown {
		t.Fatalf("gemini state = %q, want unknown", got)
	}
	if status := byPackage["@google/gemini-cli"]; status.CheckedAt != nil || status.LatestVersion != "" {
		t.Fatalf("gemini failure metadata = latest %q, checked_at %v; want both omitted", status.LatestVersion, status.CheckedAt)
	}

	if _, err := controller.ListAgentUpdateStatuses(context.Background()); err != nil {
		t.Fatalf("cached ListAgentUpdateStatuses: %v", err)
	}
	mu.Lock()
	for packageName, count := range calls {
		if count != 1 {
			t.Errorf("resolver calls for %s = %d, want 1 within TTL", packageName, count)
		}
	}
	mu.Unlock()

	now = now.Add(15*time.Minute + time.Second)
	if _, err := controller.ListAgentUpdateStatuses(context.Background()); err != nil {
		t.Fatalf("failed-cache refresh: %v", err)
	}
	mu.Lock()
	if calls["@google/gemini-cli"] != 2 {
		t.Errorf("failed resolver calls = %d, want 2 after failure TTL", calls["@google/gemini-cli"])
	}
	if calls["@agentclientprotocol/claude-agent-acp"] != 1 {
		t.Errorf("successful resolver calls = %d, want 1 before success TTL", calls["@agentclientprotocol/claude-agent-acp"])
	}
	mu.Unlock()

	now = now.Add(6*time.Hour + time.Second)
	if _, err := controller.ListAgentUpdateStatuses(context.Background()); err != nil {
		t.Fatalf("successful-cache refresh: %v", err)
	}
	mu.Lock()
	if calls["@agentclientprotocol/claude-agent-acp"] != 2 {
		t.Errorf("successful resolver calls = %d, want 2 after success TTL", calls["@agentclientprotocol/claude-agent-acp"])
	}
	mu.Unlock()

	if got := controller.ListAgentUpdateJobs(); len(got) != 0 {
		t.Fatalf("status request created %d update jobs", len(got))
	}
}

func TestListAgentUpdateStatusesInvalidatesOnePackage(t *testing.T) {
	controller := newTestController(map[string]agents.Agent{
		"claude-acp": agents.NewClaudeACP(),
	})
	var calls int
	controller.SetRuntimeUpdateStatusResolver(func(context.Context, string) (string, error) {
		calls++
		return "0.71.0", nil
	})

	if _, err := controller.ListAgentUpdateStatuses(context.Background()); err != nil {
		t.Fatalf("initial status: %v", err)
	}
	controller.InvalidateRuntimeUpdateStatus("@agentclientprotocol/claude-agent-acp")
	if _, err := controller.ListAgentUpdateStatuses(context.Background()); err != nil {
		t.Fatalf("status after invalidation: %v", err)
	}
	if calls != 2 {
		t.Fatalf("resolver calls = %d, want 2 after invalidation", calls)
	}
}

func TestListAgentUpdateStatusesInitializesCacheForBareController(t *testing.T) {
	controller := newTestController(map[string]agents.Agent{
		"claude-acp": agents.NewClaudeACP(),
	})

	statuses, err := controller.ListAgentUpdateStatuses(context.Background())
	if err != nil {
		t.Fatalf("ListAgentUpdateStatuses: %v", err)
	}
	if len(statuses.Statuses) != 1 || statuses.Statuses[0].CheckState != dto.AgentUpdateCheckStateUnknown {
		t.Fatalf("statuses = %#v, want one unknown status", statuses.Statuses)
	}
}

func TestListAgentUpdateStatusesBoundsRegistryLookupByContext(t *testing.T) {
	controller := newTestController(map[string]agents.Agent{
		"claude-acp": agents.NewClaudeACP(),
	})
	controller.SetRuntimeUpdateStatusResolver(func(ctx context.Context, _ string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	statuses, err := controller.ListAgentUpdateStatuses(ctx)
	if err != nil {
		t.Fatalf("ListAgentUpdateStatuses: %v", err)
	}
	if len(statuses.Statuses) != 1 || statuses.Statuses[0].CheckState != dto.AgentUpdateCheckStateUnknown {
		t.Fatalf("statuses = %#v, want one unknown status", statuses.Statuses)
	}
}

func TestListAgentUpdateStatusesBoundsConcurrentLookups(t *testing.T) {
	controller := newTestController(map[string]agents.Agent{
		"claude-acp":   agents.NewClaudeACP(),
		"codex-acp":    agents.NewCodexACP(),
		"copilot-acp":  agents.NewCopilotACP(),
		"gemini":       agents.NewGemini(),
		"opencode-acp": agents.NewOpenCodeACP(),
	})
	var mu sync.Mutex
	current, maximum := 0, 0
	controller.SetRuntimeUpdateStatusResolver(func(context.Context, string) (string, error) {
		mu.Lock()
		current++
		if current > maximum {
			maximum = current
		}
		mu.Unlock()
		time.Sleep(5 * time.Millisecond)
		mu.Lock()
		current--
		mu.Unlock()
		return "9.9.9", nil
	})

	if _, err := controller.ListAgentUpdateStatuses(context.Background()); err != nil {
		t.Fatalf("ListAgentUpdateStatuses: %v", err)
	}
	if maximum > 5 {
		t.Fatalf("maximum concurrent lookups = %d, want at most 5", maximum)
	}
}
