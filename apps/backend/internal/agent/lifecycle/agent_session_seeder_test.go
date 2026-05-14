package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
)

func newSeederTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return log
}

// seedTestHostHome rebinds os.UserHomeDir() to a temp dir so the seeder reads
// from a clean fixture instead of the developer's actual ~/. Returns the temp
// dir path.
func seedTestHostHome(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", tmp)
	}
	return tmp
}

func writeFile(t *testing.T, root, rel string, contents []byte) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, contents, 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// TestSeedAgentSessionDir_OnlyCopiesAuthFiles is the load-bearing check for
// the codex bug: the seeder must copy auth.json/config.toml from the host
// home into the per-container session dir but must NOT bring along state.db,
// sessions/, or other host-state caches that contain absolute host paths.
func TestSeedAgentSessionDir_OnlyCopiesAuthFiles(t *testing.T) {
	hostHome := seedTestHostHome(t)
	writeFile(t, hostHome, ".codex/auth.json", []byte(`{"token":"abc"}`))
	writeFile(t, hostHome, ".codex/config.toml", []byte(`model = "gpt"`))
	writeFile(t, hostHome, ".codex/state.db", []byte("STATE_DB_BLOB"))
	writeFile(t, hostHome, ".codex/state.db-wal", []byte("WAL"))
	writeFile(t, hostHome, ".codex/sessions/2026-05/rollout-x.jsonl", []byte("ROLLOUT"))
	writeFile(t, hostHome, ".codex/junk.txt", []byte("junk"))

	kandevHome := t.TempDir()
	const instanceID = "abcdef0123456789"
	instanceRoot := InstanceSessionRoot(kandevHome, instanceID)

	if err := SeedAgentSessionDir(context.Background(), agents.NewCodexACP(), instanceRoot, newSeederTestLogger(t)); err != nil {
		t.Fatalf("SeedAgentSessionDir: %v", err)
	}

	dotdir := filepath.Join(instanceRoot, ".codex")

	mustExist := []string{"auth.json", "config.toml"}
	for _, name := range mustExist {
		if _, err := os.Stat(filepath.Join(dotdir, name)); err != nil {
			t.Fatalf("expected %s in session dir: %v", name, err)
		}
	}

	mustNotExist := []string{"state.db", "state.db-wal", "junk.txt", "sessions"}
	for _, name := range mustNotExist {
		if _, err := os.Stat(filepath.Join(dotdir, name)); !os.IsNotExist(err) {
			t.Fatalf("unexpected %s leaked into session dir (err=%v)", name, err)
		}
	}
}

// TestSeedAgentSessionDir_PerInstanceUnique ensures two containers running
// the same agent get independent session dirs. Without this the codex state
// DB from one task would clobber another's, defeating the whole isolation
// guarantee.
func TestSeedAgentSessionDir_PerInstanceUnique(t *testing.T) {
	hostHome := seedTestHostHome(t)
	writeFile(t, hostHome, ".codex/auth.json", []byte(`{"token":"a"}`))

	kandevHome := t.TempDir()
	rootA := InstanceSessionRoot(kandevHome, "instance-a")
	rootB := InstanceSessionRoot(kandevHome, "instance-b")

	if rootA == rootB || rootA == "" {
		t.Fatalf("unexpected dir collision: a=%q b=%q", rootA, rootB)
	}

	log := newSeederTestLogger(t)
	if err := SeedAgentSessionDir(context.Background(), agents.NewCodexACP(), rootA, log); err != nil {
		t.Fatalf("seed A: %v", err)
	}
	if err := SeedAgentSessionDir(context.Background(), agents.NewCodexACP(), rootB, log); err != nil {
		t.Fatalf("seed B: %v", err)
	}

	for _, root := range []string{rootA, rootB} {
		if _, err := os.Stat(filepath.Join(root, ".codex/auth.json")); err != nil {
			t.Fatalf("missing auth.json in %s: %v", root, err)
		}
	}
}

// TestSeedAgentSessionDir_MultipleAgents proves the agent-agnostic guarantee:
// the same logic seeds claude / opencode just as it seeds codex, with no
// per-agent special cases.
func TestSeedAgentSessionDir_MultipleAgents(t *testing.T) {
	hostHome := seedTestHostHome(t)
	// Touch every auth file each tested agent might want; the seeder skips
	// missing ones silently so unrelated agents staying empty is fine.
	writeFile(t, hostHome, ".codex/auth.json", []byte("codex"))
	writeFile(t, hostHome, ".codex/config.toml", []byte("codex"))
	writeFile(t, hostHome, ".claude/.credentials.json", []byte("claude"))
	writeFile(t, hostHome, ".opencode/auth.json", []byte("opencode"))

	cases := []struct {
		name string
		ag   agents.Agent
	}{
		{"codex", agents.NewCodexACP()},
		{"claude", agents.NewClaudeACP()},
		{"opencode", agents.NewOpenCodeACP()},
	}

	kandevHome := t.TempDir()
	log := newSeederTestLogger(t)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			instanceID := tc.name + "-instance"
			root := InstanceSessionRoot(kandevHome, instanceID)
			if err := SeedAgentSessionDir(context.Background(), tc.ag, root, log); err != nil {
				t.Fatalf("seed: %v", err)
			}
			// At least the per-instance root must exist and be a subdir of
			// the kandev-managed agent-sessions root.
			if _, err := os.Stat(root); err != nil {
				t.Fatalf("instance root missing: %v", err)
			}
			if !strings.HasPrefix(root, AgentSessionsRoot(kandevHome)) {
				t.Fatalf("root %q outside kandev agent-sessions %q", root, AgentSessionsRoot(kandevHome))
			}
		})
	}
}

func TestSessionDirHostPath_TrimsHomeTemplate(t *testing.T) {
	got := SessionDirHostPath("/tmp/kandev", "abc123", "{home}/.codex")
	want := filepath.Join("/tmp/kandev", "agent-sessions", "abc123", ".codex")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestLocalFileUploader_RejectsTraversal locks in the path-injection
// sanitiser: even when an internal caller hands the uploader a malformed
// path that escapes the kandev session root, the write is refused before it
// touches the host filesystem.
func TestLocalFileUploader_RejectsTraversal(t *testing.T) {
	root := t.TempDir()
	uploader := localFileUploader{root: root}

	cases := []struct {
		name string
		path string
	}{
		{name: "parent traversal", path: filepath.Join(root, "..", "outside.txt")},
		{name: "absolute outside", path: "/etc/passwd-kandev-test"},
		{name: "deep traversal", path: filepath.Join(root, "a", "..", "..", "outside.txt")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := uploader.WriteFile(context.Background(), tc.path, []byte("nope"), 0o600)
			if err == nil {
				t.Fatalf("expected traversal %q to be refused", tc.path)
			}
		})
	}
}

func TestLocalFileUploader_AllowsContainedPath(t *testing.T) {
	root := t.TempDir()
	uploader := localFileUploader{root: root}
	target := filepath.Join(root, "sub", "auth.json")

	if err := uploader.WriteFile(context.Background(), target, []byte("ok"), 0o600); err != nil {
		t.Fatalf("contained write should succeed: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("expected file at %s: %v", target, err)
	}
}

func TestCleanupAgentSessionDir_RemovesTree(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "instance-x")
	writeFile(t, tmp, "instance-x/.codex/auth.json", []byte("a"))
	writeFile(t, tmp, "instance-x/.codex/state.db", []byte("b"))

	CleanupAgentSessionDir(root, newSeederTestLogger(t))

	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("expected dir removed, got err=%v", err)
	}
}

func TestCleanupAgentSessionDir_NoopOnEmptyPath(t *testing.T) {
	// Should not panic; just exercise the guard.
	CleanupAgentSessionDir("", newSeederTestLogger(t))
}

// TestDockerStopInstance_PreservesSessionDirOnPlainStop drives the real
// DockerExecutor.StopInstance for both preserve and destructive stop
// reasons and asserts on the on-disk session dir. We pass instances with
// ContainerID == "" so StopInstance returns before talking to the docker
// daemon — the cleanup branch under test is the kandev-side dir teardown.
func TestDockerStopInstance_PreservesSessionDirOnPlainStop(t *testing.T) {
	hostHome := seedTestHostHome(t)
	writeFile(t, hostHome, ".codex/auth.json", []byte(`{"token":"a"}`))

	cases := []struct {
		reason       string
		expectExists bool
	}{
		{reason: "", expectExists: true},
		{reason: "stopped via API", expectExists: true},
		{reason: "agent crashed", expectExists: true},
		{reason: "user requested", expectExists: true},
		{reason: "task archived", expectExists: false},
		{reason: "task deleted", expectExists: false},
		{reason: "session archived", expectExists: false},
		{reason: "session deleted", expectExists: false},
	}

	log := newSeederTestLogger(t)

	for _, tc := range cases {
		t.Run(tc.reason, func(t *testing.T) {
			kandevHome := t.TempDir()
			instanceID := "stop-test-instance"
			root := InstanceSessionRoot(kandevHome, instanceID)
			if err := SeedAgentSessionDir(context.Background(), agents.NewCodexACP(), root, log); err != nil {
				t.Fatalf("seed: %v", err)
			}
			if _, err := os.Stat(filepath.Join(root, ".codex/auth.json")); err != nil {
				t.Fatalf("seed precondition: %v", err)
			}

			exec := NewDockerExecutor(config.DockerConfig{}, kandevHome, log)
			instance := &ExecutorInstance{
				InstanceID: instanceID,
				StopReason: tc.reason,
				// ContainerID intentionally empty so StopInstance returns
				// before invoking the docker daemon — we're testing the
				// kandev-side cleanup branch only.
			}
			if err := exec.StopInstance(context.Background(), instance, false); err != nil {
				t.Fatalf("StopInstance returned error for reason=%q: %v", tc.reason, err)
			}

			_, statErr := os.Stat(root)
			gotExists := statErr == nil
			if gotExists != tc.expectExists {
				t.Fatalf("session dir for reason=%q: exists=%v, want=%v (statErr=%v)",
					tc.reason, gotExists, tc.expectExists, statErr)
			}
		})
	}
}
