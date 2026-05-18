package lifecycle

import (
	"strings"
	"testing"
)

func TestSpritesProgressPlanGrowsOnFallback(t *testing.T) {
	plan := newSpritesProgressPlan(true)
	if plan.total() != 3 {
		t.Fatalf("reconnect plan total = %d, want 3", plan.total())
	}

	plan.replacePlan([]spritesStepKey{
		spriteStepCreateSprite,
		spriteStepUploadAgentctl,
		spriteStepUploadCredentials,
		spriteStepRunPrepareScript,
		spriteStepWaitHealthy,
		spriteStepAgentInstance,
		spriteStepApplyNetworkPolicy,
	})

	if plan.total() != 7 {
		t.Fatalf("after fallback, plan total = %d, want 7", plan.total())
	}
	for _, key := range []spritesStepKey{
		spriteStepUploadAgentctl,
		spriteStepUploadCredentials,
		spriteStepRunPrepareScript,
		spriteStepApplyNetworkPolicy,
	} {
		if !plan.has(key) {
			t.Fatalf("after fallback, plan should include %s", key)
		}
	}
}

func TestSpritesStepReporterFollowsPlanReplacement(t *testing.T) {
	plan := newSpritesProgressPlan(true)
	type reportEvent struct {
		index int
		total int
	}
	var events []reportEvent
	cb := func(_ PrepareStep, idx, total int) {
		events = append(events, reportEvent{index: idx, total: total})
	}
	report := newSpritesStepReporter(cb, plan)

	report(spriteStepWaitHealthy, beginStep("Waiting for agent controller"))
	if len(events) != 1 || events[0].index != 1 || events[0].total != 3 {
		t.Fatalf("pre-replace event = %+v, want index 1 of 3", events)
	}

	plan.replacePlan([]spritesStepKey{
		spriteStepCreateSprite,
		spriteStepUploadAgentctl,
		spriteStepUploadCredentials,
		spriteStepRunPrepareScript,
		spriteStepWaitHealthy,
		spriteStepAgentInstance,
		spriteStepApplyNetworkPolicy,
	})
	report(spriteStepWaitHealthy, beginStep("Waiting for agent controller"))
	if len(events) != 2 || events[1].index != 4 || events[1].total != 7 {
		t.Fatalf("post-replace event = %+v, want index 4 of 7", events)
	}
}

func TestSpritesFallbackToFreshSandboxEmitsWarningAndMutatesPlan(t *testing.T) {
	r := newTestSpritesExecutor(nil)
	plan := newSpritesProgressPlan(true)

	var emitted []PrepareStep
	report := func(_ spritesStepKey, step PrepareStep) {
		emitted = append(emitted, step)
	}

	req := &ExecutorCreateRequest{
		InstanceID: "abcdef0123456789aaaa",
		Metadata: map[string]interface{}{
			MetadataKeyWorktreeBranch: "feature/something",
			"sprite_name":             "kandev-old1234567890",
		},
	}

	newName := r.fallbackToFreshSandbox(req, plan, report, "kandev-old1234567890")

	wantName := spritesNamePrefix + req.InstanceID[:12]
	if newName != wantName {
		t.Fatalf("new sprite name = %q, want %q", newName, wantName)
	}
	if plan.total() != 7 {
		t.Fatalf("plan total after fallback = %d, want 7", plan.total())
	}
	if len(emitted) != 1 {
		t.Fatalf("expected exactly 1 progress event from fallback, got %d", len(emitted))
	}

	notice := emitted[0]
	if notice.Status != PrepareStepSkipped {
		t.Fatalf("notice status = %s, want skipped", notice.Status)
	}
	if !strings.Contains(notice.Warning, "Previous sandbox is no longer available") {
		t.Fatalf("notice warning missing user-facing copy: %q", notice.Warning)
	}
	if !strings.Contains(notice.WarningDetail, "kandev-old1234567890") ||
		!strings.Contains(notice.WarningDetail, wantName) ||
		!strings.Contains(notice.WarningDetail, "feature/something") {
		t.Fatalf("notice detail missing context: %q", notice.WarningDetail)
	}
	if !strings.Contains(notice.Output, "Old sandbox: kandev-old1234567890") ||
		!strings.Contains(notice.Output, "New sandbox: "+wantName) ||
		!strings.Contains(notice.Output, "Branch: feature/something") {
		t.Fatalf("notice output missing structured block: %q", notice.Output)
	}
}

func TestSpritesFallbackNamesBaseBranchInWarningDetail(t *testing.T) {
	r := newTestSpritesExecutor(nil)
	plan := newSpritesProgressPlan(true)
	var emitted []PrepareStep

	// After Fix 1 (preserve BaseBranch on resume for clone-based executors),
	// the fallback request always carries MetadataKeyBaseBranch — this test
	// locks in that the user-facing warning detail names the actual base
	// branch rather than the "(unknown)" placeholder.
	req := &ExecutorCreateRequest{
		InstanceID: "abcdef0123456789aaaa",
		Metadata: map[string]interface{}{
			MetadataKeyBaseBranch: "main",
		},
	}
	r.fallbackToFreshSandbox(req, plan, func(_ spritesStepKey, step PrepareStep) {
		emitted = append(emitted, step)
	}, "kandev-old1234567890")

	if len(emitted) != 1 {
		t.Fatalf("expected 1 event, got %d", len(emitted))
	}
	if !strings.Contains(emitted[0].WarningDetail, "branch main") {
		t.Fatalf("warning detail should name base branch, got %q", emitted[0].WarningDetail)
	}
	if !strings.Contains(emitted[0].Output, "Branch: main") {
		t.Fatalf("output should name base branch, got %q", emitted[0].Output)
	}
}

func TestSpritesFallbackUsesPlaceholderBranchWhenMissing(t *testing.T) {
	r := newTestSpritesExecutor(nil)
	plan := newSpritesProgressPlan(true)
	var emitted []PrepareStep

	req := &ExecutorCreateRequest{
		InstanceID: "abcdef0123456789aaaa",
		Metadata:   map[string]interface{}{},
	}
	r.fallbackToFreshSandbox(req, plan, func(_ spritesStepKey, step PrepareStep) {
		emitted = append(emitted, step)
	}, "kandev-old1234567890")

	if len(emitted) != 1 {
		t.Fatalf("expected 1 event, got %d", len(emitted))
	}
	if !strings.Contains(emitted[0].Output, "Branch: (unknown)") {
		t.Fatalf("expected placeholder branch in output, got %q", emitted[0].Output)
	}
}
