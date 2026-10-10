package process

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const cumulativeHelperMarker = "--kandev-cumulative-external-helper"

func TestCumulativeExternalDiffHelperProcess(t *testing.T) {
	if os.Getenv("KANDEV_TEST_CUMULATIVE_EXTERNAL_HELPER") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == cumulativeHelperMarker && i+1 < len(os.Args) {
			if err := os.WriteFile(os.Args[i+1], []byte("executed\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			fmt.Print("CUSTOM DIFF OUTPUT\n")
			os.Exit(0)
		}
	}
	t.Fatal("helper invocation missing marker and sentinel")
}

type cumulativeExternalHelper struct{ command, sentinel string }

func newCumulativeExternalHelper(t *testing.T) cumulativeExternalHelper {
	t.Helper()
	t.Setenv("KANDEV_TEST_CUMULATIVE_EXTERNAL_HELPER", "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(t.TempDir(), "helper execution.txt")
	quote := func(value string) string {
		return "'" + strings.ReplaceAll(filepath.ToSlash(value), "'", "'\\''") + "'"
	}
	return cumulativeExternalHelper{
		command:  quote(executable) + " -test.run=^TestCumulativeExternalDiffHelperProcess$ -- " + cumulativeHelperMarker + " " + quote(sentinel),
		sentinel: sentinel,
	}
}

func (h cumulativeExternalHelper) assertAbsent(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(h.sentinel); !os.IsNotExist(err) {
		t.Errorf("external helper executed during built-in comparison: %v", err)
	}
}

func rawCumulativeGit(t *testing.T, repo string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir, cmd.Env = repo, append([]string(nil), env...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}

func (h cumulativeExternalHelper) prove(t *testing.T, repo, base, path string, env []string) {
	t.Helper()
	output := rawCumulativeGit(t, repo, append(append([]string(nil), env...), "GIT_EXTERNAL_DIFF="+h.command), "diff", base, "--", path)
	if !strings.Contains(output, "CUSTOM DIFF OUTPUT\n") {
		t.Fatalf("helper positive control did not emit custom output: %q", output)
	}
	data, err := os.ReadFile(h.sentinel)
	if err != nil || string(data) != "executed\n" {
		t.Fatalf("helper execution control = %q, err=%v", data, err)
	}
	if err := os.Remove(h.sentinel); err != nil {
		t.Fatal(err)
	}
}

func cumulativeExternalOperator(t *testing.T, repo string, env []string) *GitOperator {
	t.Helper()
	op := NewGitOperator(repo, newTestLogger(t), nil)
	captured := append([]string(nil), env...)
	op.setEnvironmentProvider(func() []string { return append([]string(nil), captured...) })
	return op
}

func cumulativeExternalState(t *testing.T, repo string) string {
	t.Helper()
	config, err := os.ReadFile(filepath.Join(repo, ".git", "config"))
	if err != nil {
		t.Fatal(err)
	}
	state := string(config) + runGit(t, repo, "rev-parse", "HEAD") + runGit(t, repo, "show-ref") +
		runGit(t, repo, "ls-files", "--stage", "-z") + runGit(t, repo, "--no-optional-locks", "status", "--porcelain=v1", "-z") +
		runGit(t, repo, "diff", "--no-ext-diff", "--no-color", "--binary", "HEAD")
	for _, path := range strings.Split(runGit(t, repo, "ls-files", "-z"), "\x00") {
		if path == "" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(repo, path))
		if err != nil {
			t.Fatal(err)
		}
		state += path + "\x00" + string(data) + "\x00"
	}
	return state
}

// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.3
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.5
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.8
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.9
func TestCumulativeDiffExternalHelpers(t *testing.T) {
	for _, mode := range []string{"plain", "configured", "environment", "both"} {
		t.Run(mode, func(t *testing.T) {
			repo, cleanup := setupTestRepo(t)
			t.Cleanup(cleanup)
			base := strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
			writeFile(t, repo, "README.md", "new file mode literal \x1b[31mcontent\x1b[0m\n")
			runGit(t, repo, "add", ".")
			runGit(t, repo, "commit", "-m", "external helper comparison")
			head := strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
			configured, environment := newCumulativeExternalHelper(t), newCumulativeExternalHelper(t)
			env := filterTestGitEnv(os.Environ())
			configured.prove(t, repo, base, "README.md", env)
			environment.prove(t, repo, base, "README.md", env)
			if mode == "configured" || mode == "both" {
				runGit(t, repo, "config", "diff.external", configured.command)
			}
			if mode == "environment" || mode == "both" {
				env = append(env, "GIT_EXTERNAL_DIFF="+environment.command)
			}
			op := cumulativeExternalOperator(t, repo, env)
			before := cumulativeExternalState(t, repo)
			t.Cleanup(func() {
				configured.assertAbsent(t)
				environment.assertAbsent(t)
				if cumulativeExternalState(t, repo) != before || !reflect.DeepEqual(op.environmentValues(), env) {
					t.Error("comparison changed repository or captured helper environment")
				}
			})
			t.Run("commit_control", func(t *testing.T) {
				result, err := op.ShowCommit(context.Background(), head)
				if err != nil || !result.Success || result.CommitSHA != head || result.FilesChanged != 1 || result.Insertions != 1 || result.Deletions != 1 {
					t.Fatalf("commit control = %+v, err=%v", result, err)
				}
				assertStatusMetadataChanges(t, result.Files, []statusMetadataChange{{path: "README.md", status: "modified", additions: 1, deletions: 1}},
					runGit(t, repo, "show", "--no-ext-diff", "--no-color", "--format=", "-p", "--src-prefix=a/", "--dst-prefix=b/", head))
			})
			t.Run("committed", func(t *testing.T) { assertExternalCumulative(t, op, repo, base, head, 1) })
			t.Run("dirty", func(t *testing.T) {
				writeFile(t, repo, "README.md", "new file mode literal \x1b[31mcontent\x1b[0m\ndirty tracked sentinel\n")
				defer writeFile(t, repo, "README.md", "new file mode literal \x1b[31mcontent\x1b[0m\n")
				assertExternalCumulative(t, op, repo, base, head, 2)
			})
			t.Run("empty", func(t *testing.T) {
				result, err := op.GetCumulativeDiff(context.Background(), head)
				if err != nil || !result.Success || len(result.Files) != 0 || result.BaseCommit != head || result.HeadCommit != head || result.TotalCommits != 0 || result.TruncatedFilesCount != 0 {
					t.Fatalf("empty comparison = %+v, err=%v", result, err)
				}
			})
		})
	}
}

func assertExternalCumulative(t *testing.T, op *GitOperator, repo, base, head string, additions int) {
	t.Helper()
	before := cumulativeExternalState(t, repo)
	result, err := op.GetCumulativeDiff(context.Background(), base)
	if err != nil || !result.Success || result.BaseCommit != base || result.HeadCommit != head || result.TotalCommits != 1 || result.TruncatedFilesCount != 0 {
		t.Fatalf("cumulative = %+v, err=%v", result, err)
	}
	assertStatusMetadataChanges(t, result.Files, []statusMetadataChange{{path: "README.md", status: "modified", additions: additions, deletions: 1}},
		runGit(t, repo, "diff", "--no-ext-diff", "--no-color", "--src-prefix=a/", "--dst-prefix=b/", base))
	if cumulativeExternalState(t, repo) != before {
		t.Error("cumulative read changed dirty state")
	}
}

// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.4
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.8
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.9
func TestCumulativeDiffExternalHelperBudgets(t *testing.T) {
	for _, limit := range []string{"per_file", "total_bytes", "file_count"} {
		t.Run(limit, func(t *testing.T) {
			repo, cleanup := setupTestRepo(t)
			t.Cleanup(cleanup)
			base := strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
			seedExternalBudget(t, repo, limit)
			helper := newCumulativeExternalHelper(t)
			env := filterTestGitEnv(os.Environ())
			op := cumulativeExternalOperator(t, repo, env)
			plain, err := op.GetCumulativeDiff(context.Background(), base)
			if err != nil || !plain.Success || len(plain.Files) == 0 {
				t.Fatalf("plain budget control = %+v, err=%v", plain, err)
			}
			assertExternalBudget(t, plain, limit)
			helper.prove(t, repo, base, fileKeys(plain.Files)[0], env)
			runGit(t, repo, "config", "diff.external", helper.command)
			before := cumulativeExternalState(t, repo)
			t.Cleanup(func() {
				helper.assertAbsent(t)
				if cumulativeExternalState(t, repo) != before {
					t.Error("budgeted read changed repository")
				}
			})
			result, err := op.GetCumulativeDiff(context.Background(), base)
			if err != nil || !reflect.DeepEqual(result, plain) {
				t.Fatalf("helper budget comparison differs: files=%d, want=%d, err=%v", len(result.Files), len(plain.Files), err)
			}
		})
	}
}

func seedExternalBudget(t *testing.T, repo, limit string) {
	t.Helper()
	switch limit {
	case "per_file":
		writeFile(t, repo, "README.md", strings.Repeat("large cumulative sentinel\n", 12000))
	case "total_bytes":
		for i := 0; i < 12; i++ {
			writeFile(t, repo, fmt.Sprintf("budget_%02d.txt", i), strings.Repeat("total cumulative sentinel\n", 8000))
		}
	case "file_count":
		for i := 0; i < maxCumulativeDiffFiles+1; i++ {
			writeFile(t, repo, fmt.Sprintf("count_%04d.txt", i), "count sentinel\n")
		}
	}
	runGit(t, repo, "add", ".")
}

func assertExternalBudget(t *testing.T, result *CumulativeDiffResult, limit string) {
	t.Helper()
	switch limit {
	case "per_file":
		patch, reason := fileEntryDiff(t, result.Files, "README.md")
		if len(result.Files) != 1 || reason != diffSkipReasonTruncated || len(patch) > maxDiffOutputSize {
			t.Fatal("per-file control did not exercise truncation")
		}
	case "total_bytes":
		skipped := 0
		for path := range result.Files {
			patch, reason := fileEntryDiff(t, result.Files, path)
			if reason == diffSkipReasonBudgetExceeded {
				skipped++
				if patch != "" {
					t.Fatal("budget-exceeded patch was not empty")
				}
			}
		}
		if len(result.Files) != 12 || skipped == 0 {
			t.Fatal("total-byte control did not exercise the budget")
		}
	case "file_count":
		if len(result.Files) != maxCumulativeDiffFiles || result.TruncatedFilesCount != 1 {
			t.Fatal("file-count control did not exercise the cap")
		}
	}
}
