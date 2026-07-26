package share

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/github"
)

type recordingGitHubResolver struct {
	clients    map[string]github.Client
	workspaces []string
	allowNil   bool
}

func (r *recordingGitHubResolver) ResolveGitHubAutomationClient(
	_ context.Context,
	workspaceID string,
) (github.Client, error) {
	r.workspaces = append(r.workspaces, workspaceID)
	client := r.clients[workspaceID]
	if client == nil && !r.allowNil {
		return nil, errors.New("workspace not connected")
	}
	return client, nil
}

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

	id, url, err := b.Upload(context.Background(), "workspace-1", sampleSnapshot())
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

	_, _, err := b.Upload(context.Background(), "workspace-1", snap)
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
	id, _, err := b.Upload(context.Background(), "workspace-1", sampleSnapshot())
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if err := b.Delete(context.Background(), "workspace-1", id); err != nil {
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

	err := b.Delete(context.Background(), "workspace-1", "missing-id")
	if err == nil {
		t.Fatal("expected error for missing gist")
	}
	if !IsAlreadyGone(err) {
		t.Fatalf("expected IsAlreadyGone to be true, got err=%v", err)
	}
}

func TestGistBackend_ResolvesOwningWorkspaceForEveryOperation(t *testing.T) {
	t.Parallel()
	first := github.NewMockClient()
	second := github.NewMockClient()
	resolver := &recordingGitHubResolver{clients: map[string]github.Client{
		"workspace-1": first,
		"workspace-2": second,
	}}
	backend := NewWorkspaceGistBackend(resolver)
	firstID, _, err := backend.Upload(context.Background(), "workspace-1", sampleSnapshot())
	if err != nil {
		t.Fatalf("workspace-1 upload: %v", err)
	}
	if _, _, err := backend.Upload(context.Background(), "workspace-2", sampleSnapshot()); err != nil {
		t.Fatalf("workspace-2 upload: %v", err)
	}
	if err := backend.Delete(context.Background(), "workspace-1", firstID); err != nil {
		t.Fatalf("workspace-1 delete: %v", err)
	}
	if len(first.Gists()) != 0 || len(second.Gists()) != 1 ||
		len(resolver.workspaces) != 3 || resolver.workspaces[2] != "workspace-1" {
		t.Fatalf("resolver workspaces = %v, first gists = %d, second gists = %d",
			resolver.workspaces, len(first.Gists()), len(second.Gists()))
	}
}

func TestGistBackend_FailsClosedWhenResolverReturnsNoClient(t *testing.T) {
	t.Parallel()
	backend := NewWorkspaceGistBackend(&recordingGitHubResolver{
		clients:  map[string]github.Client{},
		allowNil: true,
	})
	if _, _, err := backend.Upload(context.Background(), "workspace-1", sampleSnapshot()); err == nil {
		t.Fatal("expected upload to fail when the workspace resolver returns no client")
	}
}
