package dashboard

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// refusalRepo fails the test if the store is reached at all. The embedded
// interface supplies the full method set; only the participant write is
// overridden, so any other call would panic on the nil interface rather
// than pass silently.
type refusalRepo struct {
	Repository
	t *testing.T
}

func (r *refusalRepo) AddTaskParticipant(
	_ context.Context, taskID, agentID, role string,
) (sqlite.ParticipantWriteResult, error) {
	r.t.Helper()
	r.t.Fatalf("store reached for role %q (task %q, agent %q): an unsupported role must be refused "+
		"above the store, writing and claiming nothing", role, taskID, agentID)
	return sqlite.ParticipantWriteResult{}, nil
}

// TestAddOrRemoveParticipant_RefusesUnsupportedRole covers
// AC-OFFICE-SEAT-ASSURANCE-002.5, pinning
// AC-OFFICE-SEAT-PROVENANCE-005.7.
//
// This refusal is not reachable over HTTP, which is why the case is
// white-box rather than a request. The routes are role-fixed: each endpoint
// names its own role and the service exposes only reviewer and approver
// entry points, so no request can carry a bad role. The refusal that
// actually exists is the participantFields lookup miss inside the shared
// body those entry points call, and this exercises the same refusal an
// internal caller — a future agent tool, scheduler or fixup path — would
// meet.
//
// It calls beneath the HTTP surface but above the permission check that
// body performs, so it bypasses no authorization.
//
// A structural assertion over participantFields would be the weaker choice:
// it would pass against a body that consulted the map and then ignored it.
func TestAddOrRemoveParticipant_RefusesUnsupportedRole(t *testing.T) {
	svc := &DashboardService{repo: &refusalRepo{t: t}}

	for _, role := range []string{"watcher", "collaborator", "runner", "", "REVIEWER"} {
		t.Run("role="+role, func(t *testing.T) {
			err := svc.addOrRemoveParticipant(context.Background(), "", "task-1", "agent-1", role, true)
			if err == nil {
				t.Fatalf("addOrRemoveParticipant(role=%q) = nil, want an error: only reviewer and "+
					"approver may be seated through this surface", role)
			}
		})
	}
}

// TestAddOrRemoveParticipant_AcceptsTheSupportedRoles is the other half of
// the boundary: the two roles the surface does expose must get past the
// lookup and reach the store. Without this, a body that refused every role
// would satisfy the case above.
func TestAddOrRemoveParticipant_AcceptsTheSupportedRoles(t *testing.T) {
	for _, role := range []string{"reviewer", "approver"} {
		t.Run("role="+role, func(t *testing.T) {
			reached := false
			svc := &DashboardService{repo: &reachedRepo{reached: &reached}}
			if err := svc.addOrRemoveParticipant(
				context.Background(), "", "task-1", "agent-1", role, true); err != nil {
				t.Fatalf("addOrRemoveParticipant(role=%q) = %v, want the store to be reached", role, err)
			}
			if !reached {
				t.Fatalf("role %q never reached the store", role)
			}
		})
	}
}

// reachedRepo records that the participant write was called, and reports an
// outcome that earns no post-commit side effects so the zero-value service's
// optional collaborators are never consulted.
type reachedRepo struct {
	Repository
	reached *bool
}

func (r *reachedRepo) AddTaskParticipant(
	_ context.Context, _, _, _ string,
) (sqlite.ParticipantWriteResult, error) {
	*r.reached = true
	return sqlite.ParticipantWriteResult{Outcome: sqlite.ParticipantWriteOutcomeUnchanged}, nil
}
