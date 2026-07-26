package review

import (
	"strings"
	"testing"
)

func sizedFile(path string, bytes int) ChangedFile {
	return ChangedFile{Path: path, Diff: strings.Repeat("x", bytes)}
}

func TestPlanBatches_GroupsUnderBudget(t *testing.T) {
	files := []ChangedFile{
		sizedFile("a.go", 40),
		sizedFile("b.go", 40),
		sizedFile("c.go", 40),
	}

	plan := PlanBatches(files, 100)
	if len(plan.Batches) != 2 {
		t.Fatalf("expected 2 batches, got %d", len(plan.Batches))
	}
	if len(plan.Batches[0]) != 2 || len(plan.Batches[1]) != 1 {
		t.Fatalf("unexpected batch shape: %d then %d", len(plan.Batches[0]), len(plan.Batches[1]))
	}
	if plan.FileCount() != 3 {
		t.Fatalf("expected 3 submitted files, got %d", plan.FileCount())
	}
	if len(plan.Skipped) != 0 {
		t.Fatalf("expected nothing skipped, got %d", len(plan.Skipped))
	}
}

func TestPlanBatches_PreservesOrder(t *testing.T) {
	files := []ChangedFile{sizedFile("a.go", 10), sizedFile("b.go", 10), sizedFile("c.go", 10)}

	plan := PlanBatches(files, 15)
	var seen []string
	for _, batch := range plan.Batches {
		for _, f := range batch {
			seen = append(seen, f.Path)
		}
	}
	want := []string{"a.go", "b.go", "c.go"}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("order not preserved: got %v want %v", seen, want)
		}
	}
}

func TestPlanBatches_NeverSplitsAFile(t *testing.T) {
	// One file larger than the whole budget cannot be reviewed; it must be
	// reported rather than truncated, because a truncated diff produces findings
	// anchored to lines the reviewer never saw.
	files := []ChangedFile{sizedFile("huge.go", 500), sizedFile("small.go", 10)}

	plan := PlanBatches(files, 100)
	if len(plan.Skipped) != 1 || plan.Skipped[0].Path != "huge.go" {
		t.Fatalf("expected huge.go skipped, got %+v", plan.Skipped)
	}
	if plan.FileCount() != 1 {
		t.Fatalf("expected only small.go submitted, got %d files", plan.FileCount())
	}
}

func TestPlanBatches_SingleFileExactlyAtBudget(t *testing.T) {
	plan := PlanBatches([]ChangedFile{sizedFile("edge.go", 100)}, 100)
	if len(plan.Skipped) != 0 {
		t.Fatalf("a file exactly at budget must be reviewable, got skipped %+v", plan.Skipped)
	}
	if len(plan.Batches) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(plan.Batches))
	}
}

func TestPlanBatches_EmptyInput(t *testing.T) {
	plan := PlanBatches(nil, 100)
	if len(plan.Batches) != 0 || len(plan.Skipped) != 0 || plan.FileCount() != 0 {
		t.Fatalf("expected an empty plan, got %+v", plan)
	}
}

func TestPlanBatches_DefaultsBudget(t *testing.T) {
	plan := PlanBatches([]ChangedFile{sizedFile("a.go", PromptBudgetBytes+1)}, 0)
	if len(plan.Skipped) != 1 {
		t.Fatalf("expected the default budget to apply, got %+v", plan)
	}
}
