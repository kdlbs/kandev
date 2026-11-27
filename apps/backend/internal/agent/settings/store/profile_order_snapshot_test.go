package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

func TestGetAgentProfileOrderSnapshotsKeepsRowsAndRevisionFromOneWALSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile-order.db")
	dsn := path + "?_journal_mode=WAL&_busy_timeout=5000"
	writer, err := sqlx.Open("sqlite3", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	reader, err := sqlx.Open("sqlite3", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	writerRepo, err := newSQLiteRepository(writer, writer, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	readerRepo, err := newSQLiteRepository(writer, reader, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	agent := &models.Agent{Name: "snapshot-race"}
	if err := writerRepo.CreateAgent(ctx, agent); err != nil {
		t.Fatal(err)
	}
	first := &models.AgentProfile{AgentID: agent.ID, Name: "first", Model: "model"}
	second := &models.AgentProfile{AgentID: agent.ID, Name: "second", Model: "model"}
	for _, profile := range []*models.AgentProfile{first, second} {
		if err := writerRepo.CreateAgentProfile(ctx, profile); err != nil {
			t.Fatal(err)
		}
	}
	baseline, err := writerRepo.ListAgentProfiles(ctx, agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	oldOrder := []string{baseline[1].ID, baseline[0].ID}
	newOrder := []string{baseline[0].ID, baseline[1].ID}
	if revision, changed, err := writerRepo.ReorderAgentProfiles(ctx, agent.ID, oldOrder); err != nil || !changed || revision != 1 {
		t.Fatalf("initial order revision %d changed %t err %v, want revision 1 changed", revision, changed, err)
	}
	readAtProfiles := make(chan struct{})
	releaseReader := make(chan struct{})
	type result struct {
		snapshot map[string]AgentProfileOrderSnapshot
		err      error
	}
	readDone := make(chan result, 1)
	go func() {
		snapshot, err := readerRepo.getAgentProfileOrderSnapshots(ctx, []string{agent.ID}, func() error {
			close(readAtProfiles)
			<-releaseReader
			return nil
		})
		readDone <- result{snapshot: snapshot, err: err}
	}()
	<-readAtProfiles
	if revision, changed, err := writerRepo.ReorderAgentProfiles(ctx, agent.ID, newOrder); err != nil || !changed || revision != 2 {
		close(releaseReader)
		t.Fatalf("concurrent order revision %d changed %t err %v, want revision 2 changed", revision, changed, err)
	}
	close(releaseReader)
	old := <-readDone
	if old.err != nil {
		t.Fatal(old.err)
	}
	oldSnapshot := old.snapshot[agent.ID]
	if oldSnapshot.Revision != 1 || len(oldSnapshot.Profiles) != 2 || oldSnapshot.Profiles[0].ID != oldOrder[0] || oldSnapshot.Profiles[1].ID != oldOrder[1] {
		t.Fatalf("in-flight snapshot = revision %d profiles %#v, want revision 1 with the original order", oldSnapshot.Revision, oldSnapshot.Profiles)
	}
	fresh, err := readerRepo.GetAgentProfileOrderSnapshots(ctx, []string{agent.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got := fresh[agent.ID]; got.Revision != 2 || got.Profiles[0].ID != newOrder[0] || got.Profiles[1].ID != newOrder[1] {
		t.Fatalf("next snapshot = revision %d profiles %#v, want revision 2 with the committed order", got.Revision, got.Profiles)
	}
}
