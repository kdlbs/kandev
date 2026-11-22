package workflowsync

import (
	"github.com/kandev/kandev/internal/reposync"
)

const genericSyncFailureMessage = "Workflow sync failed"
const githubConnectionFailureMessage = reposync.GitHubConnectionFailureMessage

// safeSyncErrorMessage removes provider response bodies before a sync failure
// is persisted or returned.
func safeSyncErrorMessage(err error) string {
	return reposync.SafeErrorMessage(err, genericSyncFailureMessage)
}

// safeStoredSyncErrorMessage sanitizes historical error values read from the
// database, keeping only the exact safe summaries current code writes.
func safeStoredSyncErrorMessage(message string) string {
	return reposync.SafeStoredErrorMessage(message, genericSyncFailureMessage)
}
