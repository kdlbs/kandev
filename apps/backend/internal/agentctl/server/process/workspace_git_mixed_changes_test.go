package process

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types"
)

func TestEnrichMixedUnstagedDiffsSkipsGitWithoutMixedFacets(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	tracePath := filepath.Join(t.TempDir(), "git.trace")
	t.Setenv("GIT_TRACE", tracePath)
	update := &types.GitStatusUpdate{Files: map[string]types.FileInfo{
		"plain.txt": {Path: "plain.txt", Status: fileStatusModified},
	}}
	budget := diffBudget{}

	if err := tracker.enrichMixedUnstagedDiffsBudget(
		context.Background(), update, types.GitStatusUpdate{}, &budget,
	); err != nil {
		t.Fatalf("enrichMixedUnstagedDiffsBudget() error = %v", err)
	}
	if _, err := os.Stat(tracePath); !os.IsNotExist(err) {
		t.Fatalf("git trace exists after non-mixed update: %v", err)
	}
}

type mixedChangeFacetWire struct {
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Diff      string `json:"diff"`
}

// TestWorkspaceGitStatusPreservesMixedChangeFacets covers
// AC-PLATFORM-WORKSPACE-GIT-STATUS-001.9.
func TestWorkspaceGitStatusPreservesMixedChangeFacets(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	const filePath = "mixed-change.txt"
	writeFile(t, repoDir, filePath, "base\n")
	runGit(t, repoDir, "add", filePath)
	runGit(t, repoDir, "commit", "-m", "Add mixed change fixture")

	writeFile(t, repoDir, filePath, "base\nSTAGED_LAYER_MARKER\n")
	runGit(t, repoDir, "add", filePath)
	writeFile(t, repoDir, filePath, "base\nSTAGED_LAYER_MARKER\nUNSTAGED_LAYER_MARKER\n")

	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	status, err := tracker.getGitStatus(context.Background())
	if err != nil {
		t.Fatalf("getGitStatus() error = %v", err)
	}

	file, ok := status.Files[filePath]
	if !ok {
		t.Fatalf("status.Files missing %q", filePath)
	}
	wire, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("marshal FileInfo: %v", err)
	}

	var encoded map[string]json.RawMessage
	if err := json.Unmarshal(wire, &encoded); err != nil {
		t.Fatalf("unmarshal FileInfo wire shape: %v", err)
	}
	staged := decodeMixedChangeFacet(t, encoded, "staged_change")
	unstaged := decodeMixedChangeFacet(t, encoded, "unstaged_change")

	if staged.Status != fileStatusModified || staged.Additions != 1 || staged.Deletions != 0 {
		t.Errorf("staged facet = %+v, want modified +1 -0", staged)
	}
	if !strings.Contains(staged.Diff, "STAGED_LAYER_MARKER") {
		t.Errorf("staged diff missing staged marker: %q", staged.Diff)
	}
	if strings.Contains(staged.Diff, "UNSTAGED_LAYER_MARKER") {
		t.Errorf("staged diff contains unstaged marker: %q", staged.Diff)
	}
	if unstaged.Status != fileStatusModified || unstaged.Additions != 1 || unstaged.Deletions != 0 {
		t.Errorf("unstaged facet = %+v, want modified +1 -0", unstaged)
	}
	if !strings.Contains(unstaged.Diff, "UNSTAGED_LAYER_MARKER") {
		t.Errorf("unstaged diff missing unstaged marker: %q", unstaged.Diff)
	}
}

// TestWorkspaceGitStatusPreservesAddedMixedChangeFacets covers
// AC-PLATFORM-WORKSPACE-GIT-STATUS-001.9 for an AM path.
func TestWorkspaceGitStatusPreservesAddedMixedChangeFacets(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	const filePath = "mixed-added.txt"
	writeFile(t, repoDir, filePath, "STAGED_ADDED_MARKER\n")
	runGit(t, repoDir, "add", filePath)
	writeFile(t, repoDir, filePath, "STAGED_ADDED_MARKER\nUNSTAGED_ADDED_MARKER\n")

	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	status, err := tracker.getGitStatus(context.Background())
	if err != nil {
		t.Fatalf("getGitStatus() error = %v", err)
	}

	file, ok := status.Files[filePath]
	if !ok {
		t.Fatalf("status.Files missing %q", filePath)
	}
	wire, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("marshal FileInfo: %v", err)
	}
	var encoded map[string]json.RawMessage
	if err := json.Unmarshal(wire, &encoded); err != nil {
		t.Fatalf("unmarshal FileInfo wire shape: %v", err)
	}

	staged := decodeMixedChangeFacet(t, encoded, "staged_change")
	unstaged := decodeMixedChangeFacet(t, encoded, "unstaged_change")
	if staged.Status != "added" || staged.Additions != 1 || staged.Deletions != 0 {
		t.Errorf("staged facet = %+v, want added +1 -0", staged)
	}
	if strings.Contains(staged.Diff, "UNSTAGED_ADDED_MARKER") {
		t.Errorf("staged diff contains unstaged marker: %q", staged.Diff)
	}
	if unstaged.Status != fileStatusModified || unstaged.Additions != 1 || unstaged.Deletions != 0 {
		t.Errorf("unstaged facet = %+v, want modified +1 -0", unstaged)
	}
}

// TestMixedChangeFacetDiffBudgetCountsAllRepresentations covers
// AC-PLATFORM-WORKSPACE-GIT-STATUS-001.7.
func TestMixedChangeFacetDiffBudgetCountsAllRepresentations(t *testing.T) {
	file := fileInfoFromWire(t, `{
		"path":"mixed.txt",
		"status":"modified",
		"staged":false,
		"diff":"base",
		"staged_change":{"status":"modified","diff":"staged"},
		"unstaged_change":{"status":"modified","diff":"unstaged"}
	}`)
	update := &types.GitStatusUpdate{Files: map[string]types.FileInfo{"mixed.txt": file}}

	total, err := totalDiffBytes(context.Background(), update)
	if err != nil {
		t.Fatalf("totalDiffBytes() error = %v", err)
	}
	if total != int64(len("base")+len("staged")+len("unstaged")) {
		t.Fatalf("totalDiffBytes() = %d, want all three representations", total)
	}
}

// TestCarryForwardMixedChangeFacets covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.9.
func TestCarryForwardMixedChangeFacets(t *testing.T) {
	current := fileInfoFromWire(t, `{
		"path":"mixed.txt",
		"status":"modified",
		"staged":false,
		"diff":"fresh combined",
		"staged_change":{"status":"modified"},
		"unstaged_change":{"status":"modified"}
	}`)
	priorFile := fileInfoFromWire(t, `{
		"path":"mixed.txt",
		"status":"modified",
		"staged":false,
		"diff":"old combined",
		"staged_change":{"status":"modified","additions":1,"diff":"cached staged"},
		"unstaged_change":{"status":"modified","additions":1,"diff":"cached unstaged"}
	}`)
	update := &types.GitStatusUpdate{
		HeadCommit: "same-head",
		Files:      map[string]types.FileInfo{"mixed.txt": current},
	}
	prior := types.GitStatusUpdate{
		HeadCommit: "same-head",
		Files:      map[string]types.FileInfo{"mixed.txt": priorFile},
	}

	if err := carryForwardFileDiffs(context.Background(), update, prior); err != nil {
		t.Fatalf("carryForwardFileDiffs() error = %v", err)
	}
	wire, err := json.Marshal(update.Files["mixed.txt"])
	if err != nil {
		t.Fatalf("marshal carried FileInfo: %v", err)
	}
	var encoded map[string]json.RawMessage
	if err := json.Unmarshal(wire, &encoded); err != nil {
		t.Fatalf("unmarshal carried FileInfo: %v", err)
	}
	if got := decodeMixedChangeFacet(t, encoded, "staged_change").Diff; got != "cached staged" {
		t.Errorf("staged carried diff = %q, want cached staged", got)
	}
	if got := decodeMixedChangeFacet(t, encoded, "unstaged_change").Diff; got != "cached unstaged" {
		t.Errorf("unstaged carried diff = %q, want cached unstaged", got)
	}
}

func TestCarryForwardMixedChangeFacetsRespectsBudget(t *testing.T) {
	update := &types.GitStatusUpdate{
		HeadCommit: "same-head",
		Files: map[string]types.FileInfo{
			"mixed.txt": {
				Path:           "mixed.txt",
				Status:         fileStatusModified,
				Diff:           strings.Repeat("x", maxTotalDiffBytes),
				StagedChange:   &types.FileChangeFacet{Status: fileStatusModified},
				UnstagedChange: &types.FileChangeFacet{Status: fileStatusModified},
			},
		},
	}
	prior := types.GitStatusUpdate{
		HeadCommit: "same-head",
		Files: map[string]types.FileInfo{
			"mixed.txt": {
				Path:           "mixed.txt",
				Status:         fileStatusModified,
				StagedChange:   &types.FileChangeFacet{Status: fileStatusModified, Diff: "cached staged"},
				UnstagedChange: &types.FileChangeFacet{Status: fileStatusModified, Diff: "cached unstaged"},
			},
		},
	}

	if err := carryForwardFileDiffs(context.Background(), update, prior); err != nil {
		t.Fatalf("carryForwardFileDiffs() error = %v", err)
	}
	file := update.Files["mixed.txt"]
	if file.StagedChange.Diff != "" || file.StagedChange.DiffSkipReason != diffSkipReasonBudgetExceeded {
		t.Errorf("staged facet = %+v, want budget-exceeded without cached diff", file.StagedChange)
	}
	if file.UnstagedChange.Diff != "" || file.UnstagedChange.DiffSkipReason != diffSkipReasonBudgetExceeded {
		t.Errorf("unstaged facet = %+v, want budget-exceeded without cached diff", file.UnstagedChange)
	}
}

func fileInfoFromWire(t *testing.T, wire string) types.FileInfo {
	t.Helper()
	var file types.FileInfo
	if err := json.Unmarshal([]byte(wire), &file); err != nil {
		t.Fatalf("unmarshal FileInfo fixture: %v", err)
	}
	return file
}

func decodeMixedChangeFacet(
	t *testing.T,
	encoded map[string]json.RawMessage,
	name string,
) mixedChangeFacetWire {
	t.Helper()
	raw, ok := encoded[name]
	if !ok {
		full, _ := json.Marshal(encoded)
		t.Fatalf("FileInfo missing %s facet: %s", name, full)
	}
	var facet mixedChangeFacetWire
	if err := json.Unmarshal(raw, &facet); err != nil {
		t.Fatalf("unmarshal %s facet: %v", name, err)
	}
	return facet
}
