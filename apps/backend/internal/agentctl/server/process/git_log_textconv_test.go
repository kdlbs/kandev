package process

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const comparisonTextconvMarker = "--kandev-comparison-textconv-helper"
const comparisonTextconvPath = "content with spaces.txt"

func TestComparisonTextconvHelperProcess(t *testing.T) {
	if os.Getenv("KANDEV_TEST_COMPARISON_TEXTCONV") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg != comparisonTextconvMarker || i+3 >= len(os.Args) {
			continue
		}
		data, err := os.ReadFile(os.Args[i+3])
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(os.Args[i+1], []byte("executed\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		switch os.Args[i+2] {
		case "constant":
			fmt.Print("same converted bytes\n")
		case "changed":
			fmt.Print("CONVERTED " + strings.ToUpper(string(data)))
		default:
			t.Fatal("unknown converter mode")
		}
		os.Exit(0)
	}
	t.Fatal("converter invocation missing private arguments")
}

type comparisonTextconvHelper struct{ command, sentinel, mode string }

func newComparisonTextconvHelper(t *testing.T, mode string) comparisonTextconvHelper {
	t.Helper()
	t.Setenv("KANDEV_TEST_COMPARISON_TEXTCONV", "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(t.TempDir(), "converter execution.txt")
	quote := func(value string) string {
		return "'" + strings.ReplaceAll(filepath.ToSlash(value), "'", "'\\''") + "'"
	}
	return comparisonTextconvHelper{
		command: quote(executable) + " -test.run=^TestComparisonTextconvHelperProcess$ -- " +
			comparisonTextconvMarker + " " + quote(sentinel) + " " + mode,
		sentinel: sentinel,
		mode:     mode,
	}
}

func (h comparisonTextconvHelper) configure(t *testing.T, repo, attributes string) {
	t.Helper()
	writeFile(t, repo, ".git/info/attributes", attributes)
	runGit(t, repo, "config", "diff.probe.textconv", h.command)
	runGit(t, repo, "config", "diff.probe.cachetextconv", "false")
}

func (h comparisonTextconvHelper) prove(t *testing.T, repo, base, path string, env []string) {
	t.Helper()
	output := rawCumulativeGit(t, repo, env, "diff", "--textconv", "--no-ext-diff", "--no-color", base, "--", path)
	if h.mode == "constant" && output != "" {
		t.Fatalf("constant converter did not suppress patch: %q", output)
	}
	if h.mode == "changed" && (!strings.Contains(output, "CONVERTED ") || !strings.Contains(output, "diff --git ")) {
		t.Fatalf("changing converter did not alter patch: %q", output)
	}
	data, err := os.ReadFile(h.sentinel)
	if err != nil || string(data) != "executed\n" {
		t.Fatalf("converter positive execution control = %q, err=%v", data, err)
	}
	if err := os.Remove(h.sentinel); err != nil {
		t.Fatal(err)
	}
}

func (h comparisonTextconvHelper) assertAbsent(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(h.sentinel); !os.IsNotExist(err) {
		t.Errorf("text converter executed during comparison: %v", err)
	}
}

func comparisonTextconvState(t *testing.T, repo string) map[string]string {
	t.Helper()
	state := make(map[string]string)
	for _, path := range []string{".git/config", ".git/index", ".git/info/attributes"} {
		data, err := os.ReadFile(filepath.Join(repo, path))
		if os.IsNotExist(err) {
			state[path] = "absent"
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		state[path] = string(data)
	}
	state["HEAD"] = runGit(t, repo, "rev-parse", "HEAD")
	state["refs"] = runGit(t, repo, "show-ref")
	state["entries"] = runGit(t, repo, "ls-files", "--stage", "-z")
	state["status"] = runGit(t, repo, "--no-optional-locks", "status", "--porcelain=v1", "-z")
	state["dirty"] = runGit(t, repo, "diff", "--no-textconv", "--no-ext-diff", "--no-color", "--binary", "HEAD")
	for _, path := range strings.Split(runGit(t, repo, "ls-files", "-z"), "\x00") {
		if path == "" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(repo, path))
		if err != nil {
			t.Fatal(err)
		}
		state["file:"+path] = string(data)
	}
	return state
}

func guardComparisonTextconvRead(t *testing.T, repo string, helper comparisonTextconvHelper) {
	t.Helper()
	before := comparisonTextconvState(t, repo)
	t.Cleanup(func() {
		helper.assertAbsent(t)
		if !reflect.DeepEqual(before, comparisonTextconvState(t, repo)) {
			t.Error("comparison changed config, attributes, HEAD, refs, index or content")
		}
	})
}

func comparisonTextconvPatch(t *testing.T, repo, ref string, commit bool) string {
	t.Helper()
	args := []string{"diff", "--no-textconv", "--no-ext-diff", "--no-color", "--src-prefix=a/", "--dst-prefix=b/", ref}
	if commit {
		args = []string{"show", "--first-parent", "--no-textconv", "--no-ext-diff", "--no-color", "--format=", "-p", "--src-prefix=a/", "--dst-prefix=b/", ref}
	}
	return runGit(t, repo, args...)
}

func assertComparisonTextconvCommit(t *testing.T, repo, head string, changes []statusMetadataChange, helper comparisonTextconvHelper) {
	t.Helper()
	guardComparisonTextconvRead(t, repo, helper)
	result, err := newPlainOutputOperator(t, repo).ShowCommit(context.Background(), head)
	if err != nil || !result.Success {
		t.Fatalf("ShowCommit = %+v, err=%v", result, err)
	}
	metadata := strings.Split(strings.TrimSpace(runGit(t, repo, "show", "--no-patch", "--format=%H%n%s%n%an <%ae>%n%aI", head)), "\n")
	if len(metadata) != 4 || result.CommitSHA != metadata[0] || result.Message != metadata[1] || result.Author != metadata[2] || result.Date != metadata[3] {
		t.Errorf("commit metadata = %+v, want %v", result, metadata)
	}
	additions, deletions := 0, 0
	for _, change := range changes {
		additions += change.additions
		deletions += change.deletions
	}
	if result.FilesChanged != len(changes) || result.Insertions != additions || result.Deletions != deletions {
		t.Errorf("commit totals = %d +%d/-%d, want %d +%d/-%d", result.FilesChanged, result.Insertions, result.Deletions, len(changes), additions, deletions)
	}
	assertStatusMetadataChanges(t, result.Files, changes, comparisonTextconvPatch(t, repo, head, true))
}

func assertComparisonTextconvCumulative(t *testing.T, repo, base, head string, commits int, changes []statusMetadataChange, helper comparisonTextconvHelper) {
	t.Helper()
	guardComparisonTextconvRead(t, repo, helper)
	result, err := newPlainOutputOperator(t, repo).GetCumulativeDiff(context.Background(), base)
	if err != nil || !result.Success || result.BaseCommit != base || result.HeadCommit != head || result.TotalCommits != commits || result.TruncatedFilesCount != 0 {
		t.Fatalf("GetCumulativeDiff = %+v, err=%v", result, err)
	}
	assertStatusMetadataChanges(t, result.Files, changes, comparisonTextconvPatch(t, repo, base, false))
}

// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.3
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.10
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.11
func TestGitComparisonTextconv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	for _, mode := range []string{"no_driver", "changed", "constant"} {
		t.Run(mode, func(t *testing.T) {
			repo, cleanup := setupTestRepo(t)
			t.Cleanup(cleanup)
			writeFile(t, repo, comparisonTextconvPath, "original bytes\n")
			runGit(t, repo, "add", ".")
			runGit(t, repo, "commit", "-m", "textconv baseline")
			base := strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
			writeFile(t, repo, comparisonTextconvPath, "changed bytes\n")
			runGit(t, repo, "add", ".")
			runGit(t, repo, "commit", "-m", "actual byte change")
			head := strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
			helper := newComparisonTextconvHelper(t, mode)
			if mode != "no_driver" {
				helper.configure(t, repo, "\""+comparisonTextconvPath+"\" diff=probe\n")
				helper.prove(t, repo, base, comparisonTextconvPath, filterTestGitEnv(os.Environ()))
			}
			patch := comparisonTextconvPatch(t, repo, base, false)
			if !strings.Contains(patch, "-original bytes\n+changed bytes\n") {
				t.Fatalf("raw actual-byte oracle lacks expected change: %q", patch)
			}
			changes := []statusMetadataChange{{path: comparisonTextconvPath, status: "modified", additions: 1, deletions: 1}}
			t.Run("commit", func(t *testing.T) { assertComparisonTextconvCommit(t, repo, head, changes, helper) })
			t.Run("cumulative", func(t *testing.T) { assertComparisonTextconvCumulative(t, repo, base, head, 1, changes, helper) })
			writeFile(t, repo, comparisonTextconvPath, "changed bytes\ndirty actual bytes\n")
			changes[0].additions = 2
			t.Run("dirty", func(t *testing.T) { assertComparisonTextconvCumulative(t, repo, base, head, 1, changes, helper) })
		})
	}
}

// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.2
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.5
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.10
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.11
func TestGitComparisonTextconvControls(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	repo, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	writeFile(t, repo, "binary.bin", "\x00original bytes\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "binary baseline")
	base := strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
	writeFile(t, repo, "binary.bin", "\x00changed bytes\n")
	writeFile(t, repo, "empty.txt", "")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "binary and empty file")
	head := strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
	helper := newComparisonTextconvHelper(t, "constant")
	helper.configure(t, repo, "binary.bin diff=probe\nempty.txt diff=probe\n")
	helper.prove(t, repo, base, "binary.bin", filterTestGitEnv(os.Environ()))
	changes := []statusMetadataChange{{path: "binary.bin", status: "modified"}, {path: "empty.txt", status: "added"}}
	t.Run("commit", func(t *testing.T) { assertComparisonTextconvCommit(t, repo, head, changes, helper) })
	t.Run("cumulative", func(t *testing.T) { assertComparisonTextconvCumulative(t, repo, base, head, 1, changes, helper) })
	runGit(t, repo, "commit", "--allow-empty", "-m", "genuinely empty")
	empty := strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
	t.Run("empty_commit", func(t *testing.T) { assertComparisonTextconvCommit(t, repo, empty, nil, helper) })
	t.Run("empty_cumulative", func(t *testing.T) { assertComparisonTextconvCumulative(t, repo, empty, empty, 0, nil, helper) })
}
