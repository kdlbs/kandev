package github

import "testing"

func TestMergeChecksDeduplicatesIdentifiedRunsAgainstStatusContexts(t *testing.T) {
	checkRuns := []CheckRun{
		{ID: 11, AppID: 1, AppSlug: "ci-one", CheckSuiteID: 101, Name: "CI / test",
			Source: checkSourceCheckRun, Status: checkStatusCompleted, Conclusion: checkConclusionSuccess},
		{ID: 12, AppID: 2, AppSlug: "ci-two", CheckSuiteID: 202, Name: "ci / test",
			Source: checkSourceCheckRun, Status: checkStatusCompleted, Conclusion: checkConclusionSuccess},
	}
	statusContexts := []CheckRun{{
		Name: " CI / TEST ", Source: checkSourceStatusContext,
		Status: checkStatusCompleted, Conclusion: checkConclusionFail,
	}}

	merged := mergeChecks(checkRuns, statusContexts)
	if len(merged) != 2 {
		t.Fatalf("merged checks = %#v, want both identified check runs and no duplicate status context", merged)
	}
	if got := computeOverallCheckStatus(merged); got != checkConclusionSuccess {
		t.Fatalf("merged state = %q, want check runs to shadow the stale failing status context", got)
	}
	if merged[0].ID != checkRuns[0].ID || merged[1].ID != checkRuns[1].ID {
		t.Fatalf("merged check runs = %#v, want both independent applications", merged)
	}
}
