package managedruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRepairStorePublication(t *testing.T) {
	store, home := openTestRepairStore(t)
	input := testRepairContextInput(t, home, "managed-acp@1.2.3")
	repairContext, err := store.Context(input)
	if err != nil {
		t.Fatalf("complete repair context: %v", err)
	}
	if prepared, outcome, err := store.Lookup(context.Background(), repairContext); err != nil || prepared != nil || outcome != RepairOutcomeMiss {
		t.Fatalf("initial lookup = (%v, %s, %v), want a miss", prepared, outcome, err)
	}
	lease, err := store.BeginRecovery(context.Background(), repairContext)
	if err != nil {
		t.Fatalf("begin recovery: %v", err)
	}
	if _, outcome, err := lease.Reuse(context.Background()); err != nil || outcome != RepairOutcomeMiss {
		t.Fatalf("recovery lookup before publication = (%s, %v), want a miss", outcome, err)
	}
	prepared, err := lease.Prepare(context.Background(), testRepairEnv(input.Env), testRepairRunner(t, input.PackageSpec))
	if err != nil {
		t.Fatalf("prepare generation: %v", err)
	}
	if prepared.RepairPublished() {
		t.Fatal("preparation published before protocol initialization")
	}
	if _, outcome, _ := store.Lookup(context.Background(), repairContext); outcome != RepairOutcomeMiss {
		t.Fatalf("unpublished generation outcome = %s, want a miss", outcome)
	}
	if err := prepared.PublishRepair(); err != nil {
		t.Fatalf("publish repair: %v", err)
	}
	if err := prepared.PublishRepair(); err != nil {
		t.Fatalf("idempotent publication: %v", err)
	}
	hit, outcome, err := store.Lookup(context.Background(), repairContext)
	if err != nil || outcome != RepairOutcomeHit || hit == nil {
		t.Fatalf("lookup after publication = (%v, %s, %v)", hit, outcome, err)
	}
	if !pathWithin(store.RepairRoot(), hit.ProjectPrefix()) || !pathWithin(store.RepairRoot(), hit.CacheDirectory()) {
		t.Fatalf("published paths %q and %q are outside the store root", hit.ProjectPrefix(), hit.CacheDirectory())
	}
	generationRoot := filepath.Dir(hit.ProjectPrefix())
	if err := hit.Close(); err != nil {
		t.Fatalf("close published handle: %v", err)
	}
	if _, err := os.Stat(generationRoot); err != nil {
		t.Fatalf("published generation after handle close = %v, want present", err)
	}

	reopened, err := OpenRepairStore(home)
	if err != nil {
		t.Fatalf("reopen repair store: %v", err)
	}
	second, outcome, err := reopened.Lookup(context.Background(), repairContext)
	if err != nil || outcome != RepairOutcomeHit || second == nil {
		t.Fatalf("lookup after reopen = (%v, %s, %v)", second, outcome, err)
	}
	if second.ProjectPrefix() != hit.ProjectPrefix() || second.CacheDirectory() != hit.CacheDirectory() {
		t.Fatalf("reopened paths = %q/%q, want stable %q/%q", second.ProjectPrefix(), second.CacheDirectory(), hit.ProjectPrefix(), hit.CacheDirectory())
	}
	if err := second.Close(); err != nil {
		t.Fatalf("close reopened handle: %v", err)
	}
}

func TestRepairStoreUnpublishedCleanup(t *testing.T) {
	store, home := openTestRepairStore(t)
	input := testRepairContextInput(t, home, "managed-acp@1.2.3")
	repairContext, err := store.Context(input)
	if err != nil {
		t.Fatalf("complete repair context: %v", err)
	}
	lease, err := store.BeginRecovery(context.Background(), repairContext)
	if err != nil {
		t.Fatalf("begin recovery: %v", err)
	}
	prepared, err := lease.Prepare(context.Background(), testRepairEnv(input.Env), testRepairRunner(t, input.PackageSpec))
	if err != nil {
		t.Fatalf("prepare generation: %v", err)
	}
	root := filepath.Dir(prepared.ProjectPrefix())
	if err := prepared.Close(); err != nil {
		t.Fatalf("close unpublished generation: %v", err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("unpublished generation root = %v, want removed", err)
	}
	second, err := store.BeginRecovery(context.Background(), repairContext)
	if err != nil {
		t.Fatalf("lock was not released after unpublished cleanup: %v", err)
	}
	_ = second.Close()
}

func TestRepairStoreCancellationRemovesCandidate(t *testing.T) {
	store, home := openTestRepairStore(t)
	input := testRepairContextInput(t, home, "managed-acp@1.2.3")
	repairContext, err := store.Context(input)
	if err != nil {
		t.Fatalf("complete repair context: %v", err)
	}
	lease, err := store.BeginRecovery(context.Background(), repairContext)
	if err != nil {
		t.Fatalf("begin recovery: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancelled := make(chan struct{})
	prepared, prepareErr := lease.Prepare(ctx, testRepairEnv(input.Env), func(runCtx context.Context, _ ManagedRuntimeCommand) error {
		cancel()
		close(cancelled)
		<-runCtx.Done()
		return runCtx.Err()
	})
	if prepared != nil || !errors.Is(prepareErr, context.Canceled) {
		t.Fatalf("cancelled preparation = (%v, %v), want context cancellation", prepared, prepareErr)
	}
	<-cancelled
	if err := lease.Close(); err != nil {
		t.Fatalf("close cancelled lease: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(store.RepairRoot(), repairStoreGenerationDir))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("cancelled preparation left %d generations", len(entries))
	}
}

func TestRepairStoreInvalidation(t *testing.T) {
	store, home := openTestRepairStore(t)
	input := testRepairContextInput(t, home, "managed-acp@1.2.3")
	repairContext, err := store.Context(input)
	if err != nil {
		t.Fatalf("complete repair context: %v", err)
	}
	digest := mustRepairContextDigest(t, store, repairContext)
	prepared := publishTestRepair(t, store, input, repairContext)
	generationRoot := filepath.Dir(prepared.ProjectPrefix())
	selectionPath := filepath.Join(store.RepairRoot(), repairStoreContextsDir, digest, repairSelectionRecord)

	t.Run("corrupt selection", func(t *testing.T) {
		original, err := os.ReadFile(selectionPath)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.WriteFile(selectionPath, original, 0o600) })
		if err := os.WriteFile(selectionPath, []byte("{not-json"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, outcome, _ := store.Lookup(context.Background(), repairContext); outcome != RepairOutcomeInvalid {
			t.Fatalf("corrupt selection outcome = %s, want invalid", outcome)
		}
		if _, err := os.Stat(generationRoot); err != nil {
			t.Fatalf("invalid selection removed generation files: %v", err)
		}
	})

	t.Run("symlinked selection", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("symlink creation requires privileges on Windows")
		}
		outside := filepath.Join(t.TempDir(), "selection.json")
		original, err := os.ReadFile(selectionPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(outside, original, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(selectionPath); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, selectionPath); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = os.Remove(selectionPath)
			_ = os.WriteFile(selectionPath, original, 0o600)
		})
		if _, outcome, _ := store.Lookup(context.Background(), repairContext); outcome != RepairOutcomeInvalid {
			t.Fatalf("symlinked selection outcome = %s, want invalid", outcome)
		}
	})

	t.Run("damaged package tree", func(t *testing.T) {
		executionRoot := filepath.Join(prepared.CacheDirectory(), "_npx", NpxExecutionCacheKey(input.PackageSpec))
		manifestPath := filepath.Join(executionRoot, "package.json")
		original, err := os.ReadFile(manifestPath)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.WriteFile(manifestPath, original, 0o600) })
		if err := os.Remove(manifestPath); err != nil {
			t.Fatal(err)
		}
		if _, outcome, _ := store.Lookup(context.Background(), repairContext); outcome != RepairOutcomeInvalid {
			t.Fatalf("damaged generation outcome = %s, want invalid", outcome)
		}
		if _, err := os.Stat(generationRoot); err != nil {
			t.Fatalf("invalid generation files were removed: %v", err)
		}
		if err := os.WriteFile(manifestPath, original, 0o600); err != nil {
			t.Fatal(err)
		}
	})
}

func TestRepairStoreOldConsumerSurvivesSuccessor(t *testing.T) {
	store, home := openTestRepairStore(t)
	input := testRepairContextInput(t, home, "managed-acp@1.2.3")
	repairContext, err := store.Context(input)
	if err != nil {
		t.Fatalf("complete repair context: %v", err)
	}
	old := publishTestRepair(t, store, input, repairContext)
	oldSentinel := filepath.Join(old.ProjectPrefix(), "old-consumer.txt")
	if err := os.WriteFile(oldSentinel, []byte("old consumer\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	digest := mustRepairContextDigest(t, store, repairContext)
	selectionPath := filepath.Join(store.RepairRoot(), repairStoreContextsDir, digest, repairSelectionRecord)
	if err := os.Remove(selectionPath); err != nil {
		t.Fatal(err)
	}
	successor := publishTestRepair(t, store, input, repairContext)
	if successor.ProjectPrefix() == old.ProjectPrefix() {
		t.Fatal("successor reused the same generation directory")
	}
	if contents, err := os.ReadFile(oldSentinel); err != nil || string(contents) != "old consumer\n" {
		t.Fatalf("old consumer file = %q, %v; want preserved", contents, err)
	}
	if err := old.Close(); err != nil {
		t.Fatalf("close old consumer handle: %v", err)
	}
	hit, outcome, err := store.Lookup(context.Background(), repairContext)
	if err != nil || outcome != RepairOutcomeHit || hit.CacheDirectory() != successor.CacheDirectory() {
		t.Fatalf("successor lookup = (%v, %s, %v), want the new generation", hit, outcome, err)
	}
	_ = hit.Close()
}

func TestRepairStoreConcurrency(t *testing.T) {
	store, home := openTestRepairStore(t)
	firstInput := testRepairContextInput(t, home, "managed-acp@1.2.3")
	secondInput := testRepairContextInput(t, home, "managed-acp@1.2.4")
	first, err := store.Context(firstInput)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Context(secondInput)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.BeginRecovery(context.Background(), first)
	if err != nil {
		t.Fatalf("begin first recovery: %v", err)
	}
	independent, err := store.BeginRecovery(context.Background(), second)
	if err != nil {
		t.Fatalf("independent context was blocked by an unrelated lock: %v", err)
	}
	_ = independent.Close()

	waiterDone := make(chan error, 1)
	go func() {
		waiter, waitErr := store.BeginRecovery(context.Background(), first)
		if waitErr == nil {
			_ = waiter.Close()
		}
		waiterDone <- waitErr
	}()
	select {
	case err := <-waiterDone:
		t.Fatalf("waiter acquired a held context lock: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	if err := lease.Close(); err != nil {
		t.Fatalf("release first lease: %v", err)
	}
	select {
	case err := <-waiterDone:
		if err != nil {
			t.Fatalf("waiter after release: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waiter did not acquire the released context lock")
	}

	cancelledCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	lease, err = store.BeginRecovery(context.Background(), first)
	if err != nil {
		t.Fatalf("reacquire first recovery: %v", err)
	}
	if _, err := store.BeginRecovery(cancelledCtx, first); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancelled waiter error = %v, want deadline exceeded", err)
	}
	_ = lease.Close()
}

func TestRepairStoreLockAcrossProcesses(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the cross-process helper uses POSIX pipes")
	}
	store, home := openTestRepairStore(t)
	input := testRepairContextInput(t, home, "managed-acp@1.2.3")
	repairContext, err := store.Context(input)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(struct {
		Input RepairContextInput
		Env   []string
	}{Input: input, Env: input.Env})
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestRepairStoreLockHelperProcess$", "-test.timeout=60s")
	command.Env = append(os.Environ(),
		"KANDEV_REPAIR_LOCK_HELPER=1",
		"KANDEV_REPAIR_LOCK_HOME="+home,
		"KANDEV_REPAIR_LOCK_CONTEXT="+string(encoded),
	)
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatalf("start lock helper: %v", err)
	}
	t.Cleanup(func() { _ = command.Process.Kill() })
	if err := waitForRepairHelperLine(stdout); err != nil {
		t.Fatalf("lock helper did not report readiness: %v", err)
	}
	blockedCtx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if _, err := store.BeginRecovery(blockedCtx, repairContext); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cross-process lock error = %v, want deadline exceeded", err)
	}
	_ = stdin.Close()
	if err := command.Wait(); err != nil {
		t.Fatalf("lock helper exit: %v", err)
	}
	acquiredCtx, acquireCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer acquireCancel()
	lease, err := store.BeginRecovery(acquiredCtx, repairContext)
	if err != nil {
		t.Fatalf("acquire lock after helper exit: %v", err)
	}
	_ = lease.Close()
}

func TestRepairStoreLockHelperProcess(t *testing.T) {
	if os.Getenv("KANDEV_REPAIR_LOCK_HELPER") != "1" {
		t.Skip("helper process")
	}
	var payload struct {
		Input RepairContextInput
		Env   []string
	}
	if err := json.Unmarshal([]byte(os.Getenv("KANDEV_REPAIR_LOCK_CONTEXT")), &payload); err != nil {
		t.Fatalf("decode helper context: %v", err)
	}
	store, err := OpenRepairStore(os.Getenv("KANDEV_REPAIR_LOCK_HOME"))
	if err != nil {
		t.Fatalf("helper open store: %v", err)
	}
	repairContext, err := store.Context(payload.Input)
	if err != nil {
		t.Fatalf("helper complete context: %v", err)
	}
	lease, err := store.BeginRecovery(context.Background(), repairContext)
	if err != nil {
		t.Fatalf("helper acquire lock: %v", err)
	}
	_, _ = os.Stdout.WriteString("locked\n")
	buffer := make([]byte, 1)
	_, _ = os.Stdin.Read(buffer)
	_ = lease.Close()
}

func waitForRepairHelperLine(stdout interface{ Read([]byte) (int, error) }) error {
	deadline := time.Now().Add(10 * time.Second)
	line := make([]byte, 0, 32)
	buffer := make([]byte, 1)
	for time.Now().Before(deadline) {
		read, err := stdout.Read(buffer)
		if err != nil {
			return err
		}
		if read == 0 {
			continue
		}
		if buffer[0] == '\n' {
			if strings.TrimSpace(string(line)) == "locked" {
				return nil
			}
			line = line[:0]
			continue
		}
		line = append(line, buffer[0])
	}
	return errors.New("timed out waiting for lock helper")
}

func publishTestRepair(t *testing.T, store *RepairStore, input RepairContextInput, repairContext RepairContext) *PreparedRuntime {
	t.Helper()
	lease, err := store.BeginRecovery(context.Background(), repairContext)
	if err != nil {
		t.Fatalf("begin recovery: %v", err)
	}
	prepared, err := lease.Prepare(context.Background(), testRepairEnv(input.Env), testRepairRunner(t, input.PackageSpec))
	if err != nil {
		t.Fatalf("prepare generation: %v", err)
	}
	if err := prepared.PublishRepair(); err != nil {
		t.Fatalf("publish generation: %v", err)
	}
	t.Cleanup(func() { _ = prepared.Close() })
	return prepared
}

func TestManagedRuntimeRepairReuseLifetime(t *testing.T) {
	store, home := openTestRepairStore(t)
	input := testRepairContextInput(t, home, "managed-acp@1.2.3")
	repairContext, err := store.Context(input)
	if err != nil {
		t.Fatalf("complete repair context: %v", err)
	}
	first := publishTestRepair(t, store, input, repairContext)
	firstRoot := filepath.Dir(first.ProjectPrefix())
	if err := first.Close(); err != nil {
		t.Fatalf("close first published owner: %v", err)
	}

	restarted, err := OpenRepairStore(home)
	if err != nil {
		t.Fatalf("reopen repair store: %v", err)
	}
	second, outcome, err := restarted.Lookup(context.Background(), repairContext)
	if err != nil || outcome != RepairOutcomeHit || second == nil {
		t.Fatalf("restart lookup = (%v, %s, %v), want a hit", second, outcome, err)
	}
	if second.CacheDirectory() != first.CacheDirectory() {
		t.Fatalf("restart selected %q, want %q", second.CacheDirectory(), first.CacheDirectory())
	}
	if err := second.Close(); err != nil {
		t.Fatalf("close restarted owner: %v", err)
	}
	if _, err := os.Stat(firstRoot); err != nil {
		t.Fatalf("published generation after two owner lifetimes = %v, want present", err)
	}
}

func TestRepairStoreRejectsUnsafeRootsAndIdentityFiles(t *testing.T) {
	workspace := t.TempDir()
	if _, err := OpenRepairStore(workspace, workspace); err == nil {
		t.Fatal("opened a repair store inside an excluded workspace root")
	}
	store, home := openTestRepairStore(t)
	root := store.RepairRoot()
	keyPath := filepath.Join(root, repairStoreKeyFile)
	ownerPath := filepath.Join(root, repairStoreOwnerMarker)
	originalKey, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	originalOwner, err := os.ReadFile(ownerPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("oversized key", func(t *testing.T) {
		if err := os.WriteFile(keyPath, bytes.Repeat([]byte("a"), 256), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := OpenRepairStore(home); err == nil {
			t.Fatal("accepted an oversized fingerprint key")
		}
	})
	t.Run("nonprivate key", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Unix permission bits are required")
		}
		if err := os.WriteFile(keyPath, originalKey, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(keyPath, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := OpenRepairStore(home); err == nil {
			t.Fatal("accepted a group-readable fingerprint key")
		}
	})
	t.Run("symlinked owner marker", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("symlink creation requires privileges on Windows")
		}
		outside := filepath.Join(t.TempDir(), "owner")
		if err := os.WriteFile(outside, originalOwner, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(ownerPath); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, ownerPath); err != nil {
			t.Fatal(err)
		}
		if _, err := OpenRepairStore(home); err == nil {
			t.Fatal("accepted a symlinked owner marker")
		}
	})
}

func TestRepairStoreRejectsTamperedSelectionMAC(t *testing.T) {
	store, home := openTestRepairStore(t)
	input := testRepairContextInput(t, home, "managed-acp@1.2.3")
	repairContext, err := store.Context(input)
	if err != nil {
		t.Fatalf("complete repair context: %v", err)
	}
	published := publishTestRepair(t, store, input, repairContext)
	digest := mustRepairContextDigest(t, store, repairContext)
	selectionPath := filepath.Join(store.RepairRoot(), repairStoreContextsDir, digest, repairSelectionRecord)
	data, err := os.ReadFile(selectionPath)
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	record["generation_id"] = strings.Repeat("0", 32)
	tampered, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(selectionPath, tampered, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, outcome, _ := store.Lookup(context.Background(), repairContext); outcome != RepairOutcomeInvalid {
		t.Fatalf("tampered selection outcome = %s, want invalid", outcome)
	}
	if _, err := os.Stat(filepath.Dir(published.ProjectPrefix())); err != nil {
		t.Fatalf("tampered selection removed generation files: %v", err)
	}
}

func TestRepairStorePublicationFailureIsTerminal(t *testing.T) {
	store, home := openTestRepairStore(t)
	input := testRepairContextInput(t, home, "managed-acp@1.2.3")
	repairContext, err := store.Context(input)
	if err != nil {
		t.Fatalf("complete repair context: %v", err)
	}
	lease, err := store.BeginRecovery(context.Background(), repairContext)
	if err != nil {
		t.Fatalf("begin recovery: %v", err)
	}
	prepared, err := lease.Prepare(context.Background(), testRepairEnv(input.Env), testRepairRunner(t, input.PackageSpec))
	if err != nil {
		t.Fatalf("prepare generation: %v", err)
	}
	digest := mustRepairContextDigest(t, store, repairContext)
	contextDir := filepath.Join(store.RepairRoot(), repairStoreContextsDir, digest)
	if err := os.RemoveAll(contextDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(contextDir, []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := prepared.PublishRepair(); err == nil {
		t.Fatal("publication succeeded without a writable context directory")
	}
	if err := os.Remove(contextDir); err != nil {
		t.Fatal(err)
	}
	successor := publishTestRepair(t, store, input, repairContext)
	if err := prepared.PublishRepair(); err == nil {
		t.Fatal("a failed publication owner retried without a lease")
	}
	if _, outcome, err := store.Lookup(context.Background(), repairContext); err != nil || outcome != RepairOutcomeHit {
		t.Fatalf("successor lookup = (%s, %v), want a hit", outcome, err)
	}
	if _, err := os.Stat(filepath.Dir(successor.ProjectPrefix())); err != nil {
		t.Fatalf("successor files were removed by the old owner: %v", err)
	}
}

func TestRepairStoreOwnerBoundaries(t *testing.T) {
	store, home := openTestRepairStore(t)
	firstInput := testRepairContextInput(t, home, "managed-acp@1.2.3")
	first, err := store.Context(firstInput)
	if err != nil {
		t.Fatal(err)
	}
	published := publishTestRepair(t, store, firstInput, first)

	secondInput := firstInput
	secondInput.OwnerScope = RepairOwnerScopeTask("owner-b")
	second, err := store.Context(secondInput)
	if err != nil {
		t.Fatal(err)
	}
	if _, outcome, err := store.Lookup(context.Background(), second); err != nil || outcome != RepairOutcomeMiss {
		t.Fatalf("other owner lookup = (%s, %v), want a miss", outcome, err)
	}

	compatible := firstInput
	compatible.Env = append(compatible.Env, "KANDEV_TASK_ID=another-task", "KANDEV_SESSION_ID=another-session")
	shared, err := store.Context(compatible)
	if err != nil {
		t.Fatal(err)
	}
	if !store.MatchesRepairContext(published, shared) {
		t.Fatal("compatible tasks of one owner did not share the repair")
	}
	hit, outcome, err := store.Lookup(context.Background(), shared)
	if err != nil || outcome != RepairOutcomeHit || hit == nil {
		t.Fatalf("compatible task lookup = (%v, %s, %v), want a hit", hit, outcome, err)
	}
	if store.MatchesRepairContext(published, second) {
		t.Fatal("another owner matched the published repair context")
	}
}

func TestRepairPublishedConcurrentObservation(t *testing.T) {
	store, home := openTestRepairStore(t)
	input := testRepairContextInput(t, home, "managed-acp@1.2.3")
	repairContext, err := store.Context(input)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.BeginRecovery(context.Background(), repairContext)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := lease.Prepare(context.Background(), testRepairEnv(input.Env), testRepairRunner(t, input.PackageSpec))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for index := 0; index < 1000; index++ {
			_ = prepared.RepairPublished()
		}
	}()
	if err := prepared.PublishRepair(); err != nil {
		t.Fatalf("publish repair: %v", err)
	}
	<-done
	if !prepared.RepairPublished() {
		t.Fatal("published repair is not observable")
	}
}

func TestRepairLeasePreservesHandleWhenCleanupCannotFinish(t *testing.T) {
	store, home := openTestRepairStore(t)
	input := testRepairContextInput(t, home, "managed-acp@1.2.3")
	repairContext, err := store.Context(input)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.BeginRecovery(context.Background(), repairContext)
	if err != nil {
		t.Fatal(err)
	}
	live := true
	prepared, err := lease.Prepare(context.Background(), input.Env, func(_ context.Context, command ManagedRuntimeCommand) error {
		command.RetainWhile(func() bool { return live })
		return errors.New("installer failed")
	})
	if err == nil {
		t.Fatal("expected preparation failure")
	}
	if prepared == nil {
		t.Fatal("failed cleanup lost the owned handle")
	}
	if live {
		live = false
	}
	if closeErr := prepared.Close(); closeErr != nil {
		t.Fatalf("close retained handle: %v", closeErr)
	}
	if err := lease.Close(); err != nil {
		t.Fatalf("close retained lease: %v", err)
	}
}
