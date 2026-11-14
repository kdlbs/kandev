package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/agent/settings/models"
	"testing"
)

func TestExecutionGradeStaleWriterPreservesCurrentColumn(t *testing.T) {
	repo := newFreshRepo(t)
	ctx := context.Background()
	id := seedAgentProfile(t, repo, "Original", "grade-agent")
	stale, err := repo.GetAgentProfile(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.Exec(`UPDATE agent_profiles SET execution_grade = ? WHERE id = ?`, "New-Grade", id); err != nil {
		t.Fatal(err)
	}
	stale.Name = "Renamed"
	if err := repo.UpdateAgentProfileWithEnabledIntent(ctx, stale, nil); err != nil {
		t.Fatal(err)
	}
	saved, err := repo.GetAgentProfile(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{stale, saved} {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		if got["execution_grade"] != "New-Grade" {
			t.Fatalf("stale write lost grade: %s", raw)
		}
	}
	if saved.Name != "Renamed" || saved.Model != stale.Model {
		t.Fatal("unrelated fields changed")
	}
}

func TestExecutionGradeLegacyMigrationDefault(t *testing.T) {
	db := newLegacyDB(t)
	repo, err := newSQLiteRepositoryWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	id := seedAgentProfile(t, repo, "Legacy", "grade-agent")
	var grade string
	if err := db.Get(&grade, `SELECT execution_grade FROM agent_profiles WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	if grade != "" {
		t.Fatalf("legacy grade=%q", grade)
	}
	if err := repo.initSchema(); err != nil {
		t.Fatal(err)
	}
}

func TestExecutionGradeLegacyRebuildPreservesExisting(t *testing.T) {
	db := newLegacyDB(t)
	if _, err := db.Exec(`ALTER TABLE agent_profiles ADD COLUMN execution_grade TEXT NOT NULL DEFAULT ''`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO agents (id,name,created_at,updated_at) VALUES ('a1','grade-agent',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO agent_profiles (id,agent_id,name,agent_display_name,model,execution_grade,created_at,updated_at)
 VALUES ('p1','a1','Existing','Agent','original-model','Existing-Grade',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	repo, err := newSQLiteRepositoryWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		saved, err := repo.GetAgentProfile(context.Background(), "p1")
		if err != nil {
			t.Fatal(err)
		}
		if saved.ExecutionGrade != "Existing-Grade" || saved.Model != "original-model" || saved.Name != "Existing" {
			t.Fatalf("legacy row lost: %#v", saved)
		}
		if err := repo.initSchema(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestExecutionGradeDuplicateRejectsChangedSource(t *testing.T) {
	repo := newFreshRepo(t)
	ctx := context.Background()
	id := seedAgentProfile(t, repo, "Original", "grade-agent")
	source, err := repo.GetAgentProfile(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	// Use a distinct revision without depending on the host clock resolution.
	if _, err := repo.db.Exec(`UPDATE agent_profiles SET execution_grade = ?, updated_at = ? WHERE id = ?`, "Changed", source.UpdatedAt.Add(time.Second), id); err != nil {
		t.Fatal(err)
	}
	clone := &models.AgentProfile{AgentID: source.AgentID, Name: "Copy", ExecutionGrade: source.ExecutionGrade}
	err = repo.DuplicateAgentProfile(ctx, DuplicateAgentProfileInput{Source: source, Profile: clone})
	if !errors.Is(err, ErrProfileChanged) {
		t.Fatalf("changed grade source error=%v", err)
	}
	var copies int
	if err := repo.db.Get(&copies, `SELECT COUNT(*) FROM agent_profiles WHERE name = ?`, "Copy"); err != nil {
		t.Fatal(err)
	}
	if copies != 0 {
		t.Fatal("stale grade copy survived failed transaction")
	}
}
