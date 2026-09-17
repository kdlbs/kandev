package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

func TestAgentReplaceRejectsSuspiciousReductionBeforeWrite(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-plan-safe-replace")
	large := strings.Repeat("x", 4000)
	if _, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-plan-safe-replace", Content: large}); err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	if _, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
		TaskID:             "task-plan-safe-replace",
		Content:            strings.Repeat("y", 800),
		CreatedBy:          createdByAgent,
		EvaluateTruncation: true,
		Mode:               PlanWriteModeReplace,
		AgentWrite:         true,
	}); err == nil {
		t.Fatal("agent replacement accepted a suspicious reduction without a version and acknowledgement")
	}
}

func TestAgentReplaceRequiresAndChecksCurrentVersion(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-plan-safe-version")
	created, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-plan-safe-version", Content: "before"})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	cases := map[string]struct {
		expected string
		code     string
	}{
		"missing version": {expected: "", code: PlanErrorVersionRequired},
		"stale version":   {expected: "stale-version", code: PlanErrorVersionConflict},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
				TaskID: "task-plan-safe-version", Content: "after", CreatedBy: createdByAgent,
				AgentWrite: true, ExpectedVersion: tc.expected, Mode: PlanWriteModeReplace,
			})
			if err == nil {
				t.Fatal("agent replacement succeeded without the current version")
			}
			var safety *PlanSafetyError
			if !errors.As(err, &safety) {
				t.Fatalf("error = %T %v, want PlanSafetyError", err, err)
			}
			if safety.Code != tc.code {
				t.Fatalf("safety code = %q, want %q", safety.Code, tc.code)
			}
			if plan, getErr := svc.GetPlan(ctx, "task-plan-safe-version"); getErr != nil || plan.Content != "before" {
				t.Fatalf("plan after rejected replacement = %q, %v; want before", plan.Content, getErr)
			}
		})
	}
	if created.Plan.WriteVersion == "" {
		t.Fatal("initial write did not return a version")
	}
}

func TestAgentReplaceCanAcknowledgeReductionAfterHistoryCheck(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-plan-safe-ack")
	large := strings.Repeat("x", 4000)
	created, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-plan-safe-ack", Content: large})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	small := strings.Repeat("y", 800)
	updated, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
		TaskID: "task-plan-safe-ack", Content: small, CreatedBy: createdByAgent,
		AgentWrite: true, ExpectedVersion: created.Plan.WriteVersion,
		AllowTruncation: true, EvaluateTruncation: true, Mode: PlanWriteModeReplace,
	})
	if err != nil {
		t.Fatalf("acknowledged replacement: %v", err)
	}
	if updated.Plan.Content != small || updated.Plan.WriteVersion == created.Plan.WriteVersion {
		t.Fatalf("updated plan = content %q/version %q, want new acknowledged content/version", updated.Plan.Content, updated.Plan.WriteVersion)
	}
	revisions, err := svc.ListRevisions(ctx, "task-plan-safe-ack")
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(revisions) != 2 || revisions[1].Content != large {
		t.Fatalf("revisions = %d, contents %q/%q; want separate full predecessor and replacement", len(revisions), revisions[0].Content, revisions[1].Content)
	}
}

func TestAgentAppendKeepsOptionalVersion(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-plan-safe-append")
	if _, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-plan-safe-append", Content: "before"}); err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	updated, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
		TaskID: "task-plan-safe-append", Content: "after", CreatedBy: createdByAgent,
		AgentWrite: true, Mode: PlanWriteModeAppend,
	})
	if err != nil {
		t.Fatalf("append without expected version: %v", err)
	}
	if updated.Plan.Content != "before\n\nafter" {
		t.Fatalf("appended content = %q, want composed plan", updated.Plan.Content)
	}
}

func TestAgentInitialCreationRaceGuardsSecondWriter(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	taskID := "task-plan-safe-create-race"
	seedTask(t, ctx, repo, taskID)

	var mu sync.Mutex
	entered := 0
	ready := make(chan struct{})
	svc.SetTaskAuthorizer(func(context.Context, string) error {
		mu.Lock()
		entered++
		if entered == 2 {
			close(ready)
		}
		mu.Unlock()
		<-ready
		return nil
	})

	type attempt struct {
		result PlanWriteResult
		err    error
	}
	results := make(chan attempt, 2)
	for _, content := range []string{"first", "second"} {
		go func(content string) {
			result, err := svc.CreatePlan(ctx, CreatePlanRequest{
				TaskID: taskID, Content: content, AgentWrite: true,
			})
			results <- attempt{result: result, err: err}
		}(content)
	}

	var success attempt
	conflicts := 0
	for i := 0; i < 2; i++ {
		out := <-results
		if out.err == nil {
			success = out
			continue
		}
		var safety *PlanSafetyError
		if !errors.As(out.err, &safety) || safety.Code != PlanErrorVersionRequired {
			t.Fatalf("failed concurrent create = %T %v, want a version-required rejection", out.err, out.err)
		}
		conflicts++
	}
	if success.result.Plan == nil || success.result.Plan.WriteVersion == "" || conflicts != 1 {
		t.Fatalf("concurrent creates = success %#v/rejections %d, want one success and one rejection", success.result, conflicts)
	}
	plan, err := svc.GetPlan(ctx, taskID)
	if err != nil {
		t.Fatalf("GetPlan: %v", err)
	}
	if plan.Content != success.result.Plan.Content || plan.WriteVersion != success.result.Plan.WriteVersion {
		t.Fatalf("stored plan = %q/%q, want winning create %q/%q", plan.Content, plan.WriteVersion, success.result.Plan.Content, success.result.Plan.WriteVersion)
	}
}

func TestAgentReplacementWithSameVersionAdmitsOneConcurrentWriter(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	taskID := "task-plan-safe-update-race"
	seedTask(t, ctx, repo, taskID)
	created, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: taskID, Content: "before"})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	var mu sync.Mutex
	entered := 0
	ready := make(chan struct{})
	svc.SetTaskAuthorizer(func(context.Context, string) error {
		mu.Lock()
		entered++
		if entered == 2 {
			close(ready)
		}
		mu.Unlock()
		<-ready
		return nil
	})

	type attempt struct {
		result PlanWriteResult
		err    error
	}
	results := make(chan attempt, 2)
	for _, content := range []string{"first update", "second update"} {
		go func(content string) {
			result, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
				TaskID: taskID, Content: content, CreatedBy: createdByAgent,
				AgentWrite: true, ExpectedVersion: created.Plan.WriteVersion,
				Mode: PlanWriteModeReplace,
			})
			results <- attempt{result: result, err: err}
		}(content)
	}

	var success attempt
	conflicts := 0
	for i := 0; i < 2; i++ {
		out := <-results
		if out.err == nil {
			success = out
			continue
		}
		var safety *PlanSafetyError
		if !errors.As(out.err, &safety) || safety.Code != PlanErrorVersionConflict {
			t.Fatalf("failed concurrent update = %T %v, want a version conflict", out.err, out.err)
		}
		conflicts++
	}
	if success.result.Plan == nil || success.result.Plan.WriteVersion == created.Plan.WriteVersion || conflicts != 1 {
		t.Fatalf("concurrent updates = success %#v/conflicts %d, want one fresh success and one conflict", success.result, conflicts)
	}
	plan, err := svc.GetPlan(ctx, taskID)
	if err != nil {
		t.Fatalf("GetPlan: %v", err)
	}
	if plan.Content != success.result.Plan.Content || plan.WriteVersion != success.result.Plan.WriteVersion {
		t.Fatalf("stored plan = %q/%q, want winning update %q/%q", plan.Content, plan.WriteVersion, success.result.Plan.Content, success.result.Plan.WriteVersion)
	}
}
