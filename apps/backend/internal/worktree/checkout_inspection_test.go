package worktree

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
)

func TestMainCheckoutAdmissionAndReuse(t *testing.T) {
	for _, unborn := range []bool{false, true} {
		name := "initialized"
		if unborn {
			name = "unborn"
		}
		t.Run(name, func(t *testing.T) {
			mgr, store, wt, before := newMainCheckoutFixture(t, unborn)
			assertUnchanged := func(t *testing.T) {
				t.Helper()
				assertMainCheckoutState(t, wt.Path, before)
				if mgr.IsValid(wt.Path) {
					t.Fatal("Manager.IsValid accepted a main checkout; linked-worktree integrity must remain strict")
				}
				if _, err := os.Lstat(wt.Path + ".kandev-recovery.json"); !os.IsNotExist(err) {
					t.Fatalf("recovery record exists after read-only operation: %v", err)
				}
				if got := len(store.worktrees); got != 1 {
					t.Fatalf("worktree records = %d, want 1", got)
				}
			}

			t.Run("legacy admission", func(t *testing.T) {
				if err := mgr.AdmitTaskRecovery(context.Background(), wt.TaskID); err != nil {
					t.Fatalf("AdmitTaskRecovery: %v", err)
				}
				assertUnchanged(t)
			})

			t.Run("ambient Git overrides", func(t *testing.T) {
				t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "wrong.git"))
				t.Setenv("GIT_WORK_TREE", filepath.Join(t.TempDir(), "wrong-worktree"))
				t.Setenv("GIT_COMMON_DIR", filepath.Join(t.TempDir(), "wrong-common"))
				if err := mgr.AdmitTaskRecovery(context.Background(), wt.TaskID); err != nil {
					t.Fatalf("AdmitTaskRecovery with ambient Git overrides: %v", err)
				}
			})

			t.Run("selected admission", func(t *testing.T) {
				admission, err := mgr.AdmitRecovery(context.Background(), RecoveryAdmissionRequest{
					TaskID: wt.TaskID, SessionID: wt.SessionID, TaskEnvironmentID: wt.TaskEnvironmentID,
					OwnerTaskID: wt.TaskID, OwnershipGeneration: 1, ExecutorType: "worktree",
					Slots: []RecoverySlot{{
						WorktreeID: wt.ID, RepositoryID: wt.RepositoryID, BranchSlug: wt.BranchSlug,
						RepositoryPath: wt.RepositoryPath,
					}},
				})
				if err != nil {
					t.Fatalf("AdmitRecovery: %v", err)
				}
				if admission != nil {
					_ = admission.Release(context.Background())
					t.Fatal("healthy main checkout acquired a recovery admission")
				}
				assertUnchanged(t)
			})

			t.Run("ordinary reuse by session", func(t *testing.T) {
				got, handled, err := mgr.tryReuseExisting(context.Background(), CreateRequest{
					TaskID: "task-attached", SessionID: wt.SessionID, TaskEnvironmentID: wt.TaskEnvironmentID,
					RepositoryID: wt.RepositoryID, RepositoryPath: wt.RepositoryPath, BranchSlug: wt.BranchSlug,
				})
				if err != nil || !handled || got == nil || got.ID != wt.ID || got.Path != wt.Path {
					t.Fatalf("tryReuseExisting by session = (%+v, %t, %v), want existing main checkout", got, handled, err)
				}
				assertUnchanged(t)
			})

			t.Run("ordinary reuse by ID", func(t *testing.T) {
				got, handled, err := mgr.tryReuseExisting(context.Background(), CreateRequest{
					TaskID: "task-resumed", TaskEnvironmentID: wt.TaskEnvironmentID,
					RepositoryID: wt.RepositoryID, RepositoryPath: wt.RepositoryPath, WorktreeID: wt.ID,
				})
				if err != nil || !handled || got == nil || got.ID != wt.ID || got.Path != wt.Path {
					t.Fatalf("tryReuseExisting by ID = (%+v, %t, %v), want existing main checkout", got, handled, err)
				}
				assertUnchanged(t)
			})

			t.Run("additional-session Create", func(t *testing.T) {
				got, err := mgr.Create(context.Background(), CreateRequest{
					TaskID: "task-additional", SessionID: "session-additional", TaskEnvironmentID: wt.TaskEnvironmentID,
					RepositoryID: wt.RepositoryID, RepositoryPath: wt.RepositoryPath, WorktreeID: wt.ID,
					BaseBranch: wt.Branch, BranchSlug: wt.BranchSlug, ReuseRequired: true,
				})
				if err != nil || got == nil || got.ID != wt.ID || got.Path != wt.Path {
					t.Fatalf("Create(ReuseRequired) = (%+v, %v), want existing main checkout", got, err)
				}
				assertUnchanged(t)
			})
		})
	}
}

func TestMainCheckoutInspectionPreservesOperationalErrors(t *testing.T) {
	operations := []struct {
		name string
		run  func(context.Context, *Manager, *Worktree) error
	}{
		{
			name: "legacy admission",
			run: func(ctx context.Context, mgr *Manager, wt *Worktree) error {
				return mgr.AdmitTaskRecovery(ctx, wt.TaskID)
			},
		},
		{
			name: "selected admission",
			run: func(ctx context.Context, mgr *Manager, wt *Worktree) error {
				_, err := mgr.AdmitRecovery(ctx, RecoveryAdmissionRequest{
					TaskID: wt.TaskID, SessionID: wt.SessionID, TaskEnvironmentID: wt.TaskEnvironmentID,
					OwnerTaskID: wt.TaskID, OwnershipGeneration: 1, ExecutorType: "worktree",
					Slots: []RecoverySlot{{
						WorktreeID: wt.ID, RepositoryID: wt.RepositoryID, BranchSlug: wt.BranchSlug,
						RepositoryPath: wt.RepositoryPath,
					}},
				})
				return err
			},
		},
		{
			name: "reuse",
			run: func(ctx context.Context, mgr *Manager, wt *Worktree) error {
				_, _, err := mgr.tryReuseExisting(ctx, CreateRequest{
					TaskID: "task-resumed", TaskEnvironmentID: wt.TaskEnvironmentID,
					RepositoryID: wt.RepositoryID, BranchSlug: wt.BranchSlug, WorktreeID: wt.ID,
				})
				return err
			},
		},
	}

	for _, operation := range operations {
		for _, operationalError := range []struct {
			name string
			want error
		}{
			{name: "canceled request", want: context.Canceled},
			{name: "inspection timeout", want: context.DeadlineExceeded},
		} {
			t.Run(operation.name+"/"+operationalError.name, func(t *testing.T) {
				mgr, store, wt, before := newMainCheckoutFixture(t, false)
				originalPath := os.Getenv("PATH")
				markerPath := filepath.Join(t.TempDir(), "git-started")
				t.Setenv("KD_GIT_INSPECTION_MARKER", markerPath)
				scriptBody := `: > "${KD_GIT_INSPECTION_MARKER:?}"; exec sleep 30`
				scriptDir := writeFakeGitScript(t, scriptBody)
				t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+originalPath)

				var err error
				switch operationalError.want {
				case context.Canceled:
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					result := make(chan error, 1)
					go func() { result <- operation.run(ctx, mgr, wt) }()
					waitForFile(t, markerPath)
					cancel()
					err = <-result
				case context.DeadlineExceeded:
					mgr.inspectTimeout = 100 * time.Millisecond
					ctx := context.Background()
					err = operation.run(ctx, mgr, wt)
					if ctx.Err() != nil {
						t.Fatalf("parent context error = %v, want live parent context", ctx.Err())
					}
				default:
					t.Fatalf("unhandled operational error %v", operationalError.want)
				}
				if err == nil || !errors.Is(err, operationalError.want) {
					t.Fatalf("operation error = %v, want errors.Is(_, %v)", err, operationalError.want)
				}
				var recoveryErr *WorktreeRecoveryError
				if errors.As(err, &recoveryErr) || errors.Is(err, ErrWorktreeCorrupted) {
					t.Fatalf("operational error was classified as checkout corruption: %v", err)
				}

				if setErr := os.Setenv("PATH", originalPath); setErr != nil {
					t.Fatalf("restore PATH for checkout assertions: %v", setErr)
				}
				assertMainCheckoutState(t, wt.Path, before)
				if _, statErr := os.Lstat(wt.Path + ".kandev-recovery.json"); !os.IsNotExist(statErr) {
					t.Fatalf("recovery record exists after operational error: %v", statErr)
				}
				if len(store.worktrees) != 1 || store.worktrees[wt.ID] == nil || store.worktrees[wt.ID].Path != wt.Path {
					t.Fatalf("worktree records changed after operational error: %#v", store.worktrees)
				}
			})
		}
	}
}

func TestMainCheckoutRejectsInvalidMetadata(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *Manager, *Worktree)
	}{
		{name: "missing metadata", mutate: func(t *testing.T, _ *Manager, wt *Worktree) {
			if err := os.RemoveAll(filepath.Join(wt.Path, ".git")); err != nil {
				t.Fatalf("remove Git metadata: %v", err)
			}
		}},
		{name: "empty metadata directory", mutate: func(t *testing.T, _ *Manager, wt *Worktree) {
			gitDir := filepath.Join(wt.Path, ".git")
			if err := os.RemoveAll(gitDir); err != nil {
				t.Fatalf("remove Git metadata: %v", err)
			}
			if err := os.Mkdir(gitDir, 0o700); err != nil {
				t.Fatalf("create empty Git metadata: %v", err)
			}
		}},
		{name: "malformed pointer", mutate: func(t *testing.T, _ *Manager, wt *Worktree) {
			gitDir := filepath.Join(wt.Path, ".git")
			if err := os.RemoveAll(gitDir); err != nil {
				t.Fatalf("remove Git metadata: %v", err)
			}
			if err := os.WriteFile(gitDir, []byte("not a Git pointer\n"), 0o600); err != nil {
				t.Fatalf("write malformed Git pointer: %v", err)
			}
		}},
		{name: "symlinked metadata", mutate: func(t *testing.T, _ *Manager, wt *Worktree) {
			gitDir := filepath.Join(wt.Path, ".git")
			external := filepath.Join(t.TempDir(), "external.git")
			if err := os.Rename(gitDir, external); err != nil {
				t.Fatalf("move Git metadata: %v", err)
			}
			if err := os.Symlink(external, gitDir); err != nil {
				t.Skipf("cannot create metadata symlink on this platform: %v", err)
			}
		}},
		{name: "bare metadata", mutate: func(t *testing.T, _ *Manager, wt *Worktree) {
			gitDir := filepath.Join(wt.Path, ".git")
			if err := os.RemoveAll(gitDir); err != nil {
				t.Fatalf("remove Git metadata: %v", err)
			}
			if err := os.Mkdir(gitDir, 0o700); err != nil {
				t.Fatalf("create bare metadata directory: %v", err)
			}
			runGit(t, gitDir, "init", "--bare")
		}},
		{name: "common directory redirection", mutate: func(t *testing.T, _ *Manager, wt *Worktree) {
			external := initGitRepoForWorktreeTest(t)
			commonDir := filepath.Join(external, ".git")
			if err := os.WriteFile(filepath.Join(wt.Path, ".git", "commondir"), []byte(commonDir+"\n"), 0o600); err != nil {
				t.Fatalf("redirect Git common directory: %v", err)
			}
		}},
		{name: "invalid HEAD", mutate: func(t *testing.T, _ *Manager, wt *Worktree) {
			if err := os.WriteFile(filepath.Join(wt.Path, ".git", "HEAD"), []byte("not a ref\n"), 0o600); err != nil {
				t.Fatalf("corrupt HEAD: %v", err)
			}
		}},
		{name: "tag HEAD", mutate: func(t *testing.T, _ *Manager, wt *Worktree) {
			runGit(t, wt.Path, "update-ref", "refs/tags/main-as-head", "HEAD")
			if err := os.WriteFile(filepath.Join(wt.Path, ".git", "HEAD"), []byte("ref: refs/tags/main-as-head\n"), 0o600); err != nil {
				t.Fatalf("write tag HEAD: %v", err)
			}
		}},
		{name: "remote-tracking HEAD", mutate: func(t *testing.T, _ *Manager, wt *Worktree) {
			runGit(t, wt.Path, "update-ref", "refs/remotes/origin/main", "HEAD")
			if err := os.WriteFile(filepath.Join(wt.Path, ".git", "HEAD"), []byte("ref: refs/remotes/origin/main\n"), 0o600); err != nil {
				t.Fatalf("write remote-tracking HEAD: %v", err)
			}
		}},
		{name: "Git inspection denied", mutate: func(t *testing.T, _ *Manager, _ *Worktree) {
			scriptDir := writeFakeGitScript(t, `echo "fatal: permission denied reading Git metadata" >&2; exit 128`)
			t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr, _, wt, _ := newMainCheckoutFixture(t, false)
			before := mustReadPath(t, filepath.Join(wt.Path, "tracked.txt"))
			tt.mutate(t, mgr, wt)
			err := mgr.AdmitTaskRecovery(context.Background(), wt.TaskID)
			if err == nil {
				t.Fatal("AdmitTaskRecovery accepted invalid Git metadata")
			}
			var recoveryErr *WorktreeRecoveryError
			if !errors.As(err, &recoveryErr) {
				t.Fatalf("AdmitTaskRecovery error = %T %v, want WorktreeRecoveryError", err, err)
			}
			if _, err := os.Stat(wt.Path); err != nil {
				t.Fatalf("checkout was removed after refusal: %v", err)
			}
			if got := mustReadPath(t, filepath.Join(wt.Path, "tracked.txt")); !bytes.Equal(got, before) {
				t.Fatalf("tracked content changed after refusal: got %q, want %q", got, before)
			}
			if _, err := os.Lstat(wt.Path + ".kandev-recovery.json"); !os.IsNotExist(err) {
				t.Fatalf("recovery record exists after refusal: %v", err)
			}
		})
	}
}

func TestMainCheckoutManagedCloneIdentity(t *testing.T) {
	for _, testCase := range []struct {
		name              string
		mainAtDestination bool
		origin            string
		wantErr           bool
	}{
		{name: "matching managed clone", mainAtDestination: true, origin: "https://github.com/acme/widget.git"},
		{name: "main checkout from another clone", origin: "https://github.com/acme/widget.git", wantErr: true},
		{name: "provider origin mismatch", mainAtDestination: true, origin: "https://github.com/untrusted/widget.git", wantErr: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			mgr, store, wt, before := newMainCheckoutFixture(t, false)
			managedRoot := filepath.Dir(wt.Path)
			destination := wt.Path
			if !testCase.mainAtDestination {
				destination = filepath.Join(managedRoot, "managed", "widget")
				if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
					t.Fatalf("create managed clone root: %v", err)
				}
				if err := os.Mkdir(destination, 0o755); err != nil {
					t.Fatalf("create managed clone destination: %v", err)
				}
				runGit(t, destination, "init", "-b", "main")
			}
			if _, err := execGit(t, wt.Path, "remote", "add", "origin", testCase.origin); err != nil {
				t.Fatalf("set main checkout origin: %v", err)
			}
			if !testCase.mainAtDestination {
				if _, err := execGit(t, destination, "remote", "add", "origin", "https://github.com/acme/widget.git"); err != nil {
					t.Fatalf("set managed destination origin: %v", err)
				}
				wt.RepositoryPath = destination
			}
			configBefore := mustReadPath(t, filepath.Join(wt.Path, ".git", "config"))
			proof := &ManagedCloneRelocationProof{
				ManagedRoot: managedRoot, ExpectedDestinationPath: destination,
				Identity: ManagedRepositoryIdentity{Provider: "github", Host: "github.com", Owner: "acme", Name: "widget"},
			}
			admission, err := mgr.AdmitRecovery(context.Background(), RecoveryAdmissionRequest{
				TaskID: wt.TaskID, SessionID: wt.SessionID, TaskEnvironmentID: wt.TaskEnvironmentID,
				OwnerTaskID: wt.TaskID, OwnershipGeneration: 1, ExecutorType: "worktree",
				Slots: []RecoverySlot{{
					WorktreeID: wt.ID, RepositoryID: wt.RepositoryID, BranchSlug: wt.BranchSlug,
					RepositoryPath: destination, CloneRelocation: proof,
				}},
			})
			if testCase.wantErr {
				var recoveryErr *WorktreeRecoveryError
				if !errors.As(err, &recoveryErr) {
					t.Fatalf("AdmitRecovery() error = %v, want WorktreeRecoveryError", err)
				}
				if admission != nil {
					_ = admission.Release(context.Background())
					t.Fatal("invalid main checkout acquired a recovery admission")
				}
			} else if err != nil || admission != nil {
				if admission != nil {
					_ = admission.Release(context.Background())
				}
				t.Fatalf("AdmitRecovery() = (%v, %v), want read-only acceptance", admission, err)
			}
			assertMainCheckoutState(t, wt.Path, before)
			if got := mustReadPath(t, filepath.Join(wt.Path, ".git", "config")); !bytes.Equal(got, configBefore) {
				t.Fatal("main checkout Git config changed during managed identity validation")
			}
			if _, err := os.Lstat(wt.Path + ".kandev-recovery.json"); !os.IsNotExist(err) {
				t.Fatalf("recovery record exists after managed identity validation: %v", err)
			}
			if len(store.worktrees) != 1 || store.worktrees[wt.ID] == nil || store.worktrees[wt.ID].Path != wt.Path {
				t.Fatalf("worktree inventory changed during managed identity validation: %#v", store.worktrees)
			}
		})
	}
}

func TestMainCheckoutAcceptsDetachedCommit(t *testing.T) {
	mgr, _, wt, _ := newMainCheckoutFixture(t, false)
	runGit(t, wt.Path, "checkout", "--detach", "HEAD")
	headBefore := mustReadPath(t, filepath.Join(wt.Path, ".git", "HEAD"))
	indexBefore := mustReadPath(t, filepath.Join(wt.Path, ".git", "index"))

	if err := mgr.AdmitTaskRecovery(context.Background(), wt.TaskID); err != nil {
		t.Fatalf("AdmitTaskRecovery for detached commit: %v", err)
	}
	if got := mustReadPath(t, filepath.Join(wt.Path, ".git", "HEAD")); !bytes.Equal(got, headBefore) {
		t.Fatalf("detached HEAD changed during inspection: got %q, want %q", got, headBefore)
	}
	if got := mustReadPath(t, filepath.Join(wt.Path, ".git", "index")); !bytes.Equal(got, indexBefore) {
		t.Fatal("detached checkout index changed during inspection")
	}
}

func TestMainCheckoutTestGitHelpersIgnoreAmbientGitOverrides(t *testing.T) {
	_, _, wt, _ := newMainCheckoutFixture(t, false)
	otherRepo := initGitRepoForWorktreeTest(t)
	t.Setenv("GIT_DIR", filepath.Join(otherRepo, ".git"))
	t.Setenv("GIT_WORK_TREE", otherRepo)
	t.Setenv("GIT_COMMON_DIR", filepath.Join(otherRepo, ".git"))

	output, err := execGit(t, wt.Path, "rev-parse", "--show-toplevel")
	if err != nil {
		t.Fatalf("execGit with ambient Git overrides: %v", err)
	}
	if got := filepath.Clean(strings.TrimSpace(output)); got != filepath.Clean(wt.Path) {
		t.Fatalf("execGit repository root = %q, want %q", got, wt.Path)
	}
	if got := filepath.Clean(strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "--show-toplevel"))); got != filepath.Clean(wt.Path) {
		t.Fatalf("runGit repository root = %q, want %q", got, wt.Path)
	}
}

func TestMainCheckoutMixedInventoryRefusesBeforeRecovery(t *testing.T) {
	mgr, store, mainWorktree, before := newMainCheckoutFixture(t, false)
	taskDir := filepath.Dir(mainWorktree.Path)
	linkedPath := filepath.Join(taskDir, "linked")
	runGit(t, mainWorktree.Path, "worktree", "add", "-b", "feature/linked", linkedPath, "main")
	pointer := mustReadPath(t, filepath.Join(linkedPath, ".git"))
	adminPath := strings.TrimSpace(strings.TrimPrefix(string(pointer), "gitdir:"))
	if !filepath.IsAbs(adminPath) {
		adminPath = filepath.Join(linkedPath, adminPath)
	}
	if err := os.RemoveAll(adminPath); err != nil {
		t.Fatalf("remove linked-worktree admin metadata: %v", err)
	}
	invalidPath := filepath.Join(taskDir, "invalid")
	if err := os.MkdirAll(filepath.Join(invalidPath, ".git"), 0o700); err != nil {
		t.Fatalf("create ambiguous sibling checkout: %v", err)
	}
	linked := &Worktree{
		ID: "linked-worktree", TaskID: mainWorktree.TaskID, TaskDirName: mainWorktree.TaskDirName,
		TaskEnvironmentID: mainWorktree.TaskEnvironmentID, RepositoryID: mainWorktree.RepositoryID,
		RepositoryPath: mainWorktree.RepositoryPath, Path: linkedPath, Branch: "feature/linked",
		BranchSlug: "linked", Status: StatusActive,
	}
	invalid := &Worktree{
		ID: "invalid-worktree", TaskID: mainWorktree.TaskID, TaskDirName: mainWorktree.TaskDirName,
		TaskEnvironmentID: mainWorktree.TaskEnvironmentID, RepositoryID: "repository-invalid",
		RepositoryPath: mainWorktree.RepositoryPath, Path: invalidPath, Branch: "feature/invalid",
		BranchSlug: "invalid", Status: StatusActive,
	}
	store.worktrees[linked.ID] = linked
	store.worktrees[invalid.ID] = invalid

	_, err := mgr.AdmitRecovery(context.Background(), RecoveryAdmissionRequest{
		TaskID: mainWorktree.TaskID, SessionID: mainWorktree.SessionID,
		TaskEnvironmentID: mainWorktree.TaskEnvironmentID, OwnerTaskID: mainWorktree.TaskID,
		OwnershipGeneration: 1, ExecutorType: "worktree",
		Slots: []RecoverySlot{
			{WorktreeID: mainWorktree.ID, RepositoryID: mainWorktree.RepositoryID, BranchSlug: mainWorktree.BranchSlug, RepositoryPath: mainWorktree.RepositoryPath},
			{WorktreeID: linked.ID, RepositoryID: linked.RepositoryID, BranchSlug: linked.BranchSlug, RepositoryPath: linked.RepositoryPath},
			{WorktreeID: invalid.ID, RepositoryID: invalid.RepositoryID, BranchSlug: invalid.BranchSlug, RepositoryPath: invalid.RepositoryPath},
		},
	})
	var recoveryErr *WorktreeRecoveryError
	if !errors.As(err, &recoveryErr) {
		t.Fatalf("AdmitRecovery() error = %v, want WorktreeRecoveryError for ambiguous sibling", err)
	}
	if recoveryErr.Checkout != invalidPath {
		t.Fatalf("refusal checkout = %q, want ambiguous sibling %q", recoveryErr.Checkout, invalidPath)
	}
	assertMainCheckoutState(t, mainWorktree.Path, before)
	if got := string(mustReadPath(t, filepath.Join(linkedPath, ".git"))); got != string(pointer) {
		t.Fatalf("linked-worktree pointer changed after refusal: got %q, want %q", got, pointer)
	}
	if _, err := os.Stat(adminPath); !os.IsNotExist(err) {
		t.Fatalf("linked-worktree admin metadata was recreated: %v", err)
	}
	for _, path := range []string{mainWorktree.Path, linkedPath, invalidPath} {
		if _, err := os.Lstat(path + ".kandev-recovery.json"); !os.IsNotExist(err) {
			t.Fatalf("recovery record created for %q: %v", path, err)
		}
	}
}

func TestMainCheckoutWithRecoverableLinkedSibling(t *testing.T) {
	mgr, store, mainWorktree, before := newMainCheckoutFixture(t, false)
	taskDir := filepath.Dir(mainWorktree.Path)
	linkedPath := filepath.Join(taskDir, "linked")
	runGit(t, mainWorktree.Path, "worktree", "add", "-b", "feature/linked-recovery", linkedPath, "main")
	writeMainCheckoutFile(t, linkedPath, "kept-after-recovery.txt", "linked checkout data\n")
	pointer := mustReadPath(t, filepath.Join(linkedPath, ".git"))
	adminPath := strings.TrimSpace(strings.TrimPrefix(string(pointer), "gitdir:"))
	if !filepath.IsAbs(adminPath) {
		adminPath = filepath.Join(linkedPath, adminPath)
	}
	if err := os.RemoveAll(adminPath); err != nil {
		t.Fatalf("remove linked-worktree admin metadata: %v", err)
	}
	linked := &Worktree{
		ID: "linked-worktree", TaskID: mainWorktree.TaskID, TaskDirName: mainWorktree.TaskDirName,
		TaskEnvironmentID: mainWorktree.TaskEnvironmentID, RepositoryID: "repository-linked",
		RepositoryPath: mainWorktree.Path, Path: linkedPath, Branch: "feature/linked-recovery",
		BranchSlug: "linked-recovery", BaseBranch: "main", Status: StatusActive,
	}
	store.worktrees[linked.ID] = linked
	mgr.store = &managedCloneRelocationStore{recoveryCASStore: &recoveryCASStore{mockStore: store}}

	admission, err := mgr.AdmitRecovery(context.Background(), RecoveryAdmissionRequest{
		TaskID: mainWorktree.TaskID, SessionID: mainWorktree.SessionID,
		TaskEnvironmentID: mainWorktree.TaskEnvironmentID, OwnerTaskID: mainWorktree.TaskID,
		OwnershipGeneration: 1, ExecutorType: "worktree",
		Slots: []RecoverySlot{
			{WorktreeID: mainWorktree.ID, RepositoryID: mainWorktree.RepositoryID, BranchSlug: mainWorktree.BranchSlug, RepositoryPath: mainWorktree.RepositoryPath},
			{WorktreeID: linked.ID, RepositoryID: linked.RepositoryID, BranchSlug: linked.BranchSlug, RepositoryPath: linked.RepositoryPath},
		},
	})
	if err != nil {
		t.Fatalf("AdmitRecovery with healthy main and recoverable linked slot: %v", err)
	}
	if admission == nil {
		t.Fatal("AdmitRecovery returned no held admission for the linked-worktree recovery")
	}
	if err := admission.Release(context.Background()); err != nil {
		t.Fatalf("release recovery admission: %v", err)
	}
	assertMainCheckoutState(t, mainWorktree.Path, before)
	if _, err := os.Stat(filepath.Join(linkedPath, "kept-after-recovery.txt")); err != nil {
		t.Fatalf("original linked checkout was not retained: %v", err)
	}
	if got := len(store.worktrees); got != 2 {
		t.Fatalf("durable worktree records = %d, want main plus recovered linked checkout", got)
	}
	if current := store.worktrees[mainWorktree.ID]; current == nil || current.Path != mainWorktree.Path {
		t.Fatal("healthy main checkout record was replaced during linked recovery")
	}
	if claimStore := mgr.store.(*managedCloneRelocationStore); claimStore.claim != nil {
		t.Fatal("durable recovery claim remains after admission release")
	}
}

func mustReadPath(t *testing.T, path string) []byte {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return contents
}

type mainCheckoutState struct {
	branch      string
	head        string
	index       []byte
	indexExists bool
	tracked     []byte
	staged      []byte
	untracked   []byte
	ignored     []byte
}

func newMainCheckoutFixture(t *testing.T, unborn bool) (*Manager, *mockStore, *Worktree, mainCheckoutState) {
	t.Helper()
	cfg := newTestConfig(t)
	taskDirName := "task-main-checkout"
	taskDir := filepath.Join(cfg.TasksBasePath, taskDirName)
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatalf("create task root: %v", err)
	}
	if err := storageworkspaces.WriteOwnershipMarker(taskDir, storageworkspaces.OwnershipMarker{
		TaskID: "task-main-checkout", TaskDirName: taskDirName, LayoutVersion: storageworkspaces.LayoutVersionSemantic,
	}); err != nil {
		t.Fatalf("write task ownership marker: %v", err)
	}
	repoPath := filepath.Join(taskDir, "repo")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("create main checkout: %v", err)
	}
	runGit(t, repoPath, "init", "-b", "main")
	runGit(t, repoPath, "config", "user.email", "test@example.com")
	runGit(t, repoPath, "config", "user.name", "Test User")
	runGit(t, repoPath, "config", "commit.gpgsign", "false")
	if !unborn {
		writeMainCheckoutFile(t, repoPath, ".gitignore", "ignored.txt\n")
		writeMainCheckoutFile(t, repoPath, "tracked.txt", "committed\n")
		runGit(t, repoPath, "add", ".gitignore", "tracked.txt")
		runGit(t, repoPath, "commit", "-m", "initial commit")
		runGit(t, repoPath, "checkout", "-b", "feature/main-checkout")
		writeMainCheckoutFile(t, repoPath, "tracked.txt", "unstaged edit\n")
		writeMainCheckoutFile(t, repoPath, "staged.txt", "staged edit\n")
		runGit(t, repoPath, "add", "staged.txt")
		writeMainCheckoutFile(t, repoPath, "untracked.txt", "untracked data\n")
		writeMainCheckoutFile(t, repoPath, "ignored.txt", "ignored data\n")
	}
	branch := "main"
	if !unborn {
		branch = "feature/main-checkout"
	}
	wt := &Worktree{
		ID: "main-worktree", TaskID: "task-main-checkout", TaskDirName: taskDirName,
		TaskEnvironmentID: "environment-main-checkout", RepositoryID: "repository-main-checkout",
		SessionID: "session-main-checkout", BranchSlug: "main", RepositoryPath: repoPath,
		Path: repoPath, Branch: branch, Status: StatusActive,
	}
	store := newMockStore()
	store.worktrees[wt.ID] = wt
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return mgr, store, wt, captureMainCheckoutState(t, repoPath)
}

func writeMainCheckoutFile(t *testing.T, repoPath, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repoPath, name), []byte(contents), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func captureMainCheckoutState(t *testing.T, repoPath string) mainCheckoutState {
	t.Helper()
	state := mainCheckoutState{
		branch: strings.TrimSpace(runGit(t, repoPath, "symbolic-ref", "--short", "HEAD")),
	}
	index, err := os.ReadFile(filepath.Join(repoPath, ".git/index"))
	state.index = index
	state.indexExists = err == nil
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read Git index: %v", err)
	}
	if head, err := execGit(t, repoPath, "rev-parse", "--verify", "HEAD"); err == nil {
		state.head = strings.TrimSpace(head)
	}
	for name, target := range map[string]*[]byte{
		"tracked.txt": &state.tracked, "staged.txt": &state.staged,
		"untracked.txt": &state.untracked, "ignored.txt": &state.ignored,
	} {
		if contents, err := os.ReadFile(filepath.Join(repoPath, name)); err == nil {
			*target = contents
		}
	}
	return state
}

func assertMainCheckoutState(t *testing.T, repoPath string, want mainCheckoutState) {
	t.Helper()
	got := captureMainCheckoutState(t, repoPath)
	if got.branch != want.branch || got.head != want.head || got.indexExists != want.indexExists || !bytes.Equal(got.index, want.index) ||
		!bytes.Equal(got.tracked, want.tracked) || !bytes.Equal(got.staged, want.staged) ||
		!bytes.Equal(got.untracked, want.untracked) || !bytes.Equal(got.ignored, want.ignored) {
		t.Fatalf("main checkout changed: got %#v, want %#v", got, want)
	}
}

func execGit(t *testing.T, repoPath string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	cmd.Env = mainCheckoutInspectionEnvironment(os.Environ())
	output, err := cmd.CombinedOutput()
	return string(output), err
}
