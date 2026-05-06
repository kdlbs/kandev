package service

import (
	"context"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/summary"
)

// summaryLoadInputs adapts the office sqlite repository to the slim
// summary.Repo interface and forwards the call. Kept as a thin
// indirection so the event subscriber doesn't import the summary
// package's helpers directly — easier to stub in tests.
func summaryLoadInputs(
	ctx context.Context, repo *sqlite.Repository, run *models.Run, agentID, scope string,
) (summary.BuildInputs, error) {
	if run == nil {
		return summary.BuildInputs{}, nil
	}
	return summary.LoadInputs(
		ctx, repo, repo.ReaderDB(),
		summary.RunSnapshot{
			ID:         run.ID,
			Status:     run.Status,
			ResultJSON: run.ResultJSON,
		},
		agentID, scope,
	)
}

// summaryBuild forwards to summary.BuildSummary so the event
// subscriber callsite reads as one verb.
func summaryBuild(in summary.BuildInputs) string {
	return summary.BuildSummary(in)
}
