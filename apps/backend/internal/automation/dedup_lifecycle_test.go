package automation

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// These tests cover the webhook dedup-key lifecycle: MarkRunTerminal's
// task-id-aware blanking rule (see the doc comment on Store.MarkRunTerminal),
// the duplicate-skip audit record, and the DedupReason persisted for the two
// non-resolved DedupBinding dispositions.

func newWebhookAutomation(t *testing.T, svc *Service, name string) *Automation {
	t.Helper()
	a := &Automation{WorkspaceID: "ws-1", Name: name, Enabled: true}
	require.NoError(t, svc.store.CreateAutomation(context.Background(), a))
	return a
}

// A run that fails after BindRunTask has already recorded a task must keep
// its dedup key — the firing produced something, so a retried delivery of
// the same alert must not be re-admitted.
func TestMarkRunTerminal_FailedAfterTaskBound_RetainsDedupKey(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	a := newWebhookAutomation(t, svc, "retains-after-bind")

	run := &AutomationRun{
		AutomationID: a.ID, TriggerType: TriggerTypeWebhook, Status: RunStatusTriggered,
		DedupKey: "webhook:alert-1", TriggerData: json.RawMessage(`{}`),
	}
	require.NoError(t, svc.store.CreateRun(ctx, run))
	require.NoError(t, svc.store.BindRunTask(ctx, run.ID, "task-1", ""))

	require.NoError(t, svc.store.MarkRunTerminal(ctx, run.ID, "", "", RunStatusFailed, "dispatch failed"))

	stored, err := svc.store.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, "webhook:alert-1", stored.DedupKey, "a run that already bound a task must keep its dedup key")
	require.Equal(t, RunStatusFailed, stored.Status)
}

// A run that fails before any task ever existed must release its dedup key
// so a retried delivery of the same alert isn't permanently blocked by a
// firing that produced nothing.
func TestMarkRunTerminal_FailedBeforeTaskExists_ReleasesDedupKey(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	a := newWebhookAutomation(t, svc, "releases-before-bind")

	run := &AutomationRun{
		AutomationID: a.ID, TriggerType: TriggerTypeWebhook, Status: RunStatusTriggered,
		DedupKey: "webhook:alert-2", TriggerData: json.RawMessage(`{}`),
	}
	require.NoError(t, svc.store.CreateRun(ctx, run))

	require.NoError(t, svc.store.MarkRunTerminal(ctx, run.ID, "", "", RunStatusFailed, "publish failed"))

	stored, err := svc.store.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Empty(t, stored.DedupKey, "a run that never bound a task must release its dedup key")
}

// github_pr_merged must be byte-identical to its pre-existing behavior: the
// webhook-scoped CASE in MarkRunTerminal must never touch its dedup key,
// bound or not.
func TestMarkRunTerminal_GitHubPRMerged_NeverBlanksDedupKey(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	a := newWebhookAutomation(t, svc, "pr-merged-untouched")

	run := &AutomationRun{
		AutomationID: a.ID, TriggerType: TriggerTypeGitHubPRMerged, Status: RunStatusTriggered,
		DedupKey: "pr_merged:task-1:acme/api#7", TriggerData: json.RawMessage(`{}`),
	}
	require.NoError(t, svc.store.CreateRun(ctx, run))

	require.NoError(t, svc.store.MarkRunTerminal(ctx, run.ID, "", "", RunStatusFailed, "publish failed"))

	stored, err := svc.store.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, "pr_merged:task-1:acme/api#7", stored.DedupKey,
		"github_pr_merged's MarkRunTerminal behavior must stay unchanged by the webhook blanking rule")
}

// A run stopped by the user after BindRunTask retains its dedup key, via the
// real StopRun production path (not a direct store call).
func TestStopRun_AfterTaskBound_RetainsDedupKey(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	a := newWebhookAutomation(t, svc, "stop-after-bind")

	run := &AutomationRun{
		AutomationID: a.ID, TriggerType: TriggerTypeWebhook, Status: RunStatusTaskCreated,
		DedupKey: "webhook:alert-3", TaskID: "task-x", SessionID: "session-x", TurnID: "turn-x",
		TriggerData: json.RawMessage(`{}`),
	}
	require.NoError(t, svc.store.CreateRun(ctx, run))
	svc.SetRunStopper(runStopperStub{stopped: true})

	got, err := svc.StopRun(ctx, a.ID, run.ID)
	require.NoError(t, err)
	require.Equal(t, RunStatusFailed, got.Status)

	stored, err := svc.store.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, "webhook:alert-3", stored.DedupKey, "a stopped run that already had a bound task must keep its dedup key")
}

// Two webhook POSTs resolving to the same dedup key must result in exactly
// one task-eligible (triggered) run and one recorded duplicate-skip audit
// row — not a silent no-op on the second delivery.
func TestFireTrigger_DuplicateWebhookDedupKey_RecordsOneSkip(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	a := newWebhookAutomation(t, svc, "duplicate-skip")
	trig := &AutomationTrigger{AutomationID: a.ID, Type: TriggerTypeWebhook, Config: json.RawMessage(`{}`), Enabled: true}
	require.NoError(t, svc.store.CreateTrigger(ctx, trig))

	first, err := svc.FireTrigger(ctx, a.ID, trig.ID, TriggerTypeWebhook, json.RawMessage(`{"a":1}`), DedupKey("webhook:same"))
	require.NoError(t, err)
	require.False(t, first.Skipped)

	second, err := svc.FireTrigger(ctx, a.ID, trig.ID, TriggerTypeWebhook, json.RawMessage(`{"a":1}`), DedupKey("webhook:same"))
	require.NoError(t, err)
	require.True(t, second.Skipped)

	runs, err := svc.store.ListRuns(ctx, a.ID, 10)
	require.NoError(t, err)
	require.Len(t, runs, 2)

	var skipped, admitted int
	for _, r := range runs {
		switch r.Status {
		case RunStatusSkipped:
			skipped++
			require.Equal(t, "duplicate trigger: dedup key already fired", r.ErrorMessage)
			require.Equal(t, "webhook:same", r.DedupKey)
		case RunStatusTriggered:
			admitted++
		}
	}
	require.Equal(t, 1, skipped, "expected exactly one recorded duplicate-skip run")
	require.Equal(t, 1, admitted, "expected exactly one admitted run")
}

// A webhook trigger declaring no dedup_key path at all behaves as it always
// has: no key is stored, and the admitted run's DedupReason records why.
func TestFireTrigger_NoDedupKeyConfigured_AdmitsWithEmptyKeyAndReason(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	a := newWebhookAutomation(t, svc, "no-dedup-configured")
	trig := &AutomationTrigger{AutomationID: a.ID, Type: TriggerTypeWebhook, Config: json.RawMessage(`{}`), Enabled: true}
	require.NoError(t, svc.store.CreateTrigger(ctx, trig))

	result, err := svc.FireTrigger(ctx, a.ID, trig.ID, TriggerTypeWebhook, json.RawMessage(`{}`), DedupNotConfigured())
	require.NoError(t, err)
	require.False(t, result.Skipped)

	stored, err := svc.store.GetRun(ctx, result.RunID)
	require.NoError(t, err)
	require.Empty(t, stored.DedupKey)
	require.Equal(t, "dedup_not_configured", stored.DedupReason)
}

// A webhook trigger that declares a dedup_key path which doesn't resolve
// against the payload also admits (dedup is advisory, not a filter) and
// records why the key is empty.
func TestFireTrigger_DedupKeyUnresolved_AdmitsWithEmptyKeyAndReason(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	a := newWebhookAutomation(t, svc, "dedup-unresolved")
	trig := &AutomationTrigger{AutomationID: a.ID, Type: TriggerTypeWebhook, Config: json.RawMessage(`{"dedup_key":"missing.path"}`), Enabled: true}
	require.NoError(t, svc.store.CreateTrigger(ctx, trig))

	result, err := svc.FireTrigger(ctx, a.ID, trig.ID, TriggerTypeWebhook, json.RawMessage(`{"other":"x"}`), DedupUnresolved())
	require.NoError(t, err)
	require.False(t, result.Skipped)

	stored, err := svc.store.GetRun(ctx, result.RunID)
	require.NoError(t, err)
	require.Empty(t, stored.DedupKey)
	require.Equal(t, "dedup_unresolved", stored.DedupReason)
}
