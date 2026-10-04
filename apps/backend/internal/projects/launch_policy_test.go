package projects

import (
	"context"
	"errors"
	"testing"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

func TestResolveTaskLaunchRetainsExistingCoordinatorSessionProfileAfterProjectEdit(t *testing.T) {
	ctx := context.Background()
	svc, _ := projectFixture(t, true)
	project, err := svc.Create(ctx, validCreateRequest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	profileB := "profile-b"
	updated, err := svc.Update(ctx, project.WorkspaceID, project.ID, UpdateRequest{
		Revision: project.Revision, CoordinatorProfileID: &profileB,
	})
	if err != nil {
		t.Fatalf("Update coordinator profile to B: %v", err)
	}
	profileA := "profile-coordinator"
	if _, err := svc.Update(ctx, project.WorkspaceID, project.ID, UpdateRequest{
		Revision: updated.Revision, CoordinatorProfileID: &profileA,
	}); err != nil {
		t.Fatalf("Update coordinator profile back to A: %v", err)
	}

	task := svc.tasks.(*projectTaskStub).created[project.MainTaskID]
	session := &models.TaskSession{ID: "session-b", TaskID: task.ID, AgentProfileID: profileB}
	resolved, _, _, err := svc.ResolveTaskLaunch(ctx, task, session, "", "", "")
	if err != nil {
		t.Fatalf("ResolveTaskLaunch for existing profile-B session: %v", err)
	}
	if resolved != profileB {
		t.Fatalf("resolved profile = %q, want existing session profile %q", resolved, profileB)
	}
}

func TestResolveTaskLaunchUsesCurrentCoordinatorProfileForNewSession(t *testing.T) {
	ctx := context.Background()
	svc, _ := projectFixture(t, true)
	project, err := svc.Create(ctx, validCreateRequest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	profileB := "profile-b"
	if _, err := svc.Update(ctx, project.WorkspaceID, project.ID, UpdateRequest{
		Revision: project.Revision, CoordinatorProfileID: &profileB,
	}); err != nil {
		t.Fatalf("Update coordinator profile to B: %v", err)
	}

	task := svc.tasks.(*projectTaskStub).created[project.MainTaskID]
	resolved, _, _, err := svc.ResolveTaskLaunch(ctx, task, nil, task.AgentProjectProfileID, "", "")
	if err != nil {
		t.Fatalf("ResolveTaskLaunch for a new session with the old task profile: %v", err)
	}
	if resolved != profileB {
		t.Fatalf("resolved profile = %q, want current coordinator profile %q", resolved, profileB)
	}
}

func TestResolveTaskLaunchRejectsInvalidExistingAndRequestedProfiles(t *testing.T) {
	ctx := context.Background()
	svc, _ := projectFixture(t, true)
	project, err := svc.Create(ctx, validCreateRequest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	profileB := "profile-b"
	updated, err := svc.Update(ctx, project.WorkspaceID, project.ID, UpdateRequest{
		Revision: project.Revision, CoordinatorProfileID: &profileB,
	})
	if err != nil {
		t.Fatalf("Update coordinator profile to B: %v", err)
	}
	profileA := "profile-coordinator"
	if _, err := svc.Update(ctx, project.WorkspaceID, project.ID, UpdateRequest{
		Revision: updated.Revision, CoordinatorProfileID: &profileA,
	}); err != nil {
		t.Fatalf("Update coordinator profile back to A: %v", err)
	}
	task := svc.tasks.(*projectTaskStub).created[project.MainTaskID]

	tests := []struct {
		name    string
		session *models.TaskSession
		profile string
	}{
		{
			name:    "session belongs to another task",
			session: &models.TaskSession{ID: "other-session", TaskID: "other-task", AgentProfileID: "profile-coordinator"},
			profile: "profile-coordinator",
		},
		{
			name:    "request cannot replace stored session profile with current profile",
			session: &models.TaskSession{ID: "coordinator-session", TaskID: task.ID, AgentProfileID: profileB},
			profile: "profile-coordinator",
		},
		{
			name:    "new session cannot request an arbitrary profile",
			profile: "profile-arbitrary",
		},
		{
			name:    "new session cannot select a prior coordinator profile",
			profile: profileB,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, _, err := svc.ResolveTaskLaunch(ctx, task, tt.session, tt.profile, "", ""); !errors.Is(err, taskservice.ErrAgentProjectTaskForbidden) {
				t.Fatalf("ResolveTaskLaunch error = %v, want ErrAgentProjectTaskForbidden", err)
			}
		})
	}
}

func TestResolveTaskLaunchKeepsWorkerProfilePinned(t *testing.T) {
	ctx := context.Background()
	svc, _ := projectFixture(t, true)
	project, err := svc.Create(ctx, validCreateRequest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	worker := &models.Task{
		ID: "worker-1", WorkspaceID: project.WorkspaceID, ParentID: project.MainTaskID,
		AgentProjectID: project.ID, AgentProjectTier: models.AgentProjectTierEconomy,
		AgentProjectProfileID: project.EconomyProfileID,
	}

	resolved, _, _, err := svc.ResolveTaskLaunch(ctx, worker, nil, "", "", "")
	if err != nil {
		t.Fatalf("ResolveTaskLaunch for pinned worker profile: %v", err)
	}
	if resolved != project.EconomyProfileID {
		t.Fatalf("worker profile = %q, want pinned profile %q", resolved, project.EconomyProfileID)
	}

	session := &models.TaskSession{ID: "worker-session", TaskID: worker.ID, AgentProfileID: "profile-other"}
	if _, _, _, err := svc.ResolveTaskLaunch(ctx, worker, session, session.AgentProfileID, "", ""); !errors.Is(err, taskservice.ErrAgentProjectTaskForbidden) {
		t.Fatalf("ResolveTaskLaunch for replaced worker profile = %v, want ErrAgentProjectTaskForbidden", err)
	}
}

func TestResolveTaskLaunchRejectsUnavailablePinnedCoordinatorProfile(t *testing.T) {
	ctx := context.Background()
	svc, _ := projectFixture(t, true)
	project, err := svc.Create(ctx, validCreateRequest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	profileB := "profile-b"
	updated, err := svc.Update(ctx, project.WorkspaceID, project.ID, UpdateRequest{
		Revision: project.Revision, CoordinatorProfileID: &profileB,
	})
	if err != nil {
		t.Fatalf("Update coordinator profile to B: %v", err)
	}
	profileA := "profile-coordinator"
	if _, err := svc.Update(ctx, project.WorkspaceID, project.ID, UpdateRequest{
		Revision: updated.Revision, CoordinatorProfileID: &profileA,
	}); err != nil {
		t.Fatalf("Update coordinator profile back to A: %v", err)
	}
	svc.profiles = launchPolicyProfileStub{
		workspaceID: "workspace-1",
		unavailable: map[string]bool{profileB: true},
	}

	task := svc.tasks.(*projectTaskStub).created[project.MainTaskID]
	session := &models.TaskSession{ID: "session-b", TaskID: task.ID, AgentProfileID: profileB}
	if _, _, _, err := svc.ResolveTaskLaunch(ctx, task, session, profileB, "", ""); !errors.Is(err, ErrDependenciesMissing) {
		t.Fatalf("ResolveTaskLaunch for unavailable pinned profile = %v, want ErrDependenciesMissing", err)
	}
}

type launchPolicyProfileStub struct {
	workspaceID string
	unavailable map[string]bool
}

func (s launchPolicyProfileStub) GetAgentProfile(_ context.Context, id string) (*settingsmodels.AgentProfile, error) {
	return &settingsmodels.AgentProfile{
		ID: id, AgentID: "claude-acp", WorkspaceID: s.workspaceID, Enabled: !s.unavailable[id],
	}, nil
}
