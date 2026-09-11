package coordinator

import (
	"errors"
	"strings"
	"time"
)

// HostOperation is the small, server-owned operation vocabulary eligible for
// a mediated execution lease. It is intentionally not a shell command.
type HostOperation string

const (
	HostOperationGitPushFastForward  HostOperation = "git_push_fast_forward"
	HostOperationGitIndexUpdate      HostOperation = "git_index_update"
	HostOperationGitMergeFastForward HostOperation = "git_merge_fast_forward"
)

var ErrHostExecutionLeaseDenied = errors.New("host execution lease denied")

// HostExecutionLeaseScope is the complete server-owned identity binding for a
// single mediated operation. The Host derives every field from durable task
// and repository records; task agents never supply paths, credentials, or a
// command line.
type HostExecutionLeaseScope struct {
	GrantID      string
	PrincipalID  string
	WorkspaceID  string
	TargetTaskID string
	RepositoryID string
	Branch       string
	ExpectedHead string
	Operation    HostOperation
}

// HostExecutionLease is an opaque Host record. Its ID is only a receipt
// reference: a Host must persist and consume it atomically, rather than accept
// a serialized lease from an agent as proof of authority.
type HostExecutionLease struct {
	ID string
	HostExecutionLeaseScope
	ExpiresAt time.Time
	RevokedAt *time.Time
}

// ValidateFor denies by default. It is the shared contract guard for the Host
// before it consumes a persisted lease and starts the bounded operation.
func (lease HostExecutionLease) ValidateFor(request HostExecutionLeaseScope, now time.Time) error {
	if lease.ID == "" || lease.RevokedAt != nil || lease.ExpiresAt.IsZero() || !lease.ExpiresAt.After(now) {
		return ErrHostExecutionLeaseDenied
	}
	if !validHostExecutionLeaseScope(lease.HostExecutionLeaseScope) || !validHostExecutionLeaseScope(request) {
		return ErrHostExecutionLeaseDenied
	}
	if lease.HostExecutionLeaseScope != request {
		return ErrHostExecutionLeaseDenied
	}
	return nil
}

func validHostExecutionLeaseScope(scope HostExecutionLeaseScope) bool {
	if scope.GrantID == "" || scope.PrincipalID == "" || scope.WorkspaceID == "" || scope.TargetTaskID == "" || scope.RepositoryID == "" || scope.Branch == "" || !validGitObjectID(scope.ExpectedHead) {
		return false
	}
	switch scope.Operation {
	case HostOperationGitPushFastForward, HostOperationGitIndexUpdate, HostOperationGitMergeFastForward:
		return true
	default:
		return false
	}
}

func validGitObjectID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	return strings.Trim(value, "0123456789abcdefABCDEF") == ""
}
