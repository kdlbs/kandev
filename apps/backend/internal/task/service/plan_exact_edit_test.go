package service

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestPlanExactEdit(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	taskID := "task-plan-exact-edit"
	seedTask(t, ctx, repo, taskID)
	created, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID:  taskID,
		Title:   "Checklist",
		Content: "before\r\n- [ ] preserve this\r\n- [ ] change this\r\nafter",
	})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	updated, err := svc.EditPlan(ctx, ExactPlanEditRequest{
		TaskID:          taskID,
		ExpectedVersion: created.Plan.WriteVersion,
		OldText:         "- [ ] change this",
		NewText:         "- [x] change this",
	})
	if err != nil {
		t.Fatalf("EditPlan: %v", err)
	}
	want := "before\r\n- [ ] preserve this\r\n- [x] change this\r\nafter"
	if updated.Plan.Content != want {
		t.Fatalf("edited content = %q, want %q", updated.Plan.Content, want)
	}
	if updated.Plan.Title != created.Plan.Title {
		t.Fatalf("edited title = %q, want %q", updated.Plan.Title, created.Plan.Title)
	}
	if updated.Plan.WriteVersion == created.Plan.WriteVersion {
		t.Fatal("exact edit did not rotate the write version")
	}

	history, err := svc.ListRevisions(ctx, taskID)
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(history) != 1 || history[0].Content != want {
		t.Fatalf("history = %#v, want the edited snapshot", history)
	}
}

func TestPlanExactEditRejectsInvalidMatchWithoutMutation(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	taskID := "task-plan-exact-invalid"
	seedTask(t, ctx, repo, taskID)
	created, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: taskID, Content: "aaa"})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	cases := []struct {
		name    string
		oldText string
		code    string
	}{
		{name: "empty", oldText: "", code: PlanErrorEditTextRequired},
		{name: "missing", oldText: "not present", code: PlanErrorEditNotFound},
		{name: "overlapping", oldText: "aa", code: PlanErrorEditAmbiguous},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.EditPlan(ctx, ExactPlanEditRequest{
				TaskID: taskID, ExpectedVersion: created.Plan.WriteVersion,
				OldText: tc.oldText, NewText: "replacement",
			})
			if err == nil {
				t.Fatal("EditPlan succeeded for invalid match")
			}
			var safety *PlanSafetyError
			if !errors.As(err, &safety) || safety.Code != tc.code {
				t.Fatalf("error = %T %v, want safety code %q", err, err, tc.code)
			}
			got, getErr := svc.GetPlan(ctx, taskID)
			if getErr != nil || got.Content != created.Plan.Content || got.WriteVersion != created.Plan.WriteVersion {
				t.Fatalf("rejected edit changed plan: %#v, %v", got, getErr)
			}
		})
	}

	history, err := svc.ListRevisions(ctx, taskID)
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("rejected edits added history rows: %d", len(history))
	}
}

func TestPlanExactEditRequiresCurrentVersion(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	taskID := "task-plan-exact-version"
	seedTask(t, ctx, repo, taskID)
	created, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: taskID, Content: "before"})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	for _, expected := range []string{"", "stale"} {
		_, err := svc.EditPlan(ctx, ExactPlanEditRequest{
			TaskID: taskID, ExpectedVersion: expected,
			OldText: "before", NewText: "after",
		})
		if err == nil {
			t.Fatalf("EditPlan with expected version %q succeeded", expected)
		}
		var safety *PlanSafetyError
		if !errors.As(err, &safety) {
			t.Fatalf("error = %T %v, want PlanSafetyError", err, err)
		}
		wantCode := PlanErrorVersionRequired
		if expected != "" {
			wantCode = PlanErrorVersionConflict
		}
		if safety.Code != wantCode {
			t.Fatalf("safety code = %q, want %q", safety.Code, wantCode)
		}
	}
	got, err := svc.GetPlan(ctx, taskID)
	if err != nil || got.Content != "before" || got.WriteVersion != created.Plan.WriteVersion {
		t.Fatalf("plan after version failures = %#v, %v", got, err)
	}
}

func TestPlanExactEditAllowsDeletionAndRejectsFinalEmpty(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	taskID := "task-plan-exact-delete"
	seedTask(t, ctx, repo, taskID)
	created, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: taskID, Content: "keep\nremove"})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	updated, err := svc.EditPlan(ctx, ExactPlanEditRequest{
		TaskID: taskID, ExpectedVersion: created.Plan.WriteVersion,
		OldText: "remove", NewText: "",
	})
	if err != nil {
		t.Fatalf("delete edit: %v", err)
	}
	if updated.Plan.Content != "keep\n" {
		t.Fatalf("content after deletion = %q, want %q", updated.Plan.Content, "keep\\n")
	}

	_, err = svc.EditPlan(ctx, ExactPlanEditRequest{
		TaskID: taskID, ExpectedVersion: updated.Plan.WriteVersion,
		OldText: "keep\n", NewText: "",
	})
	if !errors.Is(err, ErrContentRequired) {
		t.Fatalf("final-empty edit error = %v, want ErrContentRequired", err)
	}
	got, err := svc.GetPlan(ctx, taskID)
	if err != nil || got.Content != "keep\n" || got.WriteVersion != updated.Plan.WriteVersion {
		t.Fatalf("final-empty rejection changed plan: %#v, %v", got, err)
	}
}

func TestPlanExactEditCountsOverlappingUnicodeMatches(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	taskID := "task-plan-exact-unicode"
	seedTask(t, ctx, repo, taskID)
	created, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: taskID, Content: "前 aa 前 aa 后",
	})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	_, err = svc.EditPlan(ctx, ExactPlanEditRequest{
		TaskID: taskID, ExpectedVersion: created.Plan.WriteVersion,
		OldText: "aa", NewText: "長",
	})
	var safety *PlanSafetyError
	if !errors.As(err, &safety) || safety.Code != PlanErrorEditAmbiguous {
		t.Fatalf("unicode ambiguous edit error = %T %v, want %q", err, err, PlanErrorEditAmbiguous)
	}

	if !strings.Contains(created.Plan.Content, "前") {
		t.Fatal("test fixture lost its Unicode content")
	}
}
