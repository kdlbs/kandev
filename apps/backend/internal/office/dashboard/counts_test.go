package dashboard_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/office/models"
)

// stubSkillLister is a test double for dashboard.SkillLister.
type stubSkillLister struct {
	skills []*models.Skill
}

func (s *stubSkillLister) ListSkills(_ context.Context, _ string) ([]*models.Skill, error) {
	return s.skills, nil
}

// stubRoutineLister is a test double for dashboard.RoutineLister.
type stubRoutineLister struct {
	routines []*models.Routine
}

func (r *stubRoutineLister) ListRoutines(_ context.Context, _ string) ([]*models.Routine, error) {
	return r.routines, nil
}

func TestDashboard_TaskSkillRoutineCounts(t *testing.T) {
	deps := newTestDeps(t)
	db := deps.db

	// Insert 3 non-ephemeral tasks for ws-count.
	for _, id := range []string{"task1", "task2", "task3"} {
		insertTestTask(t, db, id, "ws-count", id, "todo", 0)
	}
	// Insert an ephemeral task — should not be counted.
	_, err := db.Exec(`INSERT INTO tasks
		(id, workspace_id, title, state, priority, identifier, is_ephemeral, created_at, updated_at)
		VALUES ('ephem', 'ws-count', 'ephemeral', 'todo', 'medium', 'ephem', 1, datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert ephemeral task: %v", err)
	}

	// Wire stub skill/routine listers.
	deps.svc.SetSkillLister(&stubSkillLister{
		skills: []*models.Skill{{ID: "s1"}, {ID: "s2"}},
	})
	deps.svc.SetRoutineLister(&stubRoutineLister{
		routines: []*models.Routine{
			{ID: "r1", Status: "active"},
			{ID: "r2", Status: "paused"},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/ws-count/dashboard", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp dashboard.DashboardResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.TaskCount != 3 {
		t.Errorf("task_count: got %d, want 3", resp.TaskCount)
	}
	if resp.SkillCount != 2 {
		t.Errorf("skill_count: got %d, want 2", resp.SkillCount)
	}
	if resp.RoutineCount != 1 {
		t.Errorf("routine_count: got %d, want 1 (active only)", resp.RoutineCount)
	}
}

func TestDashboard_CountsZeroWhenNoListers(t *testing.T) {
	deps := newTestDeps(t)

	// No skill/routine listers wired — counts should be 0.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/ws-empty/dashboard", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp dashboard.DashboardResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.SkillCount != 0 || resp.RoutineCount != 0 {
		t.Errorf("expected zero skill/routine counts without listers, got skill=%d routine=%d",
			resp.SkillCount, resp.RoutineCount)
	}
}
