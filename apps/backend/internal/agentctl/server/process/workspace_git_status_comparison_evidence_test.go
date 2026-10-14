package process

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types"
)

// comparisonEvidenceFixture is a repository whose base and ahead/behind refs
// resolve to origin/main, with distinct remote-head and comparison refs.
type comparisonEvidenceFixture struct {
	tracker   *WorkspaceTracker
	tracePath string
	mainOID   string
	otherOID  string
}

func newComparisonEvidenceFixture(t *testing.T) comparisonEvidenceFixture {
	t.Helper()
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	mainOID := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	writeFile(t, repoDir, "feature.txt", "feature\n")
	runGit(t, repoDir, "add", "feature.txt")
	runGit(t, repoDir, "commit", "-m", "Feature commit")
	otherOID := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	runGit(t, repoDir, "update-ref", "refs/remotes/origin/feature", otherOID)
	runGit(t, repoDir, "update-ref", "refs/remotes/origin/develop", mainOID)

	tracePath := filepath.Join(t.TempDir(), "git.trace")
	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	tracker.SetGitEnvironment([]string{"GIT_TRACE=" + tracePath})
	t.Cleanup(tracker.Stop)
	return comparisonEvidenceFixture{tracker: tracker, tracePath: tracePath, mainOID: mainOID, otherOID: otherOID}
}

// commitResolutions returns the traced rev-parse invocations that resolve a
// ref to a commit object, which is how comparison evidence reads a ref.
func (f comparisonEvidenceFixture) commitResolutions(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(f.tracePath)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read Git trace: %v", err)
	}
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, "rev-parse") && strings.Contains(line, "^{commit}") {
			lines = append(lines, line)
		}
	}
	return lines
}

func (f comparisonEvidenceFixture) resetTrace(t *testing.T) {
	t.Helper()
	if err := os.Remove(f.tracePath); err != nil && !os.IsNotExist(err) {
		t.Fatalf("reset Git trace: %v", err)
	}
}

func (f comparisonEvidenceFixture) capture(t *testing.T, ctx context.Context, remoteBranch, comparisonRef string) gitStatusComparisonEvidence {
	t.Helper()
	status := types.GitStatusUpdate{RemoteBranch: remoteBranch}
	evidence, err := f.tracker.captureGitStatusComparisonEvidence(ctx, status, ComparisonResolution{Ref: comparisonRef})
	if err != nil {
		t.Fatalf("captureGitStatusComparisonEvidence() error = %v", err)
	}
	return evidence
}

func TestGitStatusComparisonEvidenceResolvesRefsInOneProcess(t *testing.T) {
	fixture := newComparisonEvidenceFixture(t)
	fixture.resetTrace(t)

	evidence := fixture.capture(t, context.Background(), "origin/feature", "origin/develop")

	want := gitStatusComparisonEvidence{
		baseRef: "origin/main", baseOID: fixture.mainOID,
		aheadRef: "origin/main", aheadOID: fixture.mainOID,
		remoteHeadOID: fixture.otherOID, comparisonOID: fixture.mainOID,
	}
	if evidence != want {
		t.Fatalf("evidence = %+v, want %+v", evidence, want)
	}
	if got := fixture.commitResolutions(t); len(got) != 1 {
		t.Fatalf("commit resolutions = %d (%q), want one batched rev-parse", len(got), got)
	}
}

func TestGitStatusComparisonEvidenceMissingRefMatchesSeparateResolution(t *testing.T) {
	fixture := newComparisonEvidenceFixture(t)

	evidence := fixture.capture(t, context.Background(), "origin/missing", "origin/develop")

	if evidence.remoteHeadOID != "" {
		t.Fatalf("missing remote head OID = %q, want empty", evidence.remoteHeadOID)
	}
	if evidence.baseOID != fixture.mainOID || evidence.aheadOID != fixture.mainOID || evidence.comparisonOID != fixture.mainOID {
		t.Fatalf("evidence = %+v, want resolvable refs at %s", evidence, fixture.mainOID)
	}
}

func TestGitStatusComparisonEvidenceKeepsUnsafeAndUnknownObjectRefsEmpty(t *testing.T) {
	fixture := newComparisonEvidenceFixture(t)
	fixture.resetTrace(t)
	unknownObject := strings.Repeat("0", 40)

	evidence := fixture.capture(t, context.Background(), "origin/bad..ref", unknownObject)

	if evidence.remoteHeadOID != "" || evidence.comparisonOID != "" {
		t.Fatalf("evidence = %+v, want unsafe and unknown refs empty", evidence)
	}
	if evidence.baseOID != fixture.mainOID || evidence.aheadOID != fixture.mainOID {
		t.Fatalf("evidence = %+v, want base refs at %s", evidence, fixture.mainOID)
	}
	for _, line := range fixture.commitResolutions(t) {
		if strings.Contains(line, "bad..ref") {
			t.Fatalf("unsafe ref reached Git: %s", line)
		}
	}
}

func TestGitStatusComparisonEvidenceCancelledContextYieldsEmptyOIDs(t *testing.T) {
	fixture := newComparisonEvidenceFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	evidence := fixture.capture(t, ctx, "origin/feature", "origin/develop")

	if evidence.baseOID != "" || evidence.aheadOID != "" || evidence.remoteHeadOID != "" || evidence.comparisonOID != "" {
		t.Fatalf("evidence = %+v, want empty OIDs for a cancelled read", evidence)
	}
}
func TestBatchedRefOIDsRejectsOutputThatDoesNotMapToObjectIDs(t *testing.T) {
	oid := strings.Repeat("a", 40)
	cases := []struct {
		name   string
		output string
		count  int
		ok     bool
	}{
		{name: "one object ID per ref", output: oid + "\n" + oid + "\r\n", count: 2, ok: true},
		{name: "carriage return on every line", output: oid + "\r\n" + oid + "\r\n", count: 2, ok: true},
		{name: "truncated", output: oid + "\n", count: 2},
		{name: "extra line", output: oid + "\n" + oid + "\n" + oid + "\n", count: 2},
		{name: "echoed non-object line", output: oid + "\norigin/main^{commit}\n", count: 2},
		{name: "extra blank record", output: oid + "\n\n", count: 1},
		{name: "extra blank CRLF record", output: oid + "\r\n\r\n", count: 1},
		{name: "sha256", output: strings.Repeat("b", 64) + "\r\n", count: 1, ok: true},
		{name: "abbreviated", output: strings.Repeat("a", 39) + "\n", count: 1},
		{name: "unterminated", output: oid, count: 1, ok: true},
		{name: "empty", output: "", count: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			oids, ok := batchedRefOIDs([]byte(tc.output), tc.count)
			if ok != tc.ok {
				t.Fatalf("batchedRefOIDs() ok = %v, want %v (oids %q)", ok, tc.ok, oids)
			}
			if ok && len(oids) != tc.count {
				t.Fatalf("batchedRefOIDs() = %q, want %d object IDs", oids, tc.count)
			}
		})
	}
}
