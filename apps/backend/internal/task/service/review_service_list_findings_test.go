package service

// Coverage for ReviewService.ListFindings (REQ-TWS-003): filters, ordering,
// truncation, authorization and error-vs-empty-success behavior.

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func findingInputAt(repo, file string, line int, title, severity string) ReviewFindingInput {
	f := validFindingInput()
	f.RepositoryName = repo
	f.FilePath = file
	f.StartLine = line
	f.EndLine = line
	f.Title = title
	f.Severity = severity
	return f
}

// reviewFindingsListErrorRepo forces ListTaskReviewFindings to fail, so
// AC-TWS-003.14's error-result path can be exercised without a real DB fault.
type reviewFindingsListErrorRepo struct {
	*sqliterepo.Repository
	err error
}

func (r *reviewFindingsListErrorRepo) ListTaskReviewFindings(_ context.Context, _ string) ([]*models.TaskReviewFinding, error) {
	return nil, r.err
}

func TestReviewService_ListFindings_RequiresTaskID(t *testing.T) {
	svc, _, _ := createTestReviewService(t)
	if _, err := svc.ListFindings(context.Background(), ListFindingsRequest{}); !errors.Is(err, ErrTaskIDRequired) {
		t.Fatalf("expected ErrTaskIDRequired, got %v", err)
	}
}

func TestReviewService_ListFindings_EmptyTaskReturnsEmptySuccess(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-lf-empty")

	result, err := svc.ListFindings(ctx, ListFindingsRequest{TaskID: "task-lf-empty"})
	if err != nil {
		t.Fatalf("ListFindings: %v", err)
	}
	if result.Findings == nil {
		t.Fatal("expected an empty (non-nil) slice, got nil")
	}
	if len(result.Findings) != 0 || result.TotalMatched != 0 || result.Truncated {
		t.Fatalf("expected empty success, got %+v", result)
	}
}

func TestReviewService_ListFindings_RoundTripsEveryField(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-lf-roundtrip")

	in := validFindingInput()
	in.RepositoryName = "kandev/kandev"
	in.EndLine = 0 // defaults to StartLine
	_, published, err := svc.PublishFindings(ctx, PublishFindingsRequest{
		TaskID:   "task-lf-roundtrip",
		Findings: []ReviewFindingInput{in},
	})
	if err != nil {
		t.Fatalf("PublishFindings: %v", err)
	}

	result, err := svc.ListFindings(ctx, ListFindingsRequest{TaskID: "task-lf-roundtrip"})
	if err != nil {
		t.Fatalf("ListFindings: %v", err)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(result.Findings))
	}
	got := result.Findings[0]
	want := published[0]
	if got.ID != want.ID || got.RunID != want.RunID || got.RepositoryName != want.RepositoryName ||
		got.FilePath != want.FilePath || got.StartLine != want.StartLine || got.EndLine != want.EndLine ||
		got.Severity != want.Severity || got.Title != want.Title {
		t.Fatalf("round-trip mismatch: got %+v, want %+v", got, want)
	}
	if got.EndLine != got.StartLine {
		t.Fatalf("expected a defaulted line_end to round-trip as the start line, got %d vs %d", got.EndLine, got.StartLine)
	}
}

func TestReviewService_ListFindings_DefaultsToOpenStatus(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-lf-default")

	_, open, _ := svc.PublishFindings(ctx, PublishFindingsRequest{
		TaskID:   "task-lf-default",
		Findings: []ReviewFindingInput{findingInputAt("r", "a.go", 1, "open one", "minor")},
	})
	_, resolved, _ := svc.PublishFindings(ctx, PublishFindingsRequest{
		TaskID:   "task-lf-default",
		Findings: []ReviewFindingInput{findingInputAt("r", "b.go", 1, "resolved one", "minor")},
	})
	if _, err := svc.UpdateFindingStatus(ctx, resolved[0].ID, models.ReviewFindingResolved); err != nil {
		t.Fatalf("UpdateFindingStatus: %v", err)
	}

	result, err := svc.ListFindings(ctx, ListFindingsRequest{TaskID: "task-lf-default"})
	if err != nil {
		t.Fatalf("ListFindings: %v", err)
	}
	if len(result.Findings) != 1 || result.Findings[0].ID != open[0].ID {
		t.Fatalf("expected only the open finding by default, got %+v", result.Findings)
	}
}

func TestReviewService_ListFindings_StatusFilterAll(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-lf-all")

	_, open, _ := svc.PublishFindings(ctx, PublishFindingsRequest{TaskID: "task-lf-all", Findings: []ReviewFindingInput{findingInputAt("r", "a.go", 1, "open", "minor")}})
	_, resolved, _ := svc.PublishFindings(ctx, PublishFindingsRequest{TaskID: "task-lf-all", Findings: []ReviewFindingInput{findingInputAt("r", "b.go", 1, "resolved", "minor")}})
	_, dismissed, _ := svc.PublishFindings(ctx, PublishFindingsRequest{TaskID: "task-lf-all", Findings: []ReviewFindingInput{findingInputAt("r", "c.go", 1, "dismissed", "minor")}})
	if _, err := svc.UpdateFindingStatus(ctx, resolved[0].ID, models.ReviewFindingResolved); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateFindingStatus(ctx, dismissed[0].ID, models.ReviewFindingDismissed); err != nil {
		t.Fatal(err)
	}

	result, err := svc.ListFindings(ctx, ListFindingsRequest{TaskID: "task-lf-all", Status: "all"})
	if err != nil {
		t.Fatalf("ListFindings: %v", err)
	}
	if len(result.Findings) != 3 {
		t.Fatalf("expected all 3 statuses returned, got %d", len(result.Findings))
	}
	_ = open
}

func TestReviewService_ListFindings_StatusFilterResolvedAndDismissed(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-lf-rd")

	_, resolved, _ := svc.PublishFindings(ctx, PublishFindingsRequest{TaskID: "task-lf-rd", Findings: []ReviewFindingInput{findingInputAt("r", "a.go", 1, "resolved", "minor")}})
	_, dismissed, _ := svc.PublishFindings(ctx, PublishFindingsRequest{TaskID: "task-lf-rd", Findings: []ReviewFindingInput{findingInputAt("r", "b.go", 1, "dismissed", "minor")}})
	if _, err := svc.UpdateFindingStatus(ctx, resolved[0].ID, models.ReviewFindingResolved); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateFindingStatus(ctx, dismissed[0].ID, models.ReviewFindingDismissed); err != nil {
		t.Fatal(err)
	}

	got, err := svc.ListFindings(ctx, ListFindingsRequest{TaskID: "task-lf-rd", Status: "resolved"})
	if err != nil || len(got.Findings) != 1 || got.Findings[0].ID != resolved[0].ID {
		t.Fatalf("resolved filter = %+v, err=%v", got, err)
	}
	got, err = svc.ListFindings(ctx, ListFindingsRequest{TaskID: "task-lf-rd", Status: "dismissed"})
	if err != nil || len(got.Findings) != 1 || got.Findings[0].ID != dismissed[0].ID {
		t.Fatalf("dismissed filter = %+v, err=%v", got, err)
	}
}

func TestReviewService_ListFindings_SeverityFilterAndIntersection(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-lf-sev")

	_, blocker, _ := svc.PublishFindings(ctx, PublishFindingsRequest{TaskID: "task-lf-sev", Findings: []ReviewFindingInput{findingInputAt("r", "a.go", 1, "blocker one", "blocker")}})
	_, minor, _ := svc.PublishFindings(ctx, PublishFindingsRequest{TaskID: "task-lf-sev", Findings: []ReviewFindingInput{findingInputAt("r", "b.go", 1, "minor one", "minor")}})
	_, dismissedBlocker, _ := svc.PublishFindings(ctx, PublishFindingsRequest{TaskID: "task-lf-sev", Findings: []ReviewFindingInput{findingInputAt("r", "c.go", 1, "blocker two", "blocker")}})
	if _, err := svc.UpdateFindingStatus(ctx, dismissedBlocker[0].ID, models.ReviewFindingDismissed); err != nil {
		t.Fatal(err)
	}

	bySeverity, err := svc.ListFindings(ctx, ListFindingsRequest{TaskID: "task-lf-sev", Status: "all", Severity: "blocker"})
	if err != nil || len(bySeverity.Findings) != 2 {
		t.Fatalf("severity filter = %+v, err=%v", bySeverity, err)
	}
	// Intersection: default status=open excludes the dismissed blocker.
	intersect, err := svc.ListFindings(ctx, ListFindingsRequest{TaskID: "task-lf-sev", Severity: "blocker"})
	if err != nil || len(intersect.Findings) != 1 || intersect.Findings[0].ID != blocker[0].ID {
		t.Fatalf("status+severity intersection = %+v, err=%v", intersect, err)
	}
	_ = minor
}

func TestReviewService_ListFindings_NormalizesCaseAndWhitespace(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-lf-norm")
	if _, _, err := svc.PublishFindings(ctx, PublishFindingsRequest{TaskID: "task-lf-norm", Findings: []ReviewFindingInput{findingInputAt("r", "a.go", 1, "blocker one", "blocker")}}); err != nil {
		t.Fatalf("PublishFindings: %v", err)
	}

	for _, status := range []string{"OPEN", " open ", "Open"} {
		got, err := svc.ListFindings(ctx, ListFindingsRequest{TaskID: "task-lf-norm", Status: status})
		if err != nil || len(got.Findings) != 1 {
			t.Fatalf("status %q: got %+v, err=%v", status, got, err)
		}
	}
	for _, sev := range []string{"BLOCKER", " blocker "} {
		got, err := svc.ListFindings(ctx, ListFindingsRequest{TaskID: "task-lf-norm", Severity: sev})
		if err != nil || len(got.Findings) != 1 {
			t.Fatalf("severity %q: got %+v, err=%v", sev, got, err)
		}
	}
}

func TestReviewService_ListFindings_EmptyOrNullFilterOmitted(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-lf-omit")
	if _, _, err := svc.PublishFindings(ctx, PublishFindingsRequest{TaskID: "task-lf-omit", Findings: []ReviewFindingInput{findingInputAt("r", "a.go", 1, "t", "blocker")}}); err != nil {
		t.Fatalf("PublishFindings: %v", err)
	}

	// Empty status takes the AC-TWS-003.5 default (open); empty severity does
	// not restrict (AC-TWS-003.6) — both present here so the finding matches.
	got, err := svc.ListFindings(ctx, ListFindingsRequest{TaskID: "task-lf-omit", Status: "", Severity: ""})
	if err != nil || len(got.Findings) != 1 {
		t.Fatalf("omitted filters = %+v, err=%v", got, err)
	}
}

func TestReviewService_ListFindings_RejectsUnrecognizedStatus(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-lf-badstatus")

	_, err := svc.ListFindings(ctx, ListFindingsRequest{TaskID: "task-lf-badstatus", Status: "archived"})
	if !errors.Is(err, ErrInvalidReviewFindingFilter) {
		t.Fatalf("expected ErrInvalidReviewFindingFilter, got %v", err)
	}
	for _, want := range []string{"open", "resolved", "dismissed", "all"} {
		if !contains(err.Error(), want) {
			t.Fatalf("error %q should name accepted value %q", err.Error(), want)
		}
	}
}

func TestReviewService_ListFindings_RejectsUnrecognizedSeverity(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-lf-badsev")

	_, err := svc.ListFindings(ctx, ListFindingsRequest{TaskID: "task-lf-badsev", Severity: "urgent"})
	if !errors.Is(err, ErrInvalidReviewFindingFilter) {
		t.Fatalf("expected ErrInvalidReviewFindingFilter, got %v", err)
	}
	for _, want := range []string{"blocker", "major", "minor", "nit"} {
		if !contains(err.Error(), want) {
			t.Fatalf("error %q should name accepted value %q", err.Error(), want)
		}
	}
}

func TestReviewService_ListFindings_Ordering(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-lf-order")

	if _, _, err := svc.PublishFindings(ctx, PublishFindingsRequest{TaskID: "task-lf-order", Findings: []ReviewFindingInput{
		findingInputAt("z-repo", "a.go", 5, "t1", "minor"),
		findingInputAt("a-repo", "z.go", 1, "t2", "minor"),
		findingInputAt("a-repo", "a.go", 9, "t3", "minor"),
		findingInputAt("a-repo", "a.go", 2, "t4", "minor"),
	}}); err != nil {
		t.Fatalf("PublishFindings: %v", err)
	}

	got, err := svc.ListFindings(ctx, ListFindingsRequest{TaskID: "task-lf-order"})
	if err != nil {
		t.Fatalf("ListFindings: %v", err)
	}
	if len(got.Findings) != 4 {
		t.Fatalf("expected 4 findings, got %d", len(got.Findings))
	}
	want := []string{"t4", "t3", "t2", "t1"} // a-repo/a.go/2, a-repo/a.go/9, a-repo/z.go/1, z-repo/a.go/5
	for i, w := range want {
		if got.Findings[i].Title != w {
			t.Fatalf("position %d = %q, want %q (full order: %v)", i, got.Findings[i].Title, w, titles(got.Findings))
		}
	}
}

func titles(findings []*models.TaskReviewFinding) []string {
	out := make([]string, len(findings))
	for i, f := range findings {
		out[i] = f.Title
	}
	return out
}

func TestReviewService_ListFindings_TruncatesAt100(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-lf-trunc")

	inputs := make([]ReviewFindingInput, 0, 101)
	for i := 0; i < 101; i++ {
		inputs = append(inputs, findingInputAt("r", "a.go", i+1, "t", "minor"))
	}
	if _, _, err := svc.PublishFindings(ctx, PublishFindingsRequest{TaskID: "task-lf-trunc", Findings: inputs}); err != nil {
		t.Fatalf("PublishFindings: %v", err)
	}

	got, err := svc.ListFindings(ctx, ListFindingsRequest{TaskID: "task-lf-trunc"})
	if err != nil {
		t.Fatalf("ListFindings: %v", err)
	}
	if len(got.Findings) != 100 || !got.Truncated || got.TotalMatched != 101 {
		t.Fatalf("expected 100/truncated=true/total=101, got %d/%v/%d", len(got.Findings), got.Truncated, got.TotalMatched)
	}
	// The retained 100 are the first in AC-TWS-003.8 order (start_line ascending here).
	if got.Findings[0].StartLine != 1 || got.Findings[99].StartLine != 100 {
		t.Fatalf("expected the first 100 by order retained, got start lines %d..%d", got.Findings[0].StartLine, got.Findings[99].StartLine)
	}
}

func TestReviewService_ListFindings_ExactlyOneHundredDoesNotTruncate(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-lf-exact100")

	inputs := make([]ReviewFindingInput, 0, 100)
	for i := 0; i < 100; i++ {
		inputs = append(inputs, findingInputAt("r", "a.go", i+1, "t", "minor"))
	}
	if _, _, err := svc.PublishFindings(ctx, PublishFindingsRequest{TaskID: "task-lf-exact100", Findings: inputs}); err != nil {
		t.Fatalf("PublishFindings: %v", err)
	}

	got, err := svc.ListFindings(ctx, ListFindingsRequest{TaskID: "task-lf-exact100"})
	if err != nil {
		t.Fatalf("ListFindings: %v", err)
	}
	if len(got.Findings) != 100 || got.Truncated || got.TotalMatched != 100 {
		t.Fatalf("expected 100/truncated=false/total=100, got %d/%v/%d", len(got.Findings), got.Truncated, got.TotalMatched)
	}
}

func TestReviewService_ListFindings_AcrossMultipleRuns(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-lf-runs")

	_, firstRun, err := svc.PublishFindings(ctx, PublishFindingsRequest{TaskID: "task-lf-runs", Findings: []ReviewFindingInput{findingInputAt("r", "a.go", 1, "first", "minor")}})
	if err != nil {
		t.Fatalf("first publish: %v", err)
	}
	_, secondRun, err := svc.PublishFindings(ctx, PublishFindingsRequest{TaskID: "task-lf-runs", Findings: []ReviewFindingInput{findingInputAt("r", "b.go", 1, "second", "minor")}})
	if err != nil {
		t.Fatalf("second publish: %v", err)
	}
	if firstRun[0].RunID == secondRun[0].RunID {
		t.Fatal("test setup invalid: expected two distinct runs")
	}

	got, err := svc.ListFindings(ctx, ListFindingsRequest{TaskID: "task-lf-runs"})
	if err != nil {
		t.Fatalf("ListFindings: %v", err)
	}
	runIDs := map[string]bool{}
	for _, f := range got.Findings {
		runIDs[f.RunID] = true
	}
	if len(runIDs) != 2 || !runIDs[firstRun[0].RunID] || !runIDs[secondRun[0].RunID] {
		t.Fatalf("expected findings from both runs, got run ids %v", runIDs)
	}
}

func TestReviewService_ListFindings_AuthorizationDenied(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-lf-denied")

	denied := errors.New("distinctive denial message")
	svc.SetTaskAuthorizer(func(_ context.Context, _ string) error { return denied })

	_, err := svc.ListFindings(ctx, ListFindingsRequest{TaskID: "task-lf-denied"})
	var accessDenied *ErrReviewAccessDenied
	if !errors.As(err, &accessDenied) {
		t.Fatalf("expected ErrReviewAccessDenied, got %v", err)
	}
	if err.Error() != denied.Error() {
		t.Fatalf("expected the authorizer's own message surfaced verbatim, got %q want %q", err.Error(), denied.Error())
	}
}

func TestReviewService_ListFindings_PersistenceFailureIsAnError(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	_, _, repo := createTestReviewService(t)
	failing := &reviewFindingsListErrorRepo{Repository: repo, err: errors.New("disk full")}
	svc := NewReviewService(failing, nil, log)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-lf-fail")

	_, err := svc.ListFindings(ctx, ListFindingsRequest{TaskID: "task-lf-fail"})
	if err == nil {
		t.Fatal("expected an error, got nil (success with empty findings is forbidden on a read failure)")
	}
	if !contains(err.Error(), "list") && !contains(err.Error(), "read") {
		t.Fatalf("error should name the failed read, got %q", err.Error())
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || len(needle) == 0 ||
		func() bool {
			for i := 0; i+len(needle) <= len(haystack); i++ {
				if haystack[i:i+len(needle)] == needle {
					return true
				}
			}
			return false
		}())
}
