package changes

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/turnchanges"
)

func TestTurnGitHistoryReadsAcceptedIntervalWithoutSQLPayloads(t *testing.T) {
	archive, repoDir, startTree, endTree := buildTurnArchiveFixture(t)
	store := NewGitHistoryStore(t.TempDir(), nil)
	receipt, err := store.Import(context.Background(), uuid.NewString(), uuid.NewString(), archive)
	if err != nil {
		t.Fatalf("import accepted archive: %v", err)
	}
	readArchive := turnchanges.CheckpointArchive{StartTreeOID: startTree, EndTreeOID: endTree}
	patch, err := store.ReadPatch(context.Background(), receipt,
		readArchive, HistoricalFile{PathBytes: []byte("src/app.txt")}, false)
	if err != nil {
		t.Fatalf("read canonical patch: %v", err)
	}
	if !strings.Contains(string(patch), "-old") || !strings.Contains(string(patch), "+new") {
		t.Fatalf("canonical patch = %q", patch)
	}
	filtered, err := store.ReadPatch(context.Background(), receipt,
		readArchive, HistoricalFile{PathBytes: []byte("src/app.txt")}, true)
	if err != nil {
		t.Fatalf("read filtered patch: %v", err)
	}
	if len(filtered) == 0 {
		t.Fatal("filtered patch is empty")
	}
	// The original working tree is not needed once the archive exists.
	if err := os.RemoveAll(repoDir); err != nil {
		t.Fatalf("remove original worktree: %v", err)
	}
	retry, err := store.ReadPatch(context.Background(), receipt,
		readArchive, HistoricalFile{PathBytes: []byte("src/app.txt")}, false)
	if err != nil {
		t.Fatalf("read patch after worktree removal: %v", err)
	}
	if !bytes.Equal(patch, retry) {
		t.Fatal("historical patch changed after the original worktree was removed")
	}
	if receipt.ObjectCount == 0 || receipt.StoredBytes == 0 || receipt.PackSHA256 == "" {
		t.Fatalf("archive receipt = %+v", receipt)
	}
}

func TestTurnGitHistoryImportsCheckpointCommitsWithMissingParents(t *testing.T) {
	archive, repoDir, startTree, endTree := buildTurnArchiveFixture(t)
	parent := strings.TrimSpace(runGitTest(t, repoDir, "rev-parse", archive.EndCommitOID))
	startCommit := strings.TrimSpace(runGitTest(t, repoDir, "commit-tree", startTree, "-p", parent, "-m", "start capture"))
	endCommit := strings.TrimSpace(runGitTest(t, repoDir, "commit-tree", endTree, "-p", parent, "-m", "end capture"))
	objects := turnArchiveObjectList(t, repoDir, startTree, endTree, startCommit, endCommit)
	pack := runGitWithInput(t, repoDir, strings.Join(objects, "\n")+"\n", "pack-objects", "--stdout", "-q")
	archive.StartCommitOID, archive.EndCommitOID = startCommit, endCommit
	archive.Pack, archive.Objects, archive.ObjectCount = pack, objects, int64(len(objects))

	store := NewGitHistoryStore(t.TempDir(), nil)
	receipt, err := store.Import(context.Background(), "repo-parentless", "checkout-parentless", archive)
	if err != nil {
		t.Fatalf("import checkpoint commits with source parents: %v", err)
	}
	if err := store.VerifyRefs(context.Background(), receipt); err != nil {
		t.Fatalf("verify imported refs: %v", err)
	}
	patch, err := store.ReadPatch(context.Background(), receipt,
		turnchanges.CheckpointArchive{StartTreeOID: startTree, EndTreeOID: endTree},
		HistoricalFile{PathBytes: []byte("src/app.txt")}, false)
	if err != nil || !strings.Contains(string(patch), "+new") {
		t.Fatalf("historical patch = %q, %v", patch, err)
	}
}

func TestTurnGitHistorySurvivesWorktreeRemovalAndDeletesOnlyOwnedRefs(t *testing.T) {
	archive, repoDir, _, _ := buildTurnArchiveFixture(t)
	store := NewGitHistoryStore(t.TempDir(), nil)
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	receipt, err := store.Import(context.Background(), uuid.NewString(), checkoutID, &turnchanges.CheckpointArchive{
		ChangeSetID: changeSetID, CheckoutID: checkoutID, HashAlgorithm: archive.HashAlgorithm,
		StartCommitOID: archive.StartCommitOID, StartTreeOID: archive.StartTreeOID,
		EndCommitOID: archive.EndCommitOID, EndTreeOID: archive.EndTreeOID,
		Pack: archive.Pack, Objects: archive.Objects, ObjectCount: archive.ObjectCount,
		UncompressedBytes: archive.UncompressedBytes, Complete: true,
	})
	if err != nil {
		t.Fatalf("import archive: %v", err)
	}
	if err := os.RemoveAll(repoDir); err != nil {
		t.Fatalf("remove original worktree: %v", err)
	}
	if err := store.VerifyRefs(context.Background(), receipt); err != nil {
		t.Fatalf("verify retained refs: %v", err)
	}
	moved := receipt
	moved.StartRef = receipt.EndRef
	if err := store.DeleteRefs(context.Background(), moved); err == nil {
		t.Fatal("cleanup accepted a ref that does not carry the expected commit")
	}
	if err := store.DeleteRefs(context.Background(), receipt); err != nil {
		t.Fatalf("delete owned refs: %v", err)
	}
	if err := store.VerifyRefs(context.Background(), receipt); !errors.Is(err, ErrHistoryContentUnavailable) {
		t.Fatalf("verify after cleanup = %v, want explicit missing source", err)
	}
}

func TestTurnGitHistoryRejectsInvalidArchiveWithoutTouchingStore(t *testing.T) {
	store := NewGitHistoryStore(t.TempDir(), nil)
	_, err := store.Import(context.Background(), uuid.NewString(), uuid.NewString(), &turnchanges.CheckpointArchive{
		ChangeSetID: uuid.NewString(), CheckoutID: uuid.NewString(), Complete: false,
	})
	if !errors.Is(err, ErrHistoryContentUnavailable) {
		t.Fatalf("incomplete archive error = %v", err)
	}
}

func buildTurnArchiveFixture(t *testing.T) (*turnchanges.CheckpointArchive, string, string, string) {
	t.Helper()
	repoDir := t.TempDir()
	runGitTest(t, repoDir, "init", "-q")
	runGitTest(t, repoDir, "config", "user.email", "turn@example.invalid")
	runGitTest(t, repoDir, "config", "user.name", "Turn Test")
	if err := os.MkdirAll(filepath.Join(repoDir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTurnArchiveFile(t, repoDir, "src/app.txt", "old\n")
	runGitTest(t, repoDir, "add", ".")
	runGitTest(t, repoDir, "commit", "-qm", "base")
	startCommit := strings.TrimSpace(runGitTest(t, repoDir, "rev-parse", "HEAD"))
	startTree := strings.TrimSpace(runGitTest(t, repoDir, "rev-parse", "HEAD^{tree}"))

	writeTurnArchiveFile(t, repoDir, "src/app.txt", "new\n")
	writeTurnArchiveFile(t, repoDir, "src/added.txt", "added\n")
	runGitTest(t, repoDir, "add", ".")
	runGitTest(t, repoDir, "commit", "-qm", "turn")
	endCommit := strings.TrimSpace(runGitTest(t, repoDir, "rev-parse", "HEAD"))
	endTree := strings.TrimSpace(runGitTest(t, repoDir, "rev-parse", "HEAD^{tree}"))

	objects := turnArchiveObjectList(t, repoDir, startTree, endTree, startCommit, endCommit)
	pack := runGitWithInput(t, repoDir, strings.Join(objects, "\n")+"\n", "pack-objects", "--stdout", "-q")
	return &turnchanges.CheckpointArchive{
		ChangeSetID: uuid.NewString(), CheckoutID: uuid.NewString(), HashAlgorithm: "sha1",
		StartCommitOID: startCommit, StartTreeOID: startTree, EndCommitOID: endCommit, EndTreeOID: endTree,
		Pack: pack, Objects: objects, ObjectCount: int64(len(objects)), Complete: true,
	}, repoDir, startTree, endTree
}

func turnArchiveObjectList(t *testing.T, repoDir, startTree, endTree, startCommit, endCommit string) []string {
	t.Helper()
	seen := make(map[string]struct{})
	objects := make([]string, 0, 32)
	for _, root := range []string{startTree, endTree} {
		output := runGitTest(t, repoDir, "ls-tree", "-r", "-t", "-z", "--full-tree", root)
		for _, record := range strings.Split(output, "\x00") {
			metadata, _, ok := strings.Cut(record, "\t")
			if !ok {
				continue
			}
			fields := strings.Fields(metadata)
			if len(fields) != 3 || fields[1] == "commit" {
				continue
			}
			if _, exists := seen[fields[2]]; !exists {
				seen[fields[2]] = struct{}{}
				objects = append(objects, fields[2])
			}
		}
		if _, exists := seen[root]; !exists {
			seen[root] = struct{}{}
			objects = append(objects, root)
		}
	}
	for _, commit := range []string{startCommit, endCommit} {
		if _, exists := seen[commit]; !exists {
			seen[commit] = struct{}{}
			objects = append(objects, commit)
		}
	}
	return objects
}

func writeTurnArchiveFile(t *testing.T, dir, path, content string) {
	t.Helper()
	full := filepath.Join(dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func runGitTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}

func runGitWithInput(t *testing.T, dir, input string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Stdin = strings.NewReader(input)
	var out bytes.Buffer
	cmd.Stdout = &out
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, stderr.String())
	}
	return out.Bytes()
}

func TestTurnGitHistoryReadPatchReportsTemporaryFailureAsRetryable(t *testing.T) {
	store := NewGitHistoryStore(t.TempDir(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := store.ReadPatch(ctx, ArchiveReceipt{SourceID: "missing"}, turnchanges.CheckpointArchive{}, HistoricalFile{PathBytes: []byte("a.txt")}, false)
	if !errors.Is(err, ErrHistoryReadRetryable) {
		t.Fatalf("canceled read = %v, want a retryable error", err)
	}
}

func TestTurnGitHistorySourceKeyIsServerDerived(t *testing.T) {
	store := NewGitHistoryStore(t.TempDir(), nil)
	first := store.sourceKey("../escape", "")
	second := store.sourceKey("aa/bb", "")
	if strings.Contains(first, "/") || strings.Contains(first, "..") || len(first) != 64 {
		t.Fatalf("unsafe source key = %q", first)
	}
	if strings.Contains(second, "/") || len(second) != 64 {
		t.Fatalf("unsafe source key = %q", second)
	}
	if store.sourceKey("repository-id", "checkout-id") != "repository-id" {
		t.Fatal("repository identity should win when it is already safe")
	}
}

func TestTurnGitHistoryImportIsIdempotentForAcceptedPair(t *testing.T) {
	archive, _, _, _ := buildTurnArchiveFixture(t)
	store := NewGitHistoryStore(t.TempDir(), nil)
	first, err := store.Import(context.Background(), "repo", "checkout", archive)
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	second, err := store.Import(context.Background(), "repo", "checkout", archive)
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if first.PackSHA256 != second.PackSHA256 || first.SourceID != second.SourceID {
		t.Fatalf("idempotent receipts differ: first=%+v second=%+v", first, second)
	}
	entries, err := os.ReadDir(filepath.Join(store.repositoryPath(second.SourceID), "objects", "pack"))
	if err != nil {
		t.Fatal(err)
	}
	packs := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".pack") {
			packs++
		}
	}
	if packs != 1 {
		t.Fatalf("idempotent import stored %d packs, want one", packs)
	}
}

func TestTurnGitHistoryReadsSpecialPaths(t *testing.T) {
	repoDir := t.TempDir()
	runGitTest(t, repoDir, "init", "-q")
	runGitTest(t, repoDir, "config", "user.email", "turn@example.invalid")
	runGitTest(t, repoDir, "config", "user.name", "Turn Test")
	special := "dir/with space/na\tme.txt"
	writeTurnArchiveFile(t, repoDir, special, "before\n")
	runGitTest(t, repoDir, "add", ".")
	runGitTest(t, repoDir, "commit", "-qm", "base")
	startCommit := strings.TrimSpace(runGitTest(t, repoDir, "rev-parse", "HEAD"))
	startTree := strings.TrimSpace(runGitTest(t, repoDir, "rev-parse", "HEAD^{tree}"))
	writeTurnArchiveFile(t, repoDir, special, "after\n")
	runGitTest(t, repoDir, "add", ".")
	runGitTest(t, repoDir, "commit", "-qm", "turn")
	endCommit := strings.TrimSpace(runGitTest(t, repoDir, "rev-parse", "HEAD"))
	endTree := strings.TrimSpace(runGitTest(t, repoDir, "rev-parse", "HEAD^{tree}"))
	objects := turnArchiveObjectList(t, repoDir, startTree, endTree, startCommit, endCommit)
	pack := runGitWithInput(t, repoDir, strings.Join(objects, "\n")+"\n", "pack-objects", "--stdout", "-q")
	store := NewGitHistoryStore(t.TempDir(), nil)
	receipt, err := store.Import(context.Background(), "repo", "checkout", &turnchanges.CheckpointArchive{
		ChangeSetID: uuid.NewString(), CheckoutID: uuid.NewString(), HashAlgorithm: "sha1",
		StartCommitOID: startCommit, StartTreeOID: startTree, EndCommitOID: endCommit, EndTreeOID: endTree,
		Pack: pack, Objects: objects, ObjectCount: int64(len(objects)), Complete: true,
	})
	if err != nil {
		t.Fatalf("import special-path archive: %v", err)
	}
	patch, err := store.ReadPatch(context.Background(), receipt,
		turnchanges.CheckpointArchive{StartTreeOID: startTree, EndTreeOID: endTree},
		HistoricalFile{PathBytes: []byte(special)}, false)
	if err != nil {
		t.Fatalf("read special path: %v", err)
	}
	if !strings.Contains(string(patch), "before") || !strings.Contains(string(patch), "after") {
		t.Fatalf("special path patch = %q", patch)
	}
}

func TestTurnGitHistorySelectsTheCopiedDestinationSection(t *testing.T) {
	repoDir := t.TempDir()
	runGitTest(t, repoDir, "init", "-q")
	runGitTest(t, repoDir, "config", "user.email", "turn@example.invalid")
	runGitTest(t, repoDir, "config", "user.name", "Turn Test")
	writeTurnArchiveFile(t, repoDir, "source.txt", "source old\n")
	runGitTest(t, repoDir, "add", ".")
	runGitTest(t, repoDir, "commit", "-qm", "base")
	startCommit := strings.TrimSpace(runGitTest(t, repoDir, "rev-parse", "HEAD"))
	writeTurnArchiveFile(t, repoDir, "source.txt", "source new\n")
	writeTurnArchiveFile(t, repoDir, "copy.txt", "source new\n")
	runGitTest(t, repoDir, "add", ".")
	runGitTest(t, repoDir, "commit", "-qm", "turn")
	endCommit := strings.TrimSpace(runGitTest(t, repoDir, "rev-parse", "HEAD"))
	archive, startTree, endTree := archiveForCommits(t, repoDir, startCommit, endCommit)
	store := NewGitHistoryStore(t.TempDir(), nil)
	receipt, err := store.Import(context.Background(), "repo-copy", "checkout-copy", archive)
	if err != nil {
		t.Fatalf("import archive: %v", err)
	}
	patch, err := store.ReadPatch(context.Background(), receipt,
		turnchanges.CheckpointArchive{StartTreeOID: startTree, EndTreeOID: endTree},
		HistoricalFile{PathBytes: []byte("copy.txt"), OldPathBytes: []byte("source.txt")}, false)
	if err != nil {
		t.Fatalf("read copied file: %v", err)
	}
	if bytes.Contains(patch, []byte("diff --git a/source.txt b/source.txt")) {
		t.Fatalf("copy selection included the changed source patch: %q", patch)
	}
	if !bytes.Contains(patch, []byte("copy.txt")) || !bytes.Contains(patch, []byte("diff --git")) {
		t.Fatalf("copy selection = %q", patch)
	}
}

func TestTurnGitHistoryReadsUseAcceptedEndpointAttributes(t *testing.T) {
	for _, attributes := range []struct {
		name string
		body string
		text bool
	}{
		{name: "text", body: "*.dat diff\n", text: true},
		{name: "binary", body: "*.dat -diff\n", text: false},
	} {
		t.Run(attributes.name, func(t *testing.T) {
			repoDir := t.TempDir()
			runGitTest(t, repoDir, "init", "-q")
			runGitTest(t, repoDir, "config", "user.email", "turn@example.invalid")
			runGitTest(t, repoDir, "config", "user.name", "Turn Test")
			writeTurnArchiveFile(t, repoDir, ".gitattributes", attributes.body)
			writeTurnArchiveFile(t, repoDir, "data.dat", "before\n")
			runGitTest(t, repoDir, "add", ".")
			runGitTest(t, repoDir, "commit", "-qm", "base")
			startCommit := strings.TrimSpace(runGitTest(t, repoDir, "rev-parse", "HEAD"))
			writeTurnArchiveFile(t, repoDir, "data.dat", "after\n")
			runGitTest(t, repoDir, "add", ".")
			runGitTest(t, repoDir, "commit", "-qm", "turn")
			endCommit := strings.TrimSpace(runGitTest(t, repoDir, "rev-parse", "HEAD"))
			archive, startTree, endTree := archiveForCommits(t, repoDir, startCommit, endCommit)
			// Live working-tree attributes must not replace the accepted endpoint.
			writeTurnArchiveFile(t, repoDir, ".gitattributes", "*.dat -diff\n")
			store := NewGitHistoryStore(t.TempDir(), nil)
			receipt, err := store.Import(context.Background(), "repo-attr", "checkout-attr", archive)
			if err != nil {
				t.Fatalf("import archive: %v", err)
			}
			patch, err := store.ReadPatch(context.Background(), receipt,
				turnchanges.CheckpointArchive{HashAlgorithm: "sha1", StartTreeOID: startTree, EndTreeOID: endTree},
				HistoricalFile{PathBytes: []byte("data.dat")}, false)
			if err != nil {
				t.Fatalf("read attributed file: %v", err)
			}
			if attributes.text != bytes.Contains(patch, []byte("@@")) {
				t.Fatalf("attribute text=%t patch=%q", attributes.text, patch)
			}
		})
	}
}

func TestTurnGitHistoryRejectsOversizedReadAtTheStreamBound(t *testing.T) {
	repoDir := t.TempDir()
	runGitTest(t, repoDir, "init", "-q")
	runGitTest(t, repoDir, "config", "user.email", "turn@example.invalid")
	runGitTest(t, repoDir, "config", "user.name", "Turn Test")
	writeTurnArchiveFile(t, repoDir, "large.txt", strings.Repeat("a", 9<<20))
	runGitTest(t, repoDir, "add", ".")
	runGitTest(t, repoDir, "commit", "-qm", "base")
	startCommit := strings.TrimSpace(runGitTest(t, repoDir, "rev-parse", "HEAD"))
	writeTurnArchiveFile(t, repoDir, "large.txt", strings.Repeat("b", 9<<20))
	runGitTest(t, repoDir, "add", ".")
	runGitTest(t, repoDir, "commit", "-qm", "turn")
	endCommit := strings.TrimSpace(runGitTest(t, repoDir, "rev-parse", "HEAD"))
	archive, startTree, endTree := archiveForCommits(t, repoDir, startCommit, endCommit)
	store := NewGitHistoryStore(t.TempDir(), nil)
	receipt, err := store.Import(context.Background(), "repo-large", "checkout-large", archive)
	if err != nil {
		t.Fatalf("import large archive: %v", err)
	}
	largeOID := strings.TrimSpace(runGitTest(t, repoDir, "rev-parse", "HEAD:large.txt"))
	_, err = store.ReadBlob(context.Background(), receipt, largeOID, "sha1")
	if !errors.Is(err, ErrHistoryContentUnavailable) {
		t.Fatalf("oversized blob error = %v", err)
	}
	_, err = store.ReadPatch(context.Background(), receipt,
		turnchanges.CheckpointArchive{HashAlgorithm: "sha1", StartTreeOID: startTree, EndTreeOID: endTree},
		HistoricalFile{PathBytes: []byte("large.txt")}, false)
	if err == nil {
		t.Fatal("oversized patch unexpectedly returned content")
	}
}

func archiveForCommits(t *testing.T, repoDir, startCommit, endCommit string) (*turnchanges.CheckpointArchive, string, string) {
	t.Helper()
	startTree := strings.TrimSpace(runGitTest(t, repoDir, "rev-parse", startCommit+"^{tree}"))
	endTree := strings.TrimSpace(runGitTest(t, repoDir, "rev-parse", endCommit+"^{tree}"))
	objects := turnArchiveObjectList(t, repoDir, startTree, endTree, startCommit, endCommit)
	pack := runGitWithInput(t, repoDir, strings.Join(objects, "\n")+"\n", "pack-objects", "--stdout", "-q")
	return &turnchanges.CheckpointArchive{
		ChangeSetID: uuid.NewString(), CheckoutID: uuid.NewString(), HashAlgorithm: "sha1",
		StartCommitOID: startCommit, StartTreeOID: startTree, EndCommitOID: endCommit, EndTreeOID: endTree,
		Pack: pack, Objects: objects, ObjectCount: int64(len(objects)), Complete: true,
	}, startTree, endTree
}

func TestTurnGitHistoryReclaimsOwnedObjectsOnlyAfterAllRefsRelease(t *testing.T) {
	first, _, _, _ := buildTurnArchiveFixture(t)
	second, _, _, _ := buildTurnArchiveFixture(t)
	store := NewGitHistoryStore(t.TempDir(), nil)
	firstReceipt, err := store.Import(context.Background(), "repo-reclaim", "checkout-one", first)
	if err != nil {
		t.Fatalf("import first archive: %v", err)
	}
	secondReceipt, err := store.Import(context.Background(), "repo-reclaim", "checkout-two", second)
	if err != nil {
		t.Fatalf("import second archive: %v", err)
	}
	if err := store.DeleteRefs(context.Background(), firstReceipt); err != nil {
		t.Fatalf("delete first refs: %v", err)
	}
	freed, err := store.Reclaim(context.Background(), []string{firstReceipt.SourceID})
	if err != nil {
		t.Fatalf("reclaim with retained refs: %v", err)
	}
	if freed != 0 {
		t.Fatalf("reclaimed %d bytes while another ref remained", freed)
	}
	if err := store.VerifyRefs(context.Background(), secondReceipt); err != nil {
		t.Fatalf("retained second refs: %v", err)
	}
	if err := store.DeleteRefs(context.Background(), secondReceipt); err != nil {
		t.Fatalf("delete second refs: %v", err)
	}
	freed, err = store.Reclaim(context.Background(), []string{firstReceipt.SourceID})
	if err != nil {
		t.Fatalf("reclaim released objects: %v", err)
	}
	if freed <= 0 {
		t.Fatalf("released archive reclaimed %d bytes", freed)
	}
	if _, err := os.Stat(filepath.Join(store.repositoryPath(firstReceipt.SourceID), "objects", "pack")); err == nil {
		entries, _ := os.ReadDir(filepath.Join(store.repositoryPath(firstReceipt.SourceID), "objects", "pack"))
		if len(entries) != 0 {
			t.Fatalf("released archive retained %d pack entries", len(entries))
		}
	}
}

func TestTurnGitHistoryImportReusesKnownObjectsWithoutNewPacks(t *testing.T) {
	first, _, _, _ := buildTurnArchiveFixture(t)
	store := NewGitHistoryStore(t.TempDir(), nil)
	firstReceipt, err := store.Import(context.Background(), "repo-dedupe", "checkout-one", first)
	if err != nil {
		t.Fatalf("import first archive: %v", err)
	}
	known := store.SourceObjects(context.Background(), "repo-dedupe", "checkout-one")
	if len(known) == 0 {
		t.Fatal("source object inventory is empty")
	}
	second := *first
	second.ChangeSetID, second.CheckoutID = uuid.NewString(), uuid.NewString()
	second.Pack = nil
	if _, err := store.Import(context.Background(), "repo-dedupe", "checkout-two", &second); err != nil {
		t.Fatalf("import known-object archive: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(store.repositoryPath(firstReceipt.SourceID), "objects", "pack"))
	if err != nil {
		t.Fatal(err)
	}
	packs := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".pack") {
			packs++
		}
	}
	if packs != 1 {
		t.Fatalf("known-object import stored %d packs, want one", packs)
	}
}
