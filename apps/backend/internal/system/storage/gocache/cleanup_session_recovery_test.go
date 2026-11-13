package gocache

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// TestCleanupRestartsRecreatedShardBeforeResume reproduces the incident where a
// bounded pass retained a shard frame, the shard was recreated, and every
// remaining candidate failed its parent identity check on resume.
func TestCleanupRestartsRecreatedShardBeforeResume(t *testing.T) {
	provider, cachePath := newManagedCacheForCleanupTest(t, 5)
	shardPath := filepath.Join(cachePath, "00")
	files := []string{"one", "two", "three"}
	for _, name := range files {
		writeCleanupFixture(t, filepath.Join(shardPath, name), "xx")
	}

	first, err := provider.cleanupContentsWithLimit(
		context.Background(), cachePath, false, 5, 3, cacheFilesystemIdentity,
	)
	if err == nil || !first.Partial || !strings.Contains(strings.Join(first.Errors, ","), "entry_limit_reached") {
		t.Fatalf("first pass = (%#v, %v), want a retained entry-limit cursor", first, err)
	}

	backup := filepath.Join(t.TempDir(), "00")
	if err := os.Rename(shardPath, backup); err != nil {
		t.Fatalf("move retained shard: %v", err)
	}
	for _, name := range files {
		writeCleanupFixture(t, filepath.Join(shardPath, name), "xx")
	}

	second, err := provider.cleanupContentsWithLimit(
		context.Background(), cachePath, false, 5, cleanupEntryLimit, cacheFilesystemIdentity,
	)
	if err != nil || second.Partial {
		t.Fatalf("resumed pass = (%#v, %v), want a clean fresh-discovery pass", second, err)
	}
	if len(second.Errors) != 0 {
		t.Fatalf("resumed pass reported stale-scan errors: %#v", second.Errors)
	}
	if second.ReclaimedBytes != 6 {
		t.Fatalf("resumed pass reclaimed %d bytes, want 6", second.ReclaimedBytes)
	}
	if second.BytesAfter == nil || *second.BytesAfter != 0 {
		t.Fatalf("resumed pass measured %v remaining bytes, want 0", second.BytesAfter)
	}
	for _, name := range files {
		if _, err := os.Stat(filepath.Join(backup, name)); err != nil {
			t.Fatalf("retained shard file %s changed: %v", name, err)
		}
	}
}

// TestCleanupPreservesRecreatedShardBelowThreshold proves that stale threshold
// authority is discarded: the retained scan crossed the configured threshold,
// but fresh discovery of the smaller replacement must not delete it.
func TestCleanupPreservesRecreatedShardBelowThreshold(t *testing.T) {
	provider, cachePath := newManagedCacheForCleanupTest(t, 5)
	shardPath := filepath.Join(cachePath, "00")
	original := map[string]string{"one": "xx", "two": "xx", "three": "xx"}
	for name, contents := range original {
		writeCleanupFixture(t, filepath.Join(shardPath, name), contents)
	}

	first, err := provider.cleanupContentsWithLimit(
		context.Background(), cachePath, false, 5, 4, cacheFilesystemIdentity,
	)
	if err == nil || !first.Partial {
		t.Fatalf("first pass = (%#v, %v), want a retained entry-limit cursor", first, err)
	}
	if first.ReclaimedBytes != 0 {
		t.Fatalf("first pass reclaimed %d bytes without deletion budget", first.ReclaimedBytes)
	}
	session := provider.cleanupState
	if session == nil || session.phase != cleanupDelete {
		t.Fatalf("session = %#v, want an established above-threshold delete cursor", session)
	}
	if session.bytes != 6 {
		t.Fatalf("retained measurement = %d bytes, want 6", session.bytes)
	}

	backup := filepath.Join(t.TempDir(), "00")
	if err := os.Rename(shardPath, backup); err != nil {
		t.Fatalf("move retained shard: %v", err)
	}
	replacement := map[string]string{"one": "xx", "two": "x", "three": "x"}
	for name, contents := range replacement {
		writeCleanupFixture(t, filepath.Join(shardPath, name), contents)
	}

	second, err := provider.cleanupContentsWithLimit(
		context.Background(), cachePath, false, 5, cleanupEntryLimit, cacheFilesystemIdentity,
	)
	if err != nil || second.Partial || len(second.Errors) != 0 {
		t.Fatalf("resumed pass = (%#v, %v), want a clean pass below the threshold", second, err)
	}
	if second.BytesBefore != 4 {
		t.Fatalf("resumed pass measured %d bytes before cleanup, want fresh four-byte evidence", second.BytesBefore)
	}
	if second.BytesAfter == nil || *second.BytesAfter != 4 {
		t.Fatalf("resumed pass measured %v remaining bytes, want 4", second.BytesAfter)
	}
	if second.ReclaimedBytes != 0 {
		t.Fatalf("resumed pass reclaimed %d below-threshold bytes", second.ReclaimedBytes)
	}
	assertCleanupFixtures(t, shardPath, replacement)
	assertCleanupFixtures(t, backup, original)
}

// TestCleanupRestartsAfterReplacementFollowingPartialUnlink matches the
// incident: a retained pass had already unlinked eligible files, then the shard
// was recreated with below-threshold content.
func TestCleanupRestartsAfterReplacementFollowingPartialUnlink(t *testing.T) {
	provider, cachePath := newManagedCacheForCleanupTest(t, 5)
	shardPath := filepath.Join(cachePath, "00")
	original := map[string]string{
		"one": "xx", "two": "xx", "three": "xx", "four": "xx", "five": "xx",
	}
	for name, contents := range original {
		writeCleanupFixture(t, filepath.Join(shardPath, name), contents)
	}

	first, err := provider.cleanupContentsWithLimit(
		context.Background(), cachePath, false, 5, 6, cacheFilesystemIdentity,
	)
	if err == nil || !first.Partial {
		t.Fatalf("first pass = (%#v, %v), want a retained partial-unlink cursor", first, err)
	}
	if first.ReclaimedBytes != 4 {
		t.Fatalf("first pass reclaimed %d bytes, want 4", first.ReclaimedBytes)
	}
	if !strings.Contains(strings.Join(first.Errors, ","), "entry_limit_reached") {
		t.Fatalf("first pass errors = %#v, want entry_limit_reached", first.Errors)
	}
	session := provider.cleanupState
	if session == nil || session.phase != cleanupDelete || session.nextDelete != 2 {
		t.Fatalf("session = %#v, want a delete cursor after two unlinks", session)
	}
	if session.bytes != 6 || len(session.entries) != 3 {
		t.Fatalf("retained session = %#v, want six measured bytes over three candidates", session)
	}

	backup := filepath.Join(t.TempDir(), "00")
	if err := os.Rename(shardPath, backup); err != nil {
		t.Fatalf("move retained shard: %v", err)
	}
	replacement := map[string]string{
		"one": "x", "two": "x", "three": "x", "four": "x", "five": "x",
	}
	for name, contents := range replacement {
		writeCleanupFixture(t, filepath.Join(shardPath, name), contents)
	}

	second, err := provider.cleanupContentsWithLimit(
		context.Background(), cachePath, false, 5, cleanupEntryLimit, cacheFilesystemIdentity,
	)
	if err != nil || second.Partial || len(second.Errors) != 0 {
		t.Fatalf("resumed pass = (%#v, %v), want fresh below-threshold discovery", second, err)
	}
	if second.BytesBefore != 5 || second.BytesAfter == nil || *second.BytesAfter != 5 {
		t.Fatalf(
			"resumed pass = (%d before, %v after), want fresh five-byte evidence",
			second.BytesBefore, second.BytesAfter,
		)
	}
	if second.ReclaimedBytes != 0 {
		t.Fatalf("resumed pass reclaimed %d below-threshold bytes", second.ReclaimedBytes)
	}
	assertCleanupFixtures(t, shardPath, replacement)
	assertCleanupFixtureCount(t, backup, 3, "xx")
}

// TestCleanupFailsClosedOnRetainedMountReplacement keeps a retained directory
// on a different mount from restarting as ordinary cache churn.
func TestCleanupFailsClosedOnRetainedMountReplacement(t *testing.T) {
	provider, cachePath := newManagedCacheForCleanupTest(t, 5)
	shardPath := filepath.Join(cachePath, "00")
	files := map[string]string{"one": "xx", "two": "xx", "three": "xx"}
	for name, contents := range files {
		writeCleanupFixture(t, filepath.Join(shardPath, name), contents)
	}
	var replacementMount atomic.Bool
	identity := func(path string) (string, error) {
		value, err := cacheFilesystemIdentity(path)
		if err != nil {
			return "", err
		}
		if replacementMount.Load() && filepath.Base(path) == "00" {
			return value + "\x00other-mount", nil
		}
		return value, nil
	}

	first, err := provider.cleanupContentsWithLimit(
		context.Background(), cachePath, false, 5, 3, identity,
	)
	if err == nil || !first.Partial {
		t.Fatalf("first pass = (%#v, %v), want a retained entry-limit cursor", first, err)
	}

	backup := filepath.Join(t.TempDir(), "00")
	if err := os.Rename(shardPath, backup); err != nil {
		t.Fatalf("move retained shard: %v", err)
	}
	for name, contents := range files {
		writeCleanupFixture(t, filepath.Join(shardPath, name), contents)
	}
	replacementMount.Store(true)

	second, err := provider.cleanupContentsWithLimit(
		context.Background(), cachePath, false, 5, cleanupEntryLimit, identity,
	)
	if err == nil || !second.Partial {
		t.Fatalf("mount replacement = (%#v, %v), want a closed safety failure", second, err)
	}
	if !strings.Contains(strings.Join(second.Errors, ","), "cache_root_changed") {
		t.Fatalf("mount replacement errors = %#v, want cache_root_changed", second.Errors)
	}
	assertCleanupFixtures(t, shardPath, files)
	assertCleanupFixtures(t, backup, files)
}

// TestCleanupRetiresRemainderEntryInvalidation covers invalidation detected
// while deleting the remainder of a scan, not a discovered candidate.
func TestCleanupRetiresRemainderEntryInvalidation(t *testing.T) {
	provider, cachePath := newManagedCacheForCleanupTest(t, 100)
	shardPath := filepath.Join(cachePath, "00")
	writeCleanupFixture(t, filepath.Join(shardPath, "one"), "xx")
	writeCleanupFixture(t, filepath.Join(shardPath, "two"), "xx")

	rootHandle, err := provider.openValidatedCacheRoot(cachePath, false, cacheFilesystemIdentity)
	if err != nil {
		t.Fatalf("open cache root: %v", err)
	}
	session, err := newCleanupSessionWithRoot(
		cachePath, false, 100, "remainder-invalidation-test", cacheFilesystemIdentity, rootHandle,
	)
	if err != nil {
		t.Fatalf("open cleanup session: %v", err)
	}
	provider.cleanupState = session
	if _, err := session.discover(context.Background(), 1); err != nil {
		t.Fatalf("discover: %v", err)
	}
	if session.directories["00"] == nil {
		t.Fatalf("session = %#v, want a retained shard frame", session)
	}
	session.entries = nil
	session.nextDelete = 0

	backup := filepath.Join(t.TempDir(), "00")
	if err := os.Rename(shardPath, backup); err != nil {
		t.Fatalf("move retained shard: %v", err)
	}
	writeCleanupFixture(t, filepath.Join(shardPath, "one"), "new")
	writeCleanupFixture(t, filepath.Join(shardPath, "two"), "new")

	result, err := provider.deleteCleanupPass(
		context.Background(), session, cachePath,
		&cleanupEntryBudget{remaining: cleanupEntryLimit}, CleanupResult{}, nil,
	)
	if err == nil || !result.Partial {
		t.Fatalf("remainder invalidation = (%#v, %v), want a partial result", result, err)
	}
	if result.Reason != "cache_scan_invalidated" {
		t.Fatalf("remainder invalidation reason = %q, want cache_scan_invalidated", result.Reason)
	}
	if len(result.Errors) != 1 || result.Errors[0] != "cache_parent_changed" {
		t.Fatalf("remainder invalidation errors = %#v, want one cache_parent_changed issue", result.Errors)
	}
	if provider.cleanupState != nil {
		t.Fatal("remainder invalidation retained its stale cursor")
	}
}

// TestCleanupRetiresDisappearedParentInvalidation covers a retained parent
// directory removed between passes while a discovered candidate remains.
func TestCleanupRetiresDisappearedParentInvalidation(t *testing.T) {
	provider, cachePath := newManagedCacheForCleanupTest(t, 1)
	shardPath := filepath.Join(cachePath, "00")
	writeCleanupFixture(t, filepath.Join(shardPath, "one"), "xx")
	writeCleanupFixture(t, filepath.Join(shardPath, "two"), "xx")

	rootHandle, err := provider.openValidatedCacheRoot(cachePath, false, cacheFilesystemIdentity)
	if err != nil {
		t.Fatalf("open cache root: %v", err)
	}
	session, err := newCleanupSessionWithRoot(
		cachePath, false, 1, "disappeared-parent-test", cacheFilesystemIdentity, rootHandle,
	)
	if err != nil {
		t.Fatalf("open cleanup session: %v", err)
	}
	provider.cleanupState = session
	if _, err := session.discover(context.Background(), cleanupEntryLimit); err != nil {
		t.Fatalf("discover: %v", err)
	}
	if session.phase != cleanupDelete || len(session.entries) == 0 {
		t.Fatalf("session = %#v, want a retained delete candidate", session)
	}
	if err := os.RemoveAll(shardPath); err != nil {
		t.Fatalf("remove retained shard: %v", err)
	}

	result, err := provider.deleteCleanupPass(
		context.Background(), session, cachePath,
		&cleanupEntryBudget{remaining: cleanupEntryLimit}, CleanupResult{}, nil,
	)
	if err == nil || !result.Partial {
		t.Fatalf("disappeared parent = (%#v, %v), want a partial result", result, err)
	}
	if result.Reason != "cache_scan_invalidated" {
		t.Fatalf("disappeared parent reason = %q, want cache_scan_invalidated", result.Reason)
	}
	if len(result.Errors) != 1 || result.Errors[0] != "cache_parent_changed" {
		t.Fatalf("disappeared parent errors = %#v, want one cache_parent_changed issue", result.Errors)
	}
	if provider.cleanupState != nil {
		t.Fatal("disappeared parent retained its stale cursor")
	}
}

// TestCleanupRetiresDirectoryRemovalInvalidation covers invalidation detected
// while removing a nested directory frame.
func TestCleanupRetiresDirectoryRemovalInvalidation(t *testing.T) {
	provider, cachePath := newManagedCacheForCleanupTest(t, 1)
	shardPath := filepath.Join(cachePath, "00")
	nestedPath := filepath.Join(shardPath, "sub")
	writeCleanupFixture(t, filepath.Join(nestedPath, "artifact"), "xx")

	rootHandle, err := provider.openValidatedCacheRoot(cachePath, false, cacheFilesystemIdentity)
	if err != nil {
		t.Fatalf("open cache root: %v", err)
	}
	session, err := newCleanupSessionWithRoot(
		cachePath, false, 1, "directory-invalidation-test", cacheFilesystemIdentity, rootHandle,
	)
	if err != nil {
		t.Fatalf("open cleanup session: %v", err)
	}
	provider.cleanupState = session
	if _, err := session.discover(context.Background(), 3); err != nil {
		t.Fatalf("discover: %v", err)
	}
	if session.phase != cleanupDelete || len(session.stack) != 3 {
		t.Fatalf("session = %#v, want a retained nested delete frame", session)
	}
	session.entries = nil
	session.nextDelete = 0

	backup := filepath.Join(t.TempDir(), "00")
	if err := os.Rename(shardPath, backup); err != nil {
		t.Fatalf("move retained shard: %v", err)
	}
	writeCleanupFixture(t, filepath.Join(nestedPath, "artifact"), "new")

	result, err := provider.deleteCleanupPass(
		context.Background(), session, cachePath,
		&cleanupEntryBudget{remaining: cleanupEntryLimit}, CleanupResult{}, nil,
	)
	if err == nil || !result.Partial {
		t.Fatalf("directory invalidation = (%#v, %v), want a partial result", result, err)
	}
	if result.Reason != "cache_scan_invalidated" {
		t.Fatalf("directory invalidation reason = %q, want cache_scan_invalidated", result.Reason)
	}
	if len(result.Errors) != 1 || result.Errors[0] != "cache_parent_changed" {
		t.Fatalf("directory invalidation errors = %#v, want one cache_parent_changed issue", result.Errors)
	}
	if provider.cleanupState != nil {
		t.Fatal("directory invalidation retained its stale cursor")
	}
}

// TestCleanupRetainsInvalidationIssueWhenSamplesFull keeps the stable issue
// when the bounded sample is already full.
func TestCleanupRetainsInvalidationIssueWhenSamplesFull(t *testing.T) {
	session := &cleanupSession{}
	for index := 0; index < cleanupIssueLimit; index++ {
		session.issues = append(session.issues, "entry_delete_failed")
	}
	session.retireInvalidatedScan()

	if !session.scanInvalidated {
		t.Fatal("invalidation was not recorded")
	}
	if len(session.issues) != cleanupIssueLimit {
		t.Fatalf("issues = %#v, want a bounded sample", session.issues)
	}
	count := 0
	for _, issue := range session.issues {
		if issue == "cache_parent_changed" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("issues = %#v, want one cache_parent_changed marker", session.issues)
	}
}

func assertCleanupFixtureCount(t *testing.T, root string, wantCount int, wantContents string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read fixture directory: %v", err)
	}
	if len(entries) != wantCount {
		t.Fatalf("fixture directory has %d entries, want %d", len(entries), wantCount)
	}
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(root, entry.Name()))
		if err != nil || string(data) != wantContents {
			t.Fatalf("fixture %s changed: data=%q err=%v", entry.Name(), data, err)
		}
	}
}

func assertCleanupFixtures(t *testing.T, root string, fixtures map[string]string) {
	t.Helper()
	for name, want := range fixtures {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(data) != want {
			t.Fatalf("fixture %s changed: data=%q err=%v", name, data, err)
		}
	}
}

// TestCleanupRetiresInvalidatedScanDuringRemoval covers a shard replaced after
// the pass was admitted. Removal must stop at the first identity mismatch, keep
// successful unlink accounting, report one stable issue, and drop the cursor.
func TestCleanupRetiresInvalidatedScanDuringRemoval(t *testing.T) {
	provider, cachePath := newManagedCacheForCleanupTest(t, 1)
	shardPath := filepath.Join(cachePath, "00")
	for _, name := range []string{"one", "two"} {
		writeCleanupFixture(t, filepath.Join(shardPath, name), "xx")
	}
	rootHandle, err := provider.openValidatedCacheRoot(cachePath, false, cacheFilesystemIdentity)
	if err != nil {
		t.Fatalf("open cache root: %v", err)
	}
	session, err := newCleanupSessionWithRoot(
		cachePath, false, 1, "removal-invalidation-test", cacheFilesystemIdentity, rootHandle,
	)
	if err != nil {
		t.Fatalf("open cleanup session: %v", err)
	}
	provider.cleanupState = session
	if _, err := session.discover(context.Background(), cleanupEntryLimit); err != nil {
		t.Fatalf("discover: %v", err)
	}
	if session.directories["00"] == nil || session.phase != cleanupDelete {
		t.Fatalf("session = %#v, want a retained shard cursor in the delete phase", session)
	}
	// The root artifact is discovered later than the shard, so add its planned
	// candidate explicitly. Successful removal through the root must survive the
	// later shard invalidation.
	rootArtifact := filepath.Join(cachePath, "root-artifact")
	writeCleanupFixture(t, rootArtifact, "rootbytes")
	artifactInfo, err := os.Stat(rootArtifact)
	if err != nil {
		t.Fatalf("stat root artifact: %v", err)
	}
	session.entries = append(
		[]cleanupEntry{{relative: "root-artifact", info: artifactInfo, bytes: int64(len("rootbytes"))}},
		session.entries...,
	)

	backup := filepath.Join(t.TempDir(), "00")
	if err := os.Rename(shardPath, backup); err != nil {
		t.Fatalf("move retained shard: %v", err)
	}
	writeCleanupFixture(t, filepath.Join(shardPath, "one"), "new")
	writeCleanupFixture(t, filepath.Join(shardPath, "two"), "new")

	result, err := provider.deleteCleanupPass(
		context.Background(), session, cachePath,
		&cleanupEntryBudget{remaining: cleanupEntryLimit}, CleanupResult{}, nil,
	)
	if err == nil || !result.Partial {
		t.Fatalf("invalidated pass = (%#v, %v), want a partial result", result, err)
	}
	if result.Reason != "cache_scan_invalidated" {
		t.Fatalf("invalidated pass reason = %q, want cache_scan_invalidated", result.Reason)
	}
	if len(result.Errors) != 1 || result.Errors[0] != "cache_parent_changed" {
		t.Fatalf("invalidated pass errors = %#v, want one cache_parent_changed issue", result.Errors)
	}
	if result.ReclaimedBytes != int64(len("rootbytes")) {
		t.Fatalf("invalidated pass reclaimed %d bytes, want %d", result.ReclaimedBytes, len("rootbytes"))
	}
	if provider.cleanupState != nil {
		t.Fatal("invalidated pass retained its stale cursor")
	}
	if data, err := os.ReadFile(filepath.Join(shardPath, "one")); err != nil || string(data) != "new" {
		t.Fatalf("replacement shard changed during invalidated removal: data=%q err=%v", data, err)
	}

	// The retired cursor must not replay the invalidated scan. The replacement
	// contains zero-byte entries, so a fresh pass draws no deletion authority.
	for _, name := range []string{"one", "two"} {
		if err := os.Remove(filepath.Join(shardPath, name)); err != nil {
			t.Fatalf("remove replacement file: %v", err)
		}
		writeCleanupFixture(t, filepath.Join(shardPath, name), "")
	}
	fresh, err := provider.Cleanup(context.Background())
	if err != nil || fresh.Partial || len(fresh.Errors) != 0 {
		t.Fatalf("fresh pass = (%#v, %v), want a clean below-threshold pass", fresh, err)
	}
	if fresh.ReclaimedBytes != 0 {
		t.Fatalf("fresh pass reclaimed %d bytes below the threshold", fresh.ReclaimedBytes)
	}
	for _, name := range []string{"one", "two"} {
		if _, err := os.Stat(filepath.Join(shardPath, name)); err != nil {
			t.Fatalf("below-threshold replacement file %s was removed: %v", name, err)
		}
	}
}

// TestCleanupCancelsRetainedValidationBeforeResume keeps a canceled invocation
// from restarting or deleting through retained frames.
func TestCleanupCancelsRetainedValidationBeforeResume(t *testing.T) {
	provider, cachePath := newManagedCacheForCleanupTest(t, 5)
	shardPath := filepath.Join(cachePath, "00")
	for _, name := range []string{"one", "two", "three"} {
		writeCleanupFixture(t, filepath.Join(shardPath, name), "xx")
	}

	first, err := provider.cleanupContentsWithLimit(
		context.Background(), cachePath, false, 5, 3, cacheFilesystemIdentity,
	)
	if err == nil || !first.Partial {
		t.Fatalf("first pass = (%#v, %v), want a retained entry-limit cursor", first, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	second, err := provider.cleanupContentsWithLimit(
		ctx, cachePath, false, 5, cleanupEntryLimit, cacheFilesystemIdentity,
	)
	if err == nil || !second.Partial {
		t.Fatalf("canceled resume = (%#v, %v), want a canceled result", second, err)
	}
	if !strings.Contains(strings.Join(second.Errors, ","), "cleanup_cancelled") {
		t.Fatalf("canceled resume errors = %#v, want cleanup_cancelled", second.Errors)
	}
	if provider.cleanupState != nil {
		t.Fatal("canceled resume retained its traversal cursor")
	}
	for _, name := range []string{"one", "two", "three"} {
		if _, err := os.Stat(filepath.Join(shardPath, name)); err != nil {
			t.Fatalf("canceled resume changed %s: %v", name, err)
		}
	}
}

// TestCleanupFailsClosedOnRetainedShardSymlink ensures an ordinary replacement
// restart never accepts a symlinked retained directory.
func TestCleanupFailsClosedOnRetainedShardSymlink(t *testing.T) {
	provider, cachePath := newManagedCacheForCleanupTest(t, 5)
	shardPath := filepath.Join(cachePath, "00")
	for _, name := range []string{"one", "two", "three"} {
		writeCleanupFixture(t, filepath.Join(shardPath, name), "xx")
	}

	first, err := provider.cleanupContentsWithLimit(
		context.Background(), cachePath, false, 5, 3, cacheFilesystemIdentity,
	)
	if err == nil || !first.Partial {
		t.Fatalf("first pass = (%#v, %v), want a retained entry-limit cursor", first, err)
	}

	external := t.TempDir()
	externalArtifact := filepath.Join(external, "external")
	writeCleanupFixture(t, externalArtifact, "external bytes")
	backup := filepath.Join(t.TempDir(), "00")
	if err := os.Rename(shardPath, backup); err != nil {
		t.Fatalf("move retained shard: %v", err)
	}
	if err := os.Symlink(external, shardPath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	second, err := provider.cleanupContentsWithLimit(
		context.Background(), cachePath, false, 5, cleanupEntryLimit, cacheFilesystemIdentity,
	)
	if err == nil || !second.Partial {
		t.Fatalf("symlinked replacement = (%#v, %v), want a closed safety failure", second, err)
	}
	if !strings.Contains(strings.Join(second.Errors, ","), "cache_root_changed") {
		t.Fatalf("symlinked replacement errors = %#v, want cache_root_changed", second.Errors)
	}
	if data, err := os.ReadFile(externalArtifact); err != nil || string(data) != "external bytes" {
		t.Fatalf("symlink target changed: data=%q err=%v", data, err)
	}
}

// TestCleanupFailsClosedWhenRetainedMountIdentityUnavailable keeps a valid
// directory but removes mount evidence between passes.
func TestCleanupFailsClosedWhenRetainedMountIdentityUnavailable(t *testing.T) {
	provider, cachePath := newManagedCacheForCleanupTest(t, 5)
	shardPath := filepath.Join(cachePath, "00")
	for _, name := range []string{"one", "two", "three"} {
		writeCleanupFixture(t, filepath.Join(shardPath, name), "xx")
	}

	var failMount atomic.Bool
	identity := func(path string) (string, error) {
		if failMount.Load() && filepath.Base(path) == "00" {
			return "", errors.New("mount identity unavailable")
		}
		return cacheFilesystemIdentity(path)
	}

	first, err := provider.cleanupContentsWithLimit(
		context.Background(), cachePath, false, 5, 3, identity,
	)
	if err == nil || !first.Partial {
		t.Fatalf("first pass = (%#v, %v), want a retained entry-limit cursor", first, err)
	}

	failMount.Store(true)
	second, err := provider.cleanupContentsWithLimit(
		context.Background(), cachePath, false, 5, cleanupEntryLimit, identity,
	)
	if err == nil || !second.Partial {
		t.Fatalf("unavailable mount identity = (%#v, %v), want a closed safety failure", second, err)
	}
	if !strings.Contains(strings.Join(second.Errors, ","), "cache_root_changed") {
		t.Fatalf("unavailable mount identity errors = %#v, want cache_root_changed", second.Errors)
	}
	for _, name := range []string{"one", "two", "three"} {
		if _, err := os.Stat(filepath.Join(shardPath, name)); err != nil {
			t.Fatalf("retained shard file %s changed: %v", name, err)
		}
	}
}
