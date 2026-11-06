package backups

import (
	"context"
	"testing"
)

type fakeRetentionSources struct {
	published []string
	verified  []string
	restored  []string
	has       bool
}

func (f *fakeRetentionSources) PublishRetentionBackup(_ context.Context, snapshotPath, _ string) error {
	f.published = append(f.published, snapshotPath)
	f.has = true
	return nil
}

func (f *fakeRetentionSources) VerifyRetentionBackup(_ context.Context, snapshotPath, _ string) error {
	f.verified = append(f.verified, snapshotPath)
	return nil
}

func (f *fakeRetentionSources) RestoreRetentionBackup(_ context.Context, snapshotPath string) error {
	f.restored = append(f.restored, snapshotPath)
	return nil
}

func (f *fakeRetentionSources) HasRetentionBackup(string) bool { return f.has }

func TestRetentionPreparationPublishesAndVerifiesCheckpointSources(t *testing.T) {
	s, _ := newTestService(t)
	sources := &fakeRetentionSources{}
	s.RetentionSources = sources
	receipt, err := s.CreateForRetention(context.Background())
	if err != nil {
		t.Fatalf("create retention backup: %v", err)
	}
	if len(sources.published) != 1 {
		t.Fatalf("retention preparation published %d checkpoint sources, want one", len(sources.published))
	}
	if err := s.VerifyRetentionBackupWithSources(context.Background(), receipt); err != nil {
		t.Fatalf("verify combined retention backup: %v", err)
	}
	if len(sources.verified) != 1 {
		t.Fatalf("combined verification checked %d checkpoint sidecars, want one", len(sources.verified))
	}
}

func TestOrdinaryBackupAlsoPublishesCheckpointSources(t *testing.T) {
	s, _ := newTestService(t)
	sources := &fakeRetentionSources{}
	s.RetentionSources = sources
	if _, err := s.createSnapshot(context.Background(), true); err != nil {
		t.Fatalf("create ordinary backup: %v", err)
	}
	if len(sources.published) != 1 {
		t.Fatalf("ordinary backup published %d checkpoint sidecars, want one complete artifact", len(sources.published))
	}
}

func TestRestoreRetentionSourcesSkipsMissingSidecar(t *testing.T) {
	service := &Service{RetentionSources: &fakeRetentionSources{}}
	if err := service.restoreRetentionSources(context.Background(), "snapshot.db"); err != nil {
		t.Fatalf("restore sidecar-free snapshot: %v", err)
	}
	sources := service.RetentionSources.(*fakeRetentionSources)
	if len(sources.verified) != 0 || len(sources.restored) != 0 {
		t.Fatalf("sidecar-free restore verified=%d restored=%d", len(sources.verified), len(sources.restored))
	}
}

func TestRestoreRetentionSourcesVerifiesPublishedSidecar(t *testing.T) {
	sources := &fakeRetentionSources{has: true}
	service := &Service{RetentionSources: sources}
	if err := service.restoreRetentionSources(context.Background(), "snapshot.db"); err != nil {
		t.Fatalf("restore published sidecar: %v", err)
	}
	if len(sources.verified) != 1 || len(sources.restored) != 1 {
		t.Fatalf("published restore verified=%d restored=%d", len(sources.verified), len(sources.restored))
	}
}
