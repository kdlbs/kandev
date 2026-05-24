package share

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/github"
)

func sampleSnapshot() *Snapshot {
	completed := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	return &Snapshot{
		Version:    SnapshotVersion,
		ExportedAt: completed,
		Task:       TaskMeta{Title: "Fix flaky test"},
		Session: SessionMeta{
			AgentType:    "claude-acp",
			Model:        "claude-opus-4-7",
			ExecutorType: "local_docker",
			StartedAt:    completed.Add(-time.Minute),
			CompletedAt:  &completed,
		},
		Messages: []Message{
			{Role: "user", Ts: completed, Blocks: []Block{{Kind: "text", Text: "hi"}}},
		},
		Redaction: RedactionLog{AppliedRules: []string{}},
	}
}

func TestGistBackend_Upload_CreatesSecretGistWithExpectedFiles(t *testing.T) {
	t.Parallel()
	mock := github.NewMockClient()
	b := NewGistBackend(mock)

	id, url, err := b.Upload(context.Background(), sampleSnapshot())
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if id == "" || url == "" {
		t.Fatalf("expected non-empty id/url, got %q / %q", id, url)
	}

	// The returned URL is the gist.githack.com raw-render URL, NOT the
	// raw gist URL — githack proxies the full file content so big shares
	// render even when GitHub's gists API would have returned an empty
	// content field.
	if !strings.HasPrefix(url, "https://gist.githack.com/") ||
		!strings.HasSuffix(url, "/raw/share.html") {
		t.Fatalf("expected gist.githack.com /raw/share.html URL, got %q", url)
	}

	gists := mock.Gists()
	g, ok := gists[id]
	if !ok {
		t.Fatalf("gist %s not stored on mock", id)
	}
	if g.Public {
		t.Fatal("backend should always create secret gists (Public=false)")
	}
	for _, name := range []string{"snapshot.json", "README.md", "share.html"} {
		if _, ok := g.Files[name]; !ok {
			t.Fatalf("missing %s in gist files", name)
		}
	}
	if !strings.Contains(g.Files["snapshot.json"].Content, `"title": "Fix flaky test"`) {
		t.Fatalf("snapshot.json does not contain task title: %s", g.Files["snapshot.json"].Content)
	}
	if !strings.Contains(g.Files["README.md"].Content, "# Fix flaky test") {
		t.Fatal("README.md missing task title heading")
	}
	if !strings.Contains(g.Files["share.html"].Content, "<!doctype html>") {
		t.Fatal("share.html missing doctype")
	}
	if !strings.Contains(g.Files["share.html"].Content, "Fix flaky test") {
		t.Fatal("share.html missing task title")
	}
}

func TestGistBackend_Upload_RejectsOversizedSnapshot(t *testing.T) {
	t.Parallel()
	mock := github.NewMockClient()
	b := NewGistBackend(mock)

	snap := sampleSnapshot()
	big := strings.Repeat("x", GistMaxBytes+1)
	snap.Messages = []Message{{Role: "user", Blocks: []Block{{Kind: "text", Text: big}}}}

	_, _, err := b.Upload(context.Background(), snap)
	if err == nil {
		t.Fatal("expected ErrSnapshotTooLarge, got nil")
	}
	if !errors.Is(err, ErrSnapshotTooLarge) {
		t.Fatalf("expected ErrSnapshotTooLarge, got %v", err)
	}
	if len(mock.Gists()) != 0 {
		t.Fatalf("no gist should have been created, got %d", len(mock.Gists()))
	}
}

func TestGistBackend_Delete_Success(t *testing.T) {
	t.Parallel()
	mock := github.NewMockClient()
	b := NewGistBackend(mock)
	id, _, err := b.Upload(context.Background(), sampleSnapshot())
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if err := b.Delete(context.Background(), id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if got := mock.DeletedGists(); len(got) != 1 || got[0] != id {
		t.Fatalf("expected DeletedGists=[%q], got %v", id, got)
	}
}

func TestGistBackend_Delete_PassesThroughNotFoundForCallerInspection(t *testing.T) {
	t.Parallel()
	mock := github.NewMockClient()
	b := NewGistBackend(mock)

	err := b.Delete(context.Background(), "missing-id")
	if err == nil {
		t.Fatal("expected error for missing gist")
	}
	if !IsAlreadyGone(err) {
		t.Fatalf("expected IsAlreadyGone to be true, got err=%v", err)
	}
}
