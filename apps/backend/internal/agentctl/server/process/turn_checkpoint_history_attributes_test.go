package process

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/turnchanges"
)

func TestTurnCheckpointHistoryAllowsBuiltInAttributes(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	writeFile(t, repoDir, ".gitattributes", "* text=auto eol=lf\n*.txt merge\n*.md merge=union\nmerge text\n")
	runGit(t, repoDir, "add", ".gitattributes")
	runGit(t, repoDir, "commit", "-q", "-m", "ordinary attributes")
	_, tracker, operator, ref := setupHistoryTarget(t, repoDir)
	base := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	writeFile(t, repoDir, "README.md", "local conflict\n")
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointStart)

	runGit(t, repoDir, "checkout", "-q", "--detach", base)
	writeFile(t, repoDir, "README.md", "upstream conflict\n")
	runGit(t, repoDir, "add", "README.md")
	runGit(t, repoDir, "commit", "-q", "-m", "upstream conflict")
	upstream := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	runGit(t, repoDir, "update-ref", ref, upstream)
	tracker.SetComparisonTargetReady(turnChangeTargetPointer(), ref)
	runGit(t, repoDir, "checkout", "-q", "-B", "main", upstream)
	captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointEnd)

	comparison := compareHistoryEndpoints(t, operator, changeSetID, checkoutID)
	if comparison.ComparisonBasis != turnchanges.ComparisonUpstreamAdjusted {
		t.Fatalf("union attribute comparison = %+v", comparison)
	}
}

func TestTurnCheckpointHistoryRejectsCustomMergeAttribute(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	writeFile(t, repoDir, ".gitattributes", "*.txt merge=custom\n")
	runGit(t, repoDir, "add", ".gitattributes")
	runGit(t, repoDir, "commit", "-q", "-m", "custom merge attribute")
	_, tracker, operator, ref := setupHistoryTarget(t, repoDir)
	base := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	writeFile(t, repoDir, "note.txt", "local\n")
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointStart)

	runGit(t, repoDir, "checkout", "-q", "--detach", base)
	writeFile(t, repoDir, "note.txt", "upstream\n")
	runGit(t, repoDir, "add", "note.txt")
	runGit(t, repoDir, "commit", "-q", "-m", "upstream")
	upstream := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	runGit(t, repoDir, "update-ref", ref, upstream)
	tracker.SetComparisonTargetReady(turnChangeTargetPointer(), ref)
	runGit(t, repoDir, "checkout", "-q", "-B", "main", upstream)
	captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointEnd)

	comparison := compareHistoryEndpoints(t, operator, changeSetID, checkoutID)
	if comparison.ComparisonBasis != turnchanges.ComparisonHistoryUnavailable ||
		comparison.ComparisonReason != turnchanges.ReasonHistoryUnsupported {
		t.Fatalf("custom merge attribute comparison = %+v", comparison)
	}
}

func TestTurnCheckpointHistoryNeverRunsRepositoryMergeDefaultDriver(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	marker := filepath.Join(t.TempDir(), "merge-driver-ran")
	runGit(t, repoDir, "config", "merge.default", "marker")
	runGit(t, repoDir, "config", "merge.marker.driver",
		"sh -c 'touch "+marker+"; exit 1' %O %A %B")
	t.Setenv("GIT_CONFIG_PARAMETERS", "'merge.default'='marker'")
	_, tracker, operator, ref := setupHistoryTarget(t, repoDir)
	base := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	writeFile(t, repoDir, "README.md", "local conflict\n")
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointStart)

	runGit(t, repoDir, "checkout", "-q", "--detach", base)
	writeFile(t, repoDir, "README.md", "upstream conflict\n")
	runGit(t, repoDir, "add", "README.md")
	runGit(t, repoDir, "commit", "-q", "-m", "upstream conflict")
	upstream := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	runGit(t, repoDir, "update-ref", ref, upstream)
	tracker.SetComparisonTargetReady(turnChangeTargetPointer(), ref)
	runGit(t, repoDir, "checkout", "-q", "-B", "main", upstream)
	captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointEnd)

	comparison := compareHistoryEndpoints(t, operator, changeSetID, checkoutID)
	if comparison.ComparisonReason != turnchanges.ReasonHistoryConflict {
		t.Fatalf("repository driver comparison = %+v", comparison)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("repository merge driver executed: %v", err)
	}
}
