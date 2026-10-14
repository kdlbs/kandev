//go:build e2e

package e2e

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/pkg/agent"
)

const nativeCodexE2EEnv = "KANDEV_CODEX_APP_SERVER_E2E"

type nativeSessionForker interface {
	ForkSession(ctx context.Context, sourceThreadID, completedTurnID string) (string, error)
}

func TestNativeCodexAppServerLifecycleResumeAndFork(t *testing.T) {
	if os.Getenv(nativeCodexE2EEnv) != "1" {
		t.Skipf("set %s=1 to run disposable authenticated Codex app-server coverage", nativeCodexE2EEnv)
	}
	if err := nativeCodexPreflight(); err != nil {
		t.Fatal(err)
	}

	command := acpCommand(t, agents.NewCodexAppServer(true))
	parts := strings.Fields(command)
	if len(parts) == 0 {
		t.Fatal("Codex app-server command is empty")
	}
	if _, err := exec.LookPath(parts[0]); err != nil {
		t.Fatalf("Codex app-server preflight: executable %q is not on PATH: %v", parts[0], err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	started := time.Now()
	workDir := setupWorkspace(t)
	cfg := buildInstanceConfig(command, agent.ProtocolCodexAppServer, workDir, false, "")
	mgr := process.NewManager(cfg, newTestLogger(t))
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("start pinned Codex app-server runtime: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		_ = mgr.Stop(stopCtx)
	})

	adpt := mgr.GetAdapter()
	if adpt == nil {
		t.Fatal("Codex app-server adapter is nil after start")
	}
	if err := adpt.Initialize(ctx); err != nil {
		t.Fatalf("initialize Codex app-server: %v", err)
	}
	if info := adpt.GetAgentInfo(); info == nil || info.Version != "codex-cli 0.154.0" {
		t.Fatalf("runtime identity = %#v, want pinned Codex 0.154.0", info)
	}
	threadID, err := adpt.NewSession(ctx, nil)
	if err != nil {
		t.Fatalf("start disposable Codex thread: %v", err)
	}

	first := collectNativeCodexTurn(ctx, t, mgr, adpt, `Create codex-app-server-e2e.txt with exactly this text: "native Codex fork probe". Then report done.`, 1)
	AssertNoErrors(t, first)
	firstTurnID := nativeCompletedTurnID(first)
	if firstTurnID == "" {
		t.Fatal("Codex did not report a provider turn ID for the initial prompt")
	}

	if err := adpt.LoadSession(ctx, threadID, nil); err != nil {
		t.Fatalf("resume Codex thread %q: %v", threadID, err)
	}
	resumed := collectNativeCodexTurn(ctx, t, mgr, adpt, "Read codex-app-server-e2e.txt and confirm its exact contents.", 2)
	AssertNoErrors(t, resumed)
	resumedTurnID := nativeCompletedTurnID(resumed)
	if resumedTurnID == "" || resumedTurnID == firstTurnID {
		t.Fatalf("resume turn ID = %q, initial turn ID = %q", resumedTurnID, firstTurnID)
	}

	forker, ok := adpt.(nativeSessionForker)
	if !ok {
		t.Fatal("Codex app-server adapter does not expose native conversation forks")
	}
	forkedThreadID, err := forker.ForkSession(ctx, threadID, resumedTurnID)
	if err != nil {
		t.Fatalf("fork idle Codex thread %q through completed turn %q: %v", threadID, resumedTurnID, err)
	}
	if forkedThreadID == "" || forkedThreadID == threadID {
		t.Fatalf("fork returned invalid thread identity %q (source %q)", forkedThreadID, threadID)
	}
	if err := adpt.LoadSession(ctx, forkedThreadID, nil); err != nil {
		t.Fatalf("resume forked Codex thread %q: %v", forkedThreadID, err)
	}
	forked := collectNativeCodexTurn(ctx, t, mgr, adpt, "Confirm the file exists in this fork and report its exact contents.", 3)
	AssertNoErrors(t, forked)

	// Ask for optional collaboration and a detached command so the capture can
	// show whether this exact binary emitted those events. Their absence is
	// recorded as unavailable evidence, not treated as a protocol failure.
	if err := adpt.LoadSession(ctx, threadID, nil); err != nil {
		t.Fatalf("resume source Codex thread %q for the activity probe: %v", threadID, err)
	}
	approvalDir, err := os.MkdirTemp("", "kandev-native-codex-approval-")
	if err != nil {
		t.Fatalf("create disposable approval target: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(approvalDir) })
	approval := collectNativeCodexTurn(ctx, t, mgr, adpt,
		fmt.Sprintf("Create %s/approval-probe.txt with exactly the text native Codex approval probe.", approvalDir), 4)
	AssertNoErrors(t, approval)
	approvalCount := CountEventsByType(approval)[adapter.EventTypePermissionRequest]
	if approvalCount == 0 {
		t.Log("Codex did not request approval for the disposable out-of-workspace write")
	} else {
		t.Logf("Codex requested approval for the disposable out-of-workspace write; resolved %d request(s)", approvalCount)
	}
	if _, err := os.Stat(filepath.Join(approvalDir, "approval-probe.txt")); err == nil {
		t.Log("Codex completed the disposable out-of-workspace write")
	}
	background := collectNativeCodexTurn(ctx, t, mgr, adpt,
		"Start a small read-only subagent task and a background `sleep 3` command, then answer immediately. Do not wait for either to finish.", 5)
	AssertNoErrors(t, background)
	backgroundTurnID := nativeCompletedTurnID(background)
	if backgroundTurnID == "" {
		t.Fatal("Codex did not report a provider turn ID for the activity probe")
	}
	postParentEvents := drainNativeCodexEvents(ctx, t, mgr, 15*time.Second)
	activityEvents := append(append([]adapter.AgentEvent(nil), background...), postParentEvents...)
	t.Logf("native activity lifecycle: thread=%s parent_turn=%s turns=%v activity=%v", threadID, backgroundTurnID, nativeTurnLifecycle(activityEvents), nativeActivitySummary(activityEvents))

	allEvents := append(append(append(first, resumed...), forked...), approval...)
	allEvents = append(allEvents, background...)
	allEvents = append(allEvents, postParentEvents...)
	allEvents = append(allEvents, forked...)
	counts := CountEventsByType(allEvents)
	t.Logf("Codex 0.154.0 completed init, prompt, resume and fork in %s; event counts: %v", time.Since(started), counts)
	if counts[adapter.EventTypePermissionRequest] > 0 {
		t.Log("Codex emitted an approval request and the test resolved it through the process manager")
	} else {
		t.Log("Codex approval request was not observed during this run")
	}
	if counts["usage_observation"] == 0 {
		t.Log("Codex response usage observations were not available during this run; native response usage is unavailable evidence")
	} else {
		for _, event := range allEvents {
			if event.UsageObservation != nil {
				t.Logf("native usage observation: source=%s scope=%s completeness=%s response_id_present=%t", event.UsageObservation.Source, event.UsageObservation.Scope, event.UsageObservation.Completeness, event.UsageObservation.ProviderResponseID != "")
			}
		}
	}
	if counts["background_complete"] == 0 {
		t.Log("Codex did not emit a background completion during this run")
	}
}

func nativeCodexPreflight() error {
	if strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) != "" {
		return nil
	}
	userHome, err := os.UserHomeDir()
	if err == nil {
		if info, statErr := os.Stat(filepath.Join(userHome, ".codex", "auth.json")); statErr == nil && !info.IsDir() {
			return nil
		}
	}
	return errors.New("Codex app-server E2E was explicitly enabled, but authentication is missing. Set OPENAI_API_KEY or sign in with `codex login` for the user running this test.")
}

func collectNativeCodexTurn(
	ctx context.Context,
	t *testing.T,
	mgr *process.Manager,
	adpt adapter.AgentAdapter,
	prompt string,
	generation uint64,
) []adapter.AgentEvent {
	t.Helper()
	if err := adpt.Prompt(ctx, prompt, nil, generation); err != nil {
		t.Fatalf("send Codex prompt: %v", err)
	}
	var events []adapter.AgentEvent
	for {
		select {
		case event, ok := <-mgr.GetUpdates():
			if !ok {
				t.Fatal("Codex event stream closed before turn completion")
			}
			events = append(events, event)
			if event.Type == adapter.EventTypePermissionRequest {
				resolveNativeCodexPermission(t, mgr, event)
			}
			if event.Type == adapter.EventTypeComplete && event.PromptGeneration == generation {
				return events
			}
		case <-ctx.Done():
			t.Fatalf("wait for Codex turn completion: %v", ctx.Err())
		}
	}
}

func drainNativeCodexEvents(
	ctx context.Context,
	t *testing.T,
	mgr *process.Manager,
	duration time.Duration,
) []adapter.AgentEvent {
	t.Helper()
	timer := time.NewTimer(duration)
	defer timer.Stop()
	var events []adapter.AgentEvent
	for {
		select {
		case event, ok := <-mgr.GetUpdates():
			if !ok {
				return events
			}
			events = append(events, event)
			if event.Type == adapter.EventTypePermissionRequest {
				resolveNativeCodexPermission(t, mgr, event)
			}
		case <-timer.C:
			return events
		case <-ctx.Done():
			return events
		}
	}
}

func resolveNativeCodexPermission(t *testing.T, mgr *process.Manager, event adapter.AgentEvent) {
	t.Helper()
	optionID := ""
	for _, option := range event.PermissionOptions {
		if option.Kind == "allow_once" || option.Kind == "allow_always" {
			optionID = option.OptionID
			break
		}
	}
	if optionID == "" {
		t.Fatalf("Codex approval request %q has no allow option", event.RequestID)
	}
	if _, err := mgr.ResolvePermission(event.RequestID, event.PendingID, optionID); err != nil {
		t.Fatalf("resolve Codex approval request %q: %v", event.RequestID, err)
	}
}

func nativeCompletedTurnID(events []adapter.AgentEvent) string {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type == adapter.EventTypeComplete && events[i].OperationID != "" {
			return events[i].OperationID
		}
	}
	return ""
}

func nativeTurnLifecycle(events []adapter.AgentEvent) []string {
	var lifecycle []string
	for _, event := range events {
		if event.Type == "turn_started" || event.Type == adapter.EventTypeComplete {
			lifecycle = append(lifecycle, fmt.Sprintf("%s session=%s turn=%s generation=%d", event.Type, event.SessionID, event.OperationID, event.PromptGeneration))
		}
	}
	return lifecycle
}

func nativeActivitySummary(events []adapter.AgentEvent) []string {
	var summary []string
	for _, event := range events {
		switch event.Type {
		case adapter.EventTypeToolCall, adapter.EventTypeToolUpdate, streams.EventTypeBackgroundComplete:
			summary = append(summary, fmt.Sprintf("%s session=%s tool=%s status=%s call=%s", event.Type, event.SessionID, event.ToolName, event.ToolStatus, event.ToolCallID))
		}
	}
	return summary
}
