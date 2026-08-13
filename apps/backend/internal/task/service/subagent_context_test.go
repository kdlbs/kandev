package service

import (
	"context"
	"errors"
	"expvar"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	commonlogger "github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

// stubSubagentContextRepo is a minimal repository.SubagentContextRepository
// double: it records every upsert it receives (or returns a fixed error),
// with no other behavior.
type stubSubagentContextRepo struct {
	mu      sync.Mutex
	upserts []*models.SubagentContext
	err     error
}

func (s *stubSubagentContextRepo) UpsertSubagentContext(_ context.Context, sc *models.SubagentContext) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.upserts = append(s.upserts, sc)
	return nil
}

func (s *stubSubagentContextRepo) ListSubagentContextsBySession(context.Context, string) ([]*models.SubagentContext, error) {
	return nil, nil
}

func (s *stubSubagentContextRepo) ListSubagentContextsByTurn(context.Context, string) ([]*models.SubagentContext, error) {
	return nil, nil
}

func (s *stubSubagentContextRepo) last(t *testing.T) *models.SubagentContext {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.upserts) == 0 {
		t.Fatal("no subagent context was upserted")
	}
	return s.upserts[len(s.upserts)-1]
}

func (s *stubSubagentContextRepo) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.upserts)
}

func newSubagentContextTestService(t *testing.T, repo *stubSubagentContextRepo) *Service {
	t.Helper()
	log, err := commonlogger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	return &Service{subagentContexts: repo, logger: log}
}

func readSubagentCounter(t *testing.T, key string) int64 {
	t.Helper()
	v := subagentContextTotal.Get(key)
	if v == nil {
		return 0
	}
	iv, ok := v.(*expvar.Int)
	if !ok {
		t.Fatalf("counter %q is %T, want *expvar.Int", key, v)
	}
	return iv.Value()
}

// counterDelta runs fn and returns how much the named subagentContextTotal
// counter moved. Safe against cross-test accumulation on the shared
// package-level expvar.Map because it snapshots before and after.
func counterDelta(t *testing.T, key string, fn func()) int64 {
	t.Helper()
	before := readSubagentCounter(t, key)
	fn()
	return readSubagentCounter(t, key) - before
}

func intptr(n int) *int { return &n }

// TestRecordSubagentContextIdentityGate covers AC-2: each of an empty
// session id, task id, or tool_call_id skips the write entirely and
// increments skipped_no_identity by exactly one.
func TestRecordSubagentContextIdentityGate(t *testing.T) {
	cases := []struct {
		name string
		req  RecordSubagentContextRequest
	}{
		{"missing_session_id", RecordSubagentContextRequest{
			TaskSessionID: "", TaskID: "task-1", ToolCallID: "tc-1",
			Payload: &streams.SubagentTaskPayload{},
		}},
		{"missing_task_id", RecordSubagentContextRequest{
			TaskSessionID: "session-1", TaskID: "", ToolCallID: "tc-1",
			Payload: &streams.SubagentTaskPayload{},
		}},
		{"missing_tool_call_id", RecordSubagentContextRequest{
			TaskSessionID: "session-1", TaskID: "task-1", ToolCallID: "",
			Payload: &streams.SubagentTaskPayload{},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &stubSubagentContextRepo{}
			svc := newSubagentContextTestService(t, repo)

			delta := counterDelta(t, "skipped_no_identity", func() {
				svc.RecordSubagentContext(context.Background(), tc.req)
			})
			if delta != 1 {
				t.Errorf("skipped_no_identity delta = %d, want 1", delta)
			}
			if repo.count() != 0 {
				t.Errorf("repository call count = %d, want 0 (no write)", repo.count())
			}
		})
	}
}

// TestRecordSubagentContextNilPayloadIsNoop is a defensive guard: the
// orchestrator only calls this after confirming Normalized.SubagentTask() is
// non-nil, but a nil payload must still be handled without a panic or a
// write.
func TestRecordSubagentContextNilPayloadIsNoop(t *testing.T) {
	repo := &stubSubagentContextRepo{}
	svc := newSubagentContextTestService(t, repo)

	svc.RecordSubagentContext(context.Background(), RecordSubagentContextRequest{
		TaskSessionID: "session-1", TaskID: "task-1", ToolCallID: "tc-1", Payload: nil,
	})
	if repo.count() != 0 {
		t.Errorf("repository call count = %d, want 0", repo.count())
	}
}

// TestRecordSubagentContextEmptyStringsNormalizeToNil covers the
// empty-string-to-NULL normalization rule for every nullable TEXT field.
func TestRecordSubagentContextEmptyStringsNormalizeToNil(t *testing.T) {
	repo := &stubSubagentContextRepo{}
	svc := newSubagentContextTestService(t, repo)

	svc.RecordSubagentContext(context.Background(), RecordSubagentContextRequest{
		TaskSessionID: "session-1", TaskID: "task-1", ToolCallID: "tc-1",
		TurnID: "", ParentToolCallID: "", ToolStatus: "",
		Payload: &streams.SubagentTaskPayload{
			SubagentType: "", Description: "", AgentID: "", ChildSessionID: "", Model: "", Status: "",
		},
		ObservedAt: time.Now().UTC(),
	})

	sc := repo.last(t)
	for name, field := range map[string]*string{
		"turn_id": sc.TurnID, "parent_tool_call_id": sc.ParentToolCallID,
		"subagent_type": sc.SubagentType, "description": sc.Description,
		"agent_id": sc.AgentID, "child_session_id": sc.ChildSessionID,
		"model": sc.Model, "agent_status": sc.AgentStatus, "tool_status": sc.ToolStatus,
	} {
		if field != nil {
			t.Errorf("%s = %q, want NULL for an empty string", name, *field)
		}
	}
}

// TestRecordSubagentContextMetricNormalization covers AC-7, AC-8, AC-9: an
// absent metric is nil, a reported zero ToolUseCount survives as 0, and a
// negative value becomes nil while incrementing anomalous_value and leaving
// the rest of the frame intact.
func TestRecordSubagentContextMetricNormalization(t *testing.T) {
	t.Run("absent_metrics_are_nil", func(t *testing.T) {
		repo := &stubSubagentContextRepo{}
		svc := newSubagentContextTestService(t, repo)
		svc.RecordSubagentContext(context.Background(), RecordSubagentContextRequest{
			TaskSessionID: "session-1", TaskID: "task-1", ToolCallID: "tc-1",
			Payload:    &streams.SubagentTaskPayload{TotalTokens: 0, DurationMs: 0, ToolUseCount: nil},
			ObservedAt: time.Now().UTC(),
		})
		sc := repo.last(t)
		if sc.TotalTokens != nil || sc.DurationMs != nil || sc.ToolUseCount != nil {
			t.Errorf("metrics = tokens=%v duration=%v count=%v, want all nil", sc.TotalTokens, sc.DurationMs, sc.ToolUseCount)
		}
	})

	t.Run("reported_zero_tool_use_count_survives", func(t *testing.T) {
		repo := &stubSubagentContextRepo{}
		svc := newSubagentContextTestService(t, repo)
		svc.RecordSubagentContext(context.Background(), RecordSubagentContextRequest{
			TaskSessionID: "session-1", TaskID: "task-1", ToolCallID: "tc-1",
			Payload:    &streams.SubagentTaskPayload{ToolUseCount: intptr(0)},
			ObservedAt: time.Now().UTC(),
		})
		sc := repo.last(t)
		if sc.ToolUseCount == nil || *sc.ToolUseCount != 0 {
			t.Errorf("tool_use_count = %v, want *0", sc.ToolUseCount)
		}
	})

	t.Run("negative_total_tokens_becomes_nil_others_intact", func(t *testing.T) {
		repo := &stubSubagentContextRepo{}
		svc := newSubagentContextTestService(t, repo)
		delta := counterDelta(t, "anomalous_value", func() {
			svc.RecordSubagentContext(context.Background(), RecordSubagentContextRequest{
				TaskSessionID: "session-1", TaskID: "task-1", ToolCallID: "tc-1",
				Payload: &streams.SubagentTaskPayload{
					TotalTokens: -5, DurationMs: 1200, Description: "kept",
				},
				ObservedAt: time.Now().UTC(),
			})
		})
		if delta != 1 {
			t.Errorf("anomalous_value delta = %d, want 1", delta)
		}
		sc := repo.last(t)
		if sc.TotalTokens != nil {
			t.Errorf("total_tokens = %v, want nil", sc.TotalTokens)
		}
		if sc.DurationMs == nil || *sc.DurationMs != 1200 {
			t.Errorf("duration_ms = %v, want 1200 (unaffected)", sc.DurationMs)
		}
		if sc.Description == nil || *sc.Description != "kept" {
			t.Errorf("description = %v, want kept (unaffected)", sc.Description)
		}
	})

	t.Run("negative_duration_ms_becomes_nil", func(t *testing.T) {
		repo := &stubSubagentContextRepo{}
		svc := newSubagentContextTestService(t, repo)
		delta := counterDelta(t, "anomalous_value", func() {
			svc.RecordSubagentContext(context.Background(), RecordSubagentContextRequest{
				TaskSessionID: "session-1", TaskID: "task-1", ToolCallID: "tc-1",
				Payload:    &streams.SubagentTaskPayload{DurationMs: -1},
				ObservedAt: time.Now().UTC(),
			})
		})
		if delta != 1 {
			t.Errorf("anomalous_value delta = %d, want 1", delta)
		}
		if repo.last(t).DurationMs != nil {
			t.Errorf("duration_ms = %v, want nil", repo.last(t).DurationMs)
		}
	})

	t.Run("negative_tool_use_count_becomes_nil", func(t *testing.T) {
		repo := &stubSubagentContextRepo{}
		svc := newSubagentContextTestService(t, repo)
		delta := counterDelta(t, "anomalous_value", func() {
			svc.RecordSubagentContext(context.Background(), RecordSubagentContextRequest{
				TaskSessionID: "session-1", TaskID: "task-1", ToolCallID: "tc-1",
				Payload:    &streams.SubagentTaskPayload{ToolUseCount: intptr(-3)},
				ObservedAt: time.Now().UTC(),
			})
		})
		if delta != 1 {
			t.Errorf("anomalous_value delta = %d, want 1", delta)
		}
		if repo.last(t).ToolUseCount != nil {
			t.Errorf("tool_use_count = %v, want nil", repo.last(t).ToolUseCount)
		}
	})
}

// TestRecordSubagentContextTerminality covers AC-10a/AC-11: settled_at is
// set only from a terminal ACP tool_status, never from agent_status —
// including the Claude-specific "async_launched" agent status, whose own
// tool call is nonetheless terminal in the ACP sense.
func TestRecordSubagentContextTerminality(t *testing.T) {
	observedAt := time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)

	t.Run("terminal_tool_status_sets_settled_at", func(t *testing.T) {
		repo := &stubSubagentContextRepo{}
		svc := newSubagentContextTestService(t, repo)
		svc.RecordSubagentContext(context.Background(), RecordSubagentContextRequest{
			TaskSessionID: "session-1", TaskID: "task-1", ToolCallID: "tc-1",
			ToolStatus: "completed", Payload: &streams.SubagentTaskPayload{Status: "async_launched"},
			ObservedAt: observedAt,
		})
		sc := repo.last(t)
		if sc.SettledAt == nil || !sc.SettledAt.Equal(observedAt) {
			t.Errorf("settled_at = %v, want %v", sc.SettledAt, observedAt)
		}
		if sc.AgentStatus == nil || *sc.AgentStatus != "async_launched" {
			t.Errorf("agent_status = %v, want async_launched stored verbatim", sc.AgentStatus)
		}
	})

	t.Run("async_launched_agent_status_alone_does_not_settle", func(t *testing.T) {
		repo := &stubSubagentContextRepo{}
		svc := newSubagentContextTestService(t, repo)
		svc.RecordSubagentContext(context.Background(), RecordSubagentContextRequest{
			TaskSessionID: "session-1", TaskID: "task-1", ToolCallID: "tc-1",
			ToolStatus: "in_progress", Payload: &streams.SubagentTaskPayload{Status: "async_launched"},
			ObservedAt: observedAt,
		})
		if repo.last(t).SettledAt != nil {
			t.Errorf("settled_at = %v, want nil (tool_status is non-terminal)", repo.last(t).SettledAt)
		}
	})

	t.Run("unrecognised_agent_status_stored_verbatim_and_does_not_settle", func(t *testing.T) {
		repo := &stubSubagentContextRepo{}
		svc := newSubagentContextTestService(t, repo)
		svc.RecordSubagentContext(context.Background(), RecordSubagentContextRequest{
			TaskSessionID: "session-1", TaskID: "task-1", ToolCallID: "tc-1",
			ToolStatus: "pending", Payload: &streams.SubagentTaskPayload{Status: "a-status-kandev-has-never-seen"},
			ObservedAt: observedAt,
		})
		sc := repo.last(t)
		if sc.AgentStatus == nil || *sc.AgentStatus != "a-status-kandev-has-never-seen" {
			t.Errorf("agent_status = %v, want stored verbatim", sc.AgentStatus)
		}
		if sc.SettledAt != nil {
			t.Errorf("settled_at = %v, want nil", sc.SettledAt)
		}
	})
}

// TestRecordSubagentContextRepositoryFailure covers AC-27: a repository
// error logs WARN with the session and tool-call id, increments failed, and
// the call still returns normally (no panic, no error surfaced to caller).
func TestRecordSubagentContextRepositoryFailure(t *testing.T) {
	repo := &stubSubagentContextRepo{err: errors.New("disk full")}
	core, logs := observer.New(zapcore.WarnLevel)
	log, err := commonlogger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	svc := &Service{subagentContexts: repo, logger: log}

	delta := counterDelta(t, "failed", func() {
		svc.RecordSubagentContext(context.Background(), RecordSubagentContextRequest{
			TaskSessionID: "session-fail", TaskID: "task-1", ToolCallID: "tc-fail",
			Payload: &streams.SubagentTaskPayload{}, ObservedAt: time.Now().UTC(),
		})
	})
	if delta != 1 {
		t.Errorf("failed delta = %d, want 1", delta)
	}
	entries := logs.FilterMessage("failed to record subagent context").All()
	if len(entries) != 1 {
		t.Fatalf("WARN log count = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["session_id"] != "session-fail" || fields["tool_call_id"] != "tc-fail" {
		t.Errorf("log fields = %#v, want session_id=session-fail tool_call_id=tc-fail", fields)
	}
}

// TestRecordSubagentContextHappyPathCounters covers AC-26: attempted and
// persisted both move by one on a successful write.
func TestRecordSubagentContextHappyPathCounters(t *testing.T) {
	repo := &stubSubagentContextRepo{}
	svc := newSubagentContextTestService(t, repo)

	var attemptedDelta, persistedDelta int64
	attemptedDelta = counterDelta(t, "attempted", func() {
		persistedDelta = counterDelta(t, "persisted", func() {
			svc.RecordSubagentContext(context.Background(), RecordSubagentContextRequest{
				TaskSessionID: "session-1", TaskID: "task-1", ToolCallID: "tc-1",
				Payload: &streams.SubagentTaskPayload{}, ObservedAt: time.Now().UTC(),
			})
		})
	})
	if attemptedDelta != 1 {
		t.Errorf("attempted delta = %d, want 1", attemptedDelta)
	}
	if persistedDelta != 1 {
		t.Errorf("persisted delta = %d, want 1", persistedDelta)
	}
	if repo.count() != 1 {
		t.Errorf("repository upsert count = %d, want 1", repo.count())
	}
}

// TestSubagentContextTerminalStatusesMatchOrchestrator pins
// subagentTerminalToolStatuses against the orchestrator's unexported peer
// isTerminalToolStatus (event_handlers_streaming.go:762). This package
// cannot import internal/orchestrator directly — orchestrator already
// imports internal/task/service for RecordSubagentContext, so that would be
// a cycle — so the peer's source is read and parsed instead.
func TestSubagentContextTerminalStatusesMatchOrchestrator(t *testing.T) {
	constsSrc, err := os.ReadFile("../../orchestrator/event_handlers.go")
	if err != nil {
		t.Fatalf("read orchestrator event_handlers.go: %v", err)
	}
	switchSrc, err := os.ReadFile("../../orchestrator/event_handlers_streaming.go")
	if err != nil {
		t.Fatalf("read orchestrator event_handlers_streaming.go: %v", err)
	}

	caseRe := regexp.MustCompile(`(?s)func isTerminalToolStatus\(status string\) bool \{.*?case (.*?):`)
	match := caseRe.FindSubmatch(switchSrc)
	if match == nil {
		t.Fatal("could not find isTerminalToolStatus's case clause in event_handlers_streaming.go")
	}
	tokens := strings.Split(string(match[1]), ",")

	constRe := regexp.MustCompile(`(\w+)\s*=\s*"([^"]+)"`)
	constValues := map[string]string{}
	for _, m := range constRe.FindAllSubmatch(constsSrc, -1) {
		constValues[string(m[1])] = string(m[2])
	}

	orchestratorSet := map[string]bool{}
	for _, tok := range tokens {
		tok = strings.TrimSpace(tok)
		if strings.HasPrefix(tok, `"`) {
			orchestratorSet[strings.Trim(tok, `"`)] = true
			continue
		}
		value, ok := constValues[tok]
		if !ok {
			t.Fatalf("orchestrator case token %q resolves to no known string constant", tok)
		}
		orchestratorSet[value] = true
	}

	if len(orchestratorSet) != len(subagentTerminalToolStatuses) {
		t.Fatalf("orchestrator terminal set = %v (%d), service set = %v (%d)",
			orchestratorSet, len(orchestratorSet), subagentTerminalToolStatuses, len(subagentTerminalToolStatuses))
	}
	for status := range subagentTerminalToolStatuses {
		if !orchestratorSet[status] {
			t.Errorf("service terminal set has %q, orchestrator's does not: %v", status, orchestratorSet)
		}
	}
}
