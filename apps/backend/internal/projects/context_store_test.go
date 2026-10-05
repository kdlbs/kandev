package projects

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
)

func TestContextStoreProvisionAndHashCheckedWrite(t *testing.T) {
	store := NewContextStore(filepath.Join(t.TempDir(), "agent-projects"))
	projectID := uuid.NewString()
	root, err := store.Provision(context.Background(), projectID)
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(root, "notes.md")); err != nil || string(got) != starterNotesContent {
		t.Fatalf("initial notes = %q, err = %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(root, "index.md")); err != nil || string(got) != starterIndexContent {
		t.Fatalf("initial index = %q, err = %v", got, err)
	}

	content, hash, err := store.ReadFile(projectID, "notes.md")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	newHash, err := store.WriteFile(projectID, "notes.md", hash, []byte("shared note\n"))
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if newHash == hash || content != starterNotesContent {
		t.Fatalf("content/hash = %q/%q, previous hash %q", content, newHash, hash)
	}
	if _, err := store.WriteFile(projectID, "notes.md", hash, []byte("stale write\n")); !errors.Is(err, ErrContextConflict) {
		t.Fatalf("stale WriteFile error = %v, want ErrContextConflict", err)
	}
	if got, err := os.ReadFile(filepath.Join(root, "notes.md")); err != nil || string(got) != "shared note\n" {
		t.Fatalf("notes after stale write = %q, err = %v", got, err)
	}
}

func TestContextStoreRejectsTraversalAndSymlinkComponents(t *testing.T) {
	store := NewContextStore(filepath.Join(t.TempDir(), "agent-projects"))
	projectID := uuid.NewString()
	root, err := store.Provision(context.Background(), projectID)
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if _, _, err := store.ReadFile(projectID, "../notes.md"); !errors.Is(err, ErrInvalidContextPath) {
		t.Fatalf("traversal ReadFile error = %v, want ErrInvalidContextPath", err)
	}
	escape := t.TempDir()
	if err := os.WriteFile(filepath.Join(escape, "secret.md"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(escape, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.ReadFile(projectID, "linked/secret.md"); !errors.Is(err, ErrInvalidContextPath) {
		t.Fatalf("symlink ReadFile error = %v, want ErrInvalidContextPath", err)
	}
}

func TestContextStoreReadAndWriteStayWithinRootAfterDirectorySymlinkSwap(t *testing.T) {
	storageRoot := t.TempDir()
	store := NewContextStore(storageRoot)
	projectID := uuid.NewString()
	contextRoot, err := store.Provision(context.Background(), projectID)
	if err != nil {
		t.Fatal(err)
	}
	docs := filepath.Join(contextRoot, "docs")
	if err := os.Mkdir(docs, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "notes.md"), []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}
	external := t.TempDir()
	if err := os.WriteFile(filepath.Join(external, "notes.md"), []byte("outside secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	resolved, _, err := store.resolvePath(projectID, "docs/notes.md", false)
	if err != nil {
		t.Fatalf("resolve before swap: %v", err)
	}
	if err := os.RemoveAll(docs); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, docs); err != nil {
		t.Fatal(err)
	}

	if data, err := store.readResolvedFile(resolved); err == nil {
		t.Fatalf("read after directory swap returned %q, want rejection", data)
	} else if !errors.Is(err, ErrInvalidContextPath) {
		t.Fatalf("read after directory swap error = %v, want ErrInvalidContextPath", err)
	}
	if _, err := store.writeResolvedFile(resolved, "", []byte("overwrite")); !errors.Is(err, ErrInvalidContextPath) {
		t.Fatalf("write after directory swap error = %v, want ErrInvalidContextPath", err)
	}
	if got, err := os.ReadFile(filepath.Join(external, "notes.md")); err != nil || string(got) != "outside secret" {
		t.Fatalf("external file after swap = %q, err %v", got, err)
	}
}

func TestContextStoreListsWithoutFollowingSymlinks(t *testing.T) {
	store := NewContextStore(filepath.Join(t.TempDir(), "agent-projects"))
	projectID := uuid.NewString()
	root, err := store.Provision(context.Background(), projectID)
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "docs"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "guide.md"), []byte("guide"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "outside")); err != nil {
		t.Fatal(err)
	}
	entries, err := store.List(projectID, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 3 || entries[0].Name != "docs" || entries[1].Name != "index.md" || entries[2].Name != "notes.md" {
		t.Fatalf("List entries = %#v, want docs, index.md, and notes.md", entries)
	}
}

const (
	starterIndexContent = "---\nokf_version: \"0.2\"\n---\n# Project context\n\n- [Project notes](notes.md): Current status, decisions, and handoffs.\n"
	starterNotesContent = "---\ntype: Project Notes\ntitle: Project notes\ndescription: Current status, decisions, and handoffs for this project.\n---\n# Project notes\n\n## Current status\n\n## Decisions\n\n## Handoffs\n"
)

func TestContextStoreProvisionCreatesOKFStarterFiles(t *testing.T) {
	store := NewContextStore(filepath.Join(t.TempDir(), "agent-projects"))
	projectID := uuid.NewString()
	root, err := store.Provision(context.Background(), projectID)
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}

	for name, want := range map[string]string{"index.md": starterIndexContent, "notes.md": starterNotesContent} {
		got, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestContextStoreProvisionPreservesExistingFiles(t *testing.T) {
	tests := []struct {
		name    string
		initial map[string]string
		want    map[string]string
	}{
		{
			name:    "empty notes and missing index",
			initial: map[string]string{"notes.md": ""},
			want:    map[string]string{"index.md": starterIndexContent, "notes.md": ""},
		},
		{
			name:    "custom index and missing notes",
			initial: map[string]string{"index.md": "custom index\r\n"},
			want:    map[string]string{"index.md": "custom index\r\n", "notes.md": starterNotesContent},
		},
		{
			name:    "custom files",
			initial: map[string]string{"index.md": "custom index\n", "notes.md": "custom notes\n"},
			want:    map[string]string{"index.md": "custom index\n", "notes.md": "custom notes\n"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewContextStore(filepath.Join(t.TempDir(), "agent-projects"))
			projectID := uuid.NewString()
			root, err := store.ContextPath(projectID)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(root, 0o700); err != nil {
				t.Fatal(err)
			}
			for name, content := range tt.initial {
				if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			provisioned, err := store.Provision(context.Background(), projectID)
			if err != nil {
				t.Fatalf("Provision: %v", err)
			}
			if provisioned != root {
				t.Fatalf("Provision root = %q, want %q", provisioned, root)
			}
			if _, err := store.Provision(context.Background(), projectID); err != nil {
				t.Fatalf("repeated Provision: %v", err)
			}

			for name, want := range tt.want {
				got, err := os.ReadFile(filepath.Join(root, name))
				if err != nil {
					t.Errorf("read %s: %v", name, err)
					continue
				}
				if string(got) != want {
					t.Errorf("%s = %q, want preserved %q", name, got, want)
				}
			}
		})
	}
}

func TestContextStoreProvisionConcurrentCreators(t *testing.T) {
	store := NewContextStore(filepath.Join(t.TempDir(), "agent-projects"))
	projectID := uuid.NewString()
	const callers = 8
	start := make(chan struct{})
	errs := make(chan error, callers)
	for range callers {
		go func() {
			<-start
			_, err := store.Provision(context.Background(), projectID)
			errs <- err
		}()
	}
	close(start)
	for range callers {
		if err := <-errs; err != nil {
			t.Errorf("concurrent Provision: %v", err)
		}
	}

	root, err := store.ContextPath(projectID)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"index.md": starterIndexContent, "notes.md": starterNotesContent} {
		got, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(got) != want {
			t.Errorf("concurrent starter %s = %q, err = %v", name, got, err)
		}
	}
}

func TestContextStoreProvisionRejectsUnsafeStarterEntry(t *testing.T) {
	storageRoot := filepath.Join(t.TempDir(), "agent-projects")
	store := NewContextStore(storageRoot)
	projectID := uuid.NewString()
	root, err := store.ContextPath(projectID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(t.TempDir(), "external-index.md")
	if err := os.WriteFile(external, []byte("external"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(root, "index.md")); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Provision(context.Background(), projectID); !errors.Is(err, ErrInvalidContextPath) {
		t.Fatalf("Provision with linked index error = %v, want ErrInvalidContextPath", err)
	}
	if got, err := os.ReadFile(external); err != nil || string(got) != "external" {
		t.Fatalf("external index = %q, err = %v", got, err)
	}
}

func TestContextStoreSharesCanonicalFilesAcrossTaskLinks(t *testing.T) {
	base := t.TempDir()
	store := NewContextStore(filepath.Join(base, "agent-projects"))
	projectID := uuid.NewString()
	contextRoot, err := store.Provision(context.Background(), projectID)
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	for _, task := range []struct{ id, dir string }{{"coordinator", "coordinator_abc"}, {"worker", "worker_def"}} {
		taskRoot := filepath.Join(base, "tasks", task.dir)
		if err := os.MkdirAll(taskRoot, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := storageworkspaces.WriteOwnershipMarker(taskRoot, storageworkspaces.OwnershipMarker{
			TaskID: task.id, WorkspaceID: "workspace-1", TaskDirName: task.dir,
			LayoutVersion: storageworkspaces.LayoutVersionSemantic,
		}); err != nil {
			t.Fatal(err)
		}
		if err := store.EnsureTaskLink(taskRoot, projectID, task.id, task.dir); err != nil {
			t.Fatalf("EnsureTaskLink(%s): %v", task.id, err)
		}
	}

	workerNotes := filepath.Join(base, "tasks", "worker_def", "context", "notes.md")
	if err := os.WriteFile(workerNotes, []byte("worker update\n"), 0o600); err != nil {
		t.Fatalf("worker direct write: %v", err)
	}
	coordinatorNotes := filepath.Join(base, "tasks", "coordinator_abc", "context", "notes.md")
	got, err := os.ReadFile(coordinatorNotes)
	if err != nil || string(got) != "worker update\n" {
		t.Fatalf("coordinator sees shared context %q, err %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(contextRoot, "notes.md")); err != nil {
		t.Fatalf("canonical notes file is missing: %v", err)
	}
}
