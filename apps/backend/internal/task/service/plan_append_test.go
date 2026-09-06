package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	commonlogger "github.com/kandev/kandev/internal/common/logger"
)

func TestParsePlanWriteMode(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    PlanWriteMode
		wantErr bool
	}{
		{"absent means replace", "", PlanWriteModeReplace, false},
		{"explicit replace", "replace", PlanWriteModeReplace, false},
		{"explicit append", "append", PlanWriteModeAppend, false},
		{"case variant Append is rejected", "Append", "", true},
		{"case variant APPEND is rejected", "APPEND", "", true},
		{"case variant REPLACE is rejected", "REPLACE", "", true},
		{"unknown value is rejected", "merge", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePlanWriteMode(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParsePlanWriteMode(%q) = %q, nil, want an error", tt.raw, got)
				}
				if !strings.Contains(err.Error(), "replace") || !strings.Contains(err.Error(), "append") {
					t.Errorf("error %q does not name both accepted values", err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePlanWriteMode(%q): unexpected error: %v", tt.raw, err)
			}
			if got != tt.want {
				t.Errorf("ParsePlanWriteMode(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestValidateAppendFragment(t *testing.T) {
	if err := validateAppendFragment(""); !errors.Is(err, ErrContentRequired) {
		t.Errorf("empty fragment: err = %v, want ErrContentRequired", err)
	}
	if err := validateAppendFragment("   \n\t "); !errors.Is(err, ErrPlanAppendFragmentWhitespaceOnly) {
		t.Errorf("ASCII-whitespace-only fragment: err = %v, want ErrPlanAppendFragmentWhitespaceOnly", err)
	}
	// AC-TASKS-PLAN-APPEND-001.5's boundary test: a fragment whose only
	// non-ASCII-whitespace characters are U+00A0 (NBSP) or U+0085 (NEL) must
	// still be rejected, pinning the Unicode White_Space definition against
	// a hand-rolled " \t\r\n" cutset that would treat these as content.
	if err := validateAppendFragment("\u00a0\u0085"); !errors.Is(err, ErrPlanAppendFragmentWhitespaceOnly) {
		t.Errorf("NBSP/NEL-only fragment: err = %v, want ErrPlanAppendFragmentWhitespaceOnly", err)
	}
	if err := validateAppendFragment("  x  "); err != nil {
		t.Errorf("fragment with a non-whitespace character: unexpected error: %v", err)
	}
}

// TestComposePlanAppend pins the separator normalization algorithm in
// docs/specs/tasks/system-design/plan-write-append-mode.md ("Separator
// normalization"). Every case composes S' and F' (the trimmed bodies), never
// the raw S and F — asserting the untrimmed form would be a wrong test, not
// a found bug (see that section).
func TestComposePlanAppend(t *testing.T) {
	tests := []struct {
		name     string
		stored   string
		fragment string
		want     string
	}{
		{
			name:     "no trailing newline on stored, fragment has leading blank line",
			stored:   "previous",
			fragment: "   \n## New section",
			want:     "previous\n\n## New section",
		},
		{
			name:     "stored ends with one newline",
			stored:   "previous\n",
			fragment: "## New section",
			want:     "previous\n\n## New section",
		},
		{
			name:     "stored ends with several newlines",
			stored:   "previous\n\n\n\n",
			fragment: "## New section",
			want:     "previous\n\n## New section",
		},
		{
			name:     "fragment has several leading blank lines",
			stored:   "previous",
			fragment: "\n\n\n## New section",
			want:     "previous\n\n## New section",
		},
		{
			name:     "fragment's first non-empty line keeps its own indent",
			stored:   "previous",
			fragment: "\n    indented continuation",
			want:     "previous\n\n    indented continuation",
		},
		{
			name:     "empty stored content",
			stored:   "",
			fragment: "## New section",
			want:     "## New section",
		},
		{
			name:     "whitespace-only stored content",
			stored:   " \n\t \n",
			fragment: "\n\n## New section",
			want:     "## New section",
		},
		{
			name:     "fragment's trailing characters preserved exactly, no trailing newline",
			stored:   "previous",
			fragment: "## New section",
			want:     "previous\n\n## New section",
		},
		{
			name:     "fragment's trailing newline preserved",
			stored:   "previous",
			fragment: "## New section\n",
			want:     "previous\n\n## New section\n",
		},
		{
			name:     "CRLF stored and fragment",
			stored:   "previous\r\n",
			fragment: "\r\n## New section",
			want:     "previous\n\n## New section",
		},
		{
			name:     "interior whitespace of stored is untouched",
			stored:   "line one\n\nline two  with trailing spaces  \nline three",
			fragment: "## New section",
			want:     "line one\n\nline two  with trailing spaces  \nline three\n\n## New section",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := composePlanAppend(tt.stored, tt.fragment)
			if got != tt.want {
				t.Errorf("composePlanAppend(%q, %q) = %q, want %q", tt.stored, tt.fragment, got, tt.want)
			}
		})
	}
}

// TestUpdatePlanAppend_ComposesAndPreservesTitle covers the happy path: the
// stored content and the fragment are composed and committed, an omitted
// title leaves the stored title unchanged, and a supplied title applies
// exactly as replace does (AC-TASKS-PLAN-APPEND-002.6).
func TestUpdatePlanAppend_ComposesAndPreservesTitle(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-append-1")

	if _, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: "task-append-1", Title: "Original Title", Content: "# Plan\n\nstep one",
	}); err != nil {
		t.Fatalf("seed CreatePlan: %v", err)
	}

	result, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
		TaskID: "task-append-1", Content: "## Step two", Mode: PlanWriteModeAppend,
	})
	if err != nil {
		t.Fatalf("UpdatePlan append: %v", err)
	}
	want := "# Plan\n\nstep one\n\n## Step two"
	if result.Plan.Content != want {
		t.Errorf("composed content = %q, want %q", result.Plan.Content, want)
	}
	if result.Plan.Title != "Original Title" {
		t.Errorf("title = %q, want stored title preserved (no title supplied)", result.Plan.Title)
	}

	// AC-TASKS-PLAN-APPEND-005.5: the stored revision's content must be the
	// composed document, not the raw fragment that was submitted — a reader
	// of the revision history must be able to see the whole document at that
	// point in time, the same as a replace-mode revision.
	rev, err := svc.GetLatestRevision(ctx, "task-append-1")
	if err != nil {
		t.Fatalf("GetLatestRevision: %v", err)
	}
	if rev.Content != want {
		t.Errorf("revision content = %q, want the composed content %q, not the raw fragment", rev.Content, want)
	}

	result, err = svc.UpdatePlan(ctx, UpdatePlanRequest{
		TaskID: "task-append-1", Title: "New Title", Content: "## Step three", Mode: PlanWriteModeAppend,
	})
	if err != nil {
		t.Fatalf("UpdatePlan append with title: %v", err)
	}
	if result.Plan.Title != "New Title" {
		t.Errorf("title = %q, want the supplied title applied", result.Plan.Title)
	}
}

// TestUpdatePlanAppend_IsNotIdempotent pins AC-TASKS-PLAN-APPEND-003.6:
// appending the identical fragment twice produces two occurrences in the
// composed content, not one — an append composes onto whatever is currently
// stored; it does not deduplicate against a prior append of the same text.
func TestUpdatePlanAppend_IsNotIdempotent(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-append-repeat")
	if _, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-append-repeat", Content: "base"}); err != nil {
		t.Fatalf("seed CreatePlan: %v", err)
	}

	fragment := "## Repeated section"
	if _, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
		TaskID: "task-append-repeat", Content: fragment, Mode: PlanWriteModeAppend,
	}); err != nil {
		t.Fatalf("first append: %v", err)
	}
	result, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
		TaskID: "task-append-repeat", Content: fragment, Mode: PlanWriteModeAppend,
	})
	if err != nil {
		t.Fatalf("second append: %v", err)
	}

	if got := strings.Count(result.Plan.Content, fragment); got != 2 {
		t.Fatalf("expected the identical fragment to appear twice after two appends, got %d occurrence(s) in %q", got, result.Plan.Content)
	}
}

// TestUpdatePlanAppend_EventParityWithReplace pins that an append publishes
// the same event types, in the same order, as a replace that produces the
// same final content: append is a composition detail of UpdatePlan, not a
// distinct write path from the event bus's perspective, so subscribers (WS
// broadcast, etc.) need not special-case it.
func TestUpdatePlanAppend_EventParityWithReplace(t *testing.T) {
	ctx := context.Background()

	appendSvc, appendBus, appendRepo := createTestPlanService(t)
	seedTask(t, ctx, appendRepo, "task-append-parity")
	if _, err := appendSvc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-append-parity", Content: "base"}); err != nil {
		t.Fatalf("seed CreatePlan (append side): %v", err)
	}
	appendBus.ClearEvents()
	if _, err := appendSvc.UpdatePlan(ctx, UpdatePlanRequest{
		TaskID: "task-append-parity", Content: "## New section", Mode: PlanWriteModeAppend,
	}); err != nil {
		t.Fatalf("UpdatePlan append: %v", err)
	}
	appendEvents := appendBus.GetPublishedEvents()

	replaceSvc, replaceBus, replaceRepo := createTestPlanService(t)
	seedTask(t, ctx, replaceRepo, "task-replace-parity")
	if _, err := replaceSvc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-replace-parity", Content: "base"}); err != nil {
		t.Fatalf("seed CreatePlan (replace side): %v", err)
	}
	replaceBus.ClearEvents()
	if _, err := replaceSvc.UpdatePlan(ctx, UpdatePlanRequest{
		TaskID: "task-replace-parity", Content: "base\n\n## New section", Mode: PlanWriteModeReplace,
	}); err != nil {
		t.Fatalf("UpdatePlan replace: %v", err)
	}
	replaceEvents := replaceBus.GetPublishedEvents()

	if len(appendEvents) != len(replaceEvents) {
		t.Fatalf("event count mismatch: append published %d event(s) %v, replace published %d event(s) %v",
			len(appendEvents), eventTypes(appendEvents), len(replaceEvents), eventTypes(replaceEvents))
	}
	for i := range appendEvents {
		if appendEvents[i].Type != replaceEvents[i].Type {
			t.Errorf("event[%d] type mismatch: append=%q replace=%q", i, appendEvents[i].Type, replaceEvents[i].Type)
		}
	}
}

// TestUpdatePlanAppend_RejectsAbsentPlan pins AC-TASKS-PLAN-APPEND-001.6: an
// append against a task with no plan gets the same plan-not-found outcome as
// replace, and creates no plan.
func TestUpdatePlanAppend_RejectsAbsentPlan(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-append-absent")

	_, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
		TaskID: "task-append-absent", Content: "fragment", Mode: PlanWriteModeAppend,
	})
	if !errors.Is(err, ErrTaskPlanNotFound) {
		t.Fatalf("err = %v, want ErrTaskPlanNotFound", err)
	}
	plan, err := svc.GetPlan(ctx, "task-append-absent")
	if err != nil {
		t.Fatalf("GetPlan: %v", err)
	}
	if plan != nil {
		t.Fatalf("expected no plan created, got %+v", plan)
	}
}

// TestUpdatePlanAppend_RejectsEmptyOrWhitespaceOnlyFragment pins
// AC-TASKS-PLAN-APPEND-001.4/001.5 and asserts the rejection is
// non-destructive: the stored plan and revision count are unchanged.
func TestUpdatePlanAppend_RejectsEmptyOrWhitespaceOnlyFragment(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-append-empty")
	if _, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-append-empty", Content: "base"}); err != nil {
		t.Fatalf("seed CreatePlan: %v", err)
	}

	cases := []struct {
		name    string
		content string
		wantErr error
	}{
		{"empty content", "", ErrContentRequired},
		{"whitespace-only fragment", "  \n\t  ", ErrPlanAppendFragmentWhitespaceOnly},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
				TaskID: "task-append-empty", Content: tc.content, Mode: PlanWriteModeAppend,
			})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			plan, getErr := svc.GetPlan(ctx, "task-append-empty")
			if getErr != nil {
				t.Fatalf("GetPlan: %v", getErr)
			}
			if plan.Content != "base" {
				t.Fatalf("stored content changed after a rejected append: %q", plan.Content)
			}
			revs, listErr := svc.ListRevisions(ctx, "task-append-empty")
			if listErr != nil {
				t.Fatalf("ListRevisions: %v", listErr)
			}
			if len(revs) != 1 {
				t.Fatalf("expected the seed's single revision to survive, got %d", len(revs))
			}
		})
	}
}

// TestUpdatePlanAppend_AuthorizationRunsBeforeContentValidity pins
// AC-TASKS-PLAN-APPEND-001.7's order for append: a request invalid on both
// task-reach authorization and content validity reports the authorization
// failure, not the content error.
func TestUpdatePlanAppend_AuthorizationRunsBeforeContentValidity(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-append-denied")
	if _, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-append-denied", Content: "base"}); err != nil {
		t.Fatalf("seed CreatePlan: %v", err)
	}

	denied := errors.New("not visible to this caller")
	svc.SetTaskAuthorizer(func(context.Context, string) error { return denied })

	_, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
		TaskID: "task-append-denied", Content: "", Mode: PlanWriteModeAppend,
	})
	if !errors.Is(err, denied) {
		t.Fatalf("err = %v, want the authorization failure (not the empty-content error)", err)
	}
}

// TestUpdatePlanReplace_AuthorizationRunsBeforeContentValidity pins
// AC-TASKS-PLAN-APPEND-001.7's order for an explicit replace, mirroring
// TestUpdatePlanAppend_AuthorizationRunsBeforeContentValidity: a request
// invalid on both task-reach authorization and content validity (empty
// content) reports the authorization failure, not ErrContentRequired. This is
// the regression pin for the ordering bug this fix corrects — before it,
// UpdatePlan checked req.Content == "" ahead of authorization.
func TestUpdatePlanReplace_AuthorizationRunsBeforeContentValidity(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-replace-denied")
	if _, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-replace-denied", Content: "base"}); err != nil {
		t.Fatalf("seed CreatePlan: %v", err)
	}

	denied := errors.New("not visible to this caller")
	svc.SetTaskAuthorizer(func(context.Context, string) error { return denied })

	_, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
		TaskID: "task-replace-denied", Content: "", Mode: PlanWriteModeReplace,
	})
	if !errors.Is(err, denied) {
		t.Fatalf("err = %v, want the authorization failure (not ErrContentRequired)", err)
	}
	if errors.Is(err, ErrContentRequired) {
		t.Fatalf("err must not also match ErrContentRequired: %v", err)
	}
}

// TestUpdatePlanAppend_ReadFailureIsDistinctFromNotFound pins
// AC-TASKS-PLAN-APPEND-003.5: a failed read of the stored content aborts the
// write, leaves the stored plan untouched, creates no revision, and is
// reported as a different sentinel than ErrTaskPlanNotFound.
func TestUpdatePlanAppend_ReadFailureIsDistinctFromNotFound(t *testing.T) {
	svc, eventBus, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-append-readfail")
	if _, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-append-readfail", Content: "base"}); err != nil {
		t.Fatalf("seed CreatePlan: %v", err)
	}

	flaky := newFlakyPlanService(t, repo, eventBus, 1) // every GetTaskPlan call fails
	_, err := flaky.UpdatePlan(ctx, UpdatePlanRequest{
		TaskID: "task-append-readfail", Content: "fragment", Mode: PlanWriteModeAppend,
	})
	if !errors.Is(err, ErrPlanContentReadFailed) {
		t.Fatalf("err = %v, want ErrPlanContentReadFailed", err)
	}
	if errors.Is(err, ErrTaskPlanNotFound) {
		t.Fatalf("read failure must not also match ErrTaskPlanNotFound: %v", err)
	}

	plan, getErr := svc.GetPlan(ctx, "task-append-readfail")
	if getErr != nil {
		t.Fatalf("GetPlan: %v", getErr)
	}
	if plan.Content != "base" {
		t.Fatalf("stored content changed after a read-failed append: %q", plan.Content)
	}
	revs, listErr := svc.ListRevisions(ctx, "task-append-readfail")
	if listErr != nil {
		t.Fatalf("ListRevisions: %v", listErr)
	}
	if len(revs) != 1 {
		t.Fatalf("expected no new revision, got %d", len(revs))
	}
}

// TestUpdatePlanAppend_ReadFailureLogsError pins COR-001: a HEAD read
// failure on the append path (planHeadUnknown) is visible in the logs, not
// silently swallowed. readPlanHead's caller previously discarded the
// underlying repository error entirely; mirroring logPlanWriteError's
// pattern, it must reach an Error-level log line naming the task.
func TestUpdatePlanAppend_ReadFailureLogsError(t *testing.T) {
	svc, eventBus, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-append-readfail-log")
	if _, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-append-readfail-log", Content: "base"}); err != nil {
		t.Fatalf("seed CreatePlan: %v", err)
	}

	flaky := newFlakyPlanService(t, repo, eventBus, 1) // every GetTaskPlan call fails
	core, logs := observer.New(zapcore.WarnLevel)
	logWithObserver, err := commonlogger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	flaky.logger = logWithObserver

	_, err = flaky.UpdatePlan(ctx, UpdatePlanRequest{
		TaskID: "task-append-readfail-log", Content: "fragment", Mode: PlanWriteModeAppend,
	})
	if !errors.Is(err, ErrPlanContentReadFailed) {
		t.Fatalf("err = %v, want ErrPlanContentReadFailed", err)
	}

	entries := logs.All()
	found := false
	for _, entry := range entries {
		if entry.Level < zapcore.ErrorLevel {
			continue
		}
		ctxMap := entry.ContextMap()
		if ctxMap["task_id"] == "task-append-readfail-log" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected an Error-level log naming task_id=task-append-readfail-log, got: %+v", entries)
	}
}

// TestUpdatePlanAppend_NoTruncationGuard pins REQ-TASKS-PLAN-APPEND-004: the
// truncation guard never fires on an append, even if a caller mistakenly
// sets EvaluateTruncation, and the write is not forced onto a new revision
// on the guard's account.
func TestUpdatePlanAppend_NoTruncationGuard(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-append-guard")

	large := strings.Repeat("x", planTruncationMinPriorChars+1000)
	if _, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: "task-append-guard", Content: large, AuthorKind: "agent", AuthorName: "Claude",
	}); err != nil {
		t.Fatalf("seed CreatePlan: %v", err)
	}

	result, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
		TaskID: "task-append-guard", Content: "small fragment", Mode: PlanWriteModeAppend,
		AuthorKind: "agent", AuthorName: "Claude",
		EvaluateTruncation: true, // a caller mistake the service must override
	})
	if err != nil {
		t.Fatalf("UpdatePlan append: %v", err)
	}
	if result.TruncationDetected {
		t.Fatalf("truncation guard fired on an append, which cannot lose content by construction")
	}
	if result.PriorRevisionNumber != 0 {
		t.Errorf("PriorRevisionNumber = %d, want 0 (unwrapped response shape)", result.PriorRevisionNumber)
	}

	// Same author within the coalesce window: not forced onto a new
	// revision on the guard's account (AC-TASKS-PLAN-APPEND-004.2).
	revs, err := svc.ListRevisions(ctx, "task-append-guard")
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(revs) != 1 {
		t.Fatalf("expected the append to coalesce with the seed revision (same author, in-window), got %d revisions", len(revs))
	}
}

// TestUpdatePlanAppend_SizeLimit pins REQ-TASKS-PLAN-APPEND-007: the size
// limit is evaluated against the composed content, not the fragment, and
// the reported size is always the composed one.
func TestUpdatePlanAppend_SizeLimit(t *testing.T) {
	t.Run("fragment far below the limit but composed content exceeds it", func(t *testing.T) {
		svc, _, repo := createTestPlanService(t)
		ctx := context.Background()
		seedTask(t, ctx, repo, "task-append-size-1")
		stored := strings.Repeat("a", MaxPlanContentBytes-10)
		if _, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-append-size-1", Content: stored}); err != nil {
			t.Fatalf("seed CreatePlan: %v", err)
		}
		fragment := strings.Repeat("b", 100) // far below MaxPlanContentBytes alone
		_, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
			TaskID: "task-append-size-1", Content: fragment, Mode: PlanWriteModeAppend,
		})
		var sizeErr *PlanContentTooLargeError
		if !errors.As(err, &sizeErr) {
			t.Fatalf("err = %v, want *PlanContentTooLargeError", err)
		}
		wantComposed := len(stored) + len("\n\n") + len(fragment)
		if sizeErr.Submitted != wantComposed {
			t.Errorf("Submitted = %d, want the composed size %d (not the fragment's %d)", sizeErr.Submitted, wantComposed, len(fragment))
		}
		plan, getErr := svc.GetPlan(ctx, "task-append-size-1")
		if getErr != nil {
			t.Fatalf("GetPlan: %v", getErr)
		}
		if plan.Content != stored {
			t.Fatalf("stored content changed after a rejected oversized append")
		}
		revs, listErr := svc.ListRevisions(ctx, "task-append-size-1")
		if listErr != nil {
			t.Fatalf("ListRevisions: %v", listErr)
		}
		if len(revs) != 1 {
			t.Fatalf("expected no new revision, got %d", len(revs))
		}
	})

	t.Run("fragment alone exceeds the limit", func(t *testing.T) {
		svc, _, repo := createTestPlanService(t)
		ctx := context.Background()
		seedTask(t, ctx, repo, "task-append-size-2")
		if _, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-append-size-2", Content: "base"}); err != nil {
			t.Fatalf("seed CreatePlan: %v", err)
		}
		fragment := strings.Repeat("b", MaxPlanContentBytes+1)
		_, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
			TaskID: "task-append-size-2", Content: fragment, Mode: PlanWriteModeAppend,
		})
		var sizeErr *PlanContentTooLargeError
		if !errors.As(err, &sizeErr) {
			t.Fatalf("err = %v, want *PlanContentTooLargeError", err)
		}
		wantComposed := len("base") + len("\n\n") + len(fragment)
		if sizeErr.Submitted != wantComposed {
			t.Errorf("Submitted = %d, want the composed size %d", sizeErr.Submitted, wantComposed)
		}
	})

	t.Run("composed content exactly at the limit is accepted", func(t *testing.T) {
		svc, _, repo := createTestPlanService(t)
		ctx := context.Background()
		seedTask(t, ctx, repo, "task-append-size-3")
		stored := strings.Repeat("a", 10)
		if _, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-append-size-3", Content: stored}); err != nil {
			t.Fatalf("seed CreatePlan: %v", err)
		}
		fragmentLen := MaxPlanContentBytes - len(stored) - len("\n\n")
		fragment := strings.Repeat("b", fragmentLen)
		result, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
			TaskID: "task-append-size-3", Content: fragment, Mode: PlanWriteModeAppend,
		})
		if err != nil {
			t.Fatalf("UpdatePlan append at the exact limit: %v", err)
		}
		if len(result.Plan.Content) != MaxPlanContentBytes {
			t.Fatalf("composed content length = %d, want exactly %d", len(result.Plan.Content), MaxPlanContentBytes)
		}
	})
}

// TestCreatePlan_ModeIsInert pins the design's "composition must not trigger
// when requireExistingHead is false": a Mode set on CreatePlanRequest never
// composes anything, since create has no stored content to compose against.
func TestCreatePlan_ModeIsInert(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-create-mode-inert")

	result, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: "task-create-mode-inert", Content: "fragment", Mode: PlanWriteModeAppend,
	})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	if result.Plan.Content != "fragment" {
		t.Fatalf("content = %q, want the literal submitted content, uncomposed", result.Plan.Content)
	}
}

// TestUpdatePlanReplace_UnaffectedByAppendMode is a regression pin:
// replace-mode behavior (AC-TASKS-PLAN-APPEND-005.1) is untouched by this
// capability. Mode's zero value already exercises replace via every other
// test in this package; this asserts the same for an explicit
// PlanWriteModeReplace value.
func TestUpdatePlanReplace_UnaffectedByAppendMode(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-replace-explicit")
	if _, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-replace-explicit", Content: "base"}); err != nil {
		t.Fatalf("seed CreatePlan: %v", err)
	}
	result, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
		TaskID: "task-replace-explicit", Content: "whole new document", Mode: PlanWriteModeReplace,
	})
	if err != nil {
		t.Fatalf("UpdatePlan replace: %v", err)
	}
	if result.Plan.Content != "whole new document" {
		t.Errorf("content = %q, want the submitted content verbatim (replace, not composed)", result.Plan.Content)
	}
}
