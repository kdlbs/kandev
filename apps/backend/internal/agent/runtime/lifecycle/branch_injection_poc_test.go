package lifecycle

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/config"
)

// TestBranchNameCommandInjection_Regression guards the CRITICAL RCE fix where an
// untrusted git branch name (e.g. a fork PR head branch, attacker controlled)
// was interpolated UNESCAPED into the prepare script that the Docker/Sprites
// executors run via `eval "$KANDEV_PREPARE_SCRIPT"` inside a shell.
//
// git check-ref-format permits `;`, `|`, `&`, `$`, backticks and `()` in ref
// names, so `$(touch pwned)` and `main;touch pwned;#` are legal, nameable refs.
//
// Before the fix (double-quoted postlude + fully-unquoted clone line, and no
// shell escaping in scriptengine), the snippets below executed the injected
// command — the marker file appeared on disk. This test FAILS before the fix
// and PASSES after: it asserts (1) the resolved script carries the payload only
// inside safe single quotes and (2) running the snippet through the real
// `sh -c`/`eval` sink does NOT create the marker.
//
// See the "PoC — before fix" write-up in the task for the original captured
// exploit output.
func TestBranchNameCommandInjection_Regression(t *testing.T) {
	exec := NewDockerExecutor(config.DockerConfig{}, "", newTestDockerLogger())

	// ---------------------------------------------------------------------
	// Case A: postlude checkout (was double-quoted; $(...) would substitute).
	// ---------------------------------------------------------------------
	t.Run("postlude does not run $() from branch name", func(t *testing.T) {
		tmp := t.TempDir()
		marker := filepath.Join(tmp, "pwned_postlude")

		workspace := filepath.Join(tmp, "workspace")
		if err := os.MkdirAll(filepath.Join(workspace, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}

		maliciousBranch := "$(touch " + marker + ")" // legal, nameable ref

		req := &ExecutorCreateRequest{
			Metadata: map[string]interface{}{
				"base_branch":          "main",
				"repository_clone_url": "https://github.com/org/repo.git",
				"repository_path":      "/tmp/repo",
				"worktree_branch":      maliciousBranch,
			},
			Env: map[string]string{},
		}

		script := exec.resolvePrepareScript(req)

		// The raw, unquoted `$(...)` must NOT appear naked. It may only appear
		// inside single quotes, where the shell treats it as a literal.
		assertNoUnquotedPayload(t, script, maliciousBranch)

		postlude := extractPostlude(t, script)
		postlude = strings.ReplaceAll(postlude, "/workspace", workspace)
		runViaEval(t, tmp, postlude)

		if _, err := os.Stat(marker); err == nil {
			t.Fatalf("REGRESSION: injected `touch` ran via postlude; marker created: %s", marker)
		}
		t.Logf("safe: postlude did not execute injected command; marker absent")
	})

	// ---------------------------------------------------------------------
	// Case B: unquoted clone line (was `--branch {{repository.branch}}`).
	// ---------------------------------------------------------------------
	t.Run("clone line does not run ; chained command from branch name", func(t *testing.T) {
		tmp := t.TempDir()
		marker := filepath.Join(tmp, "pwned_clone")

		maliciousBranch := "main;touch " + marker + ";#"

		req := &ExecutorCreateRequest{
			Metadata: map[string]interface{}{
				"base_branch":          maliciousBranch,
				"repository_clone_url": "https://github.com/org/repo.git",
				"repository_path":      "/tmp/repo",
				"worktree_branch":      "feature/task-abc",
			},
			Env: map[string]string{},
		}

		script := exec.resolvePrepareScript(req)
		assertNoUnquotedPayload(t, script, maliciousBranch)

		cloneLine := extractLine(t, script, "git clone --depth=1 --branch")
		runViaEval(t, tmp, cloneLine)

		if _, err := os.Stat(marker); err == nil {
			t.Fatalf("REGRESSION: injected `touch` ran via clone line; marker created: %s", marker)
		}
		t.Logf("safe: clone line did not execute injected command; marker absent")
	})
}

// assertNoUnquotedPayload fails if the payload appears in the script outside of
// a single-quoted context. Every line that mentions the payload must wrap it in
// single quotes (the only place command substitution / `;` is inert).
func assertNoUnquotedPayload(t *testing.T, script, payload string) {
	t.Helper()
	for _, line := range strings.Split(script, "\n") {
		// Every occurrence of the payload on the line must sit between a
		// preceding `'` and a following `'` (single-quoted, hence inert).
		for off := 0; ; {
			idx := strings.Index(line[off:], payload)
			if idx < 0 {
				break
			}
			abs := off + idx
			before := line[:abs]
			after := line[abs+len(payload):]
			if !strings.Contains(before, "'") || !strings.Contains(after, "'") {
				t.Fatalf("payload not single-quoted; line still injectable:\n%s", line)
			}
			off = abs + len(payload)
		}
	}
}

// runViaEval mimics container.go's bootstrap: it passes the snippet to a shell
// as $KANDEV_PREPARE_SCRIPT and runs `eval "$KANDEV_PREPARE_SCRIPT"` inside
// `sh -c`, exactly like the real execution sink.
func runViaEval(t *testing.T, dir, snippet string) {
	t.Helper()
	cmd := exec.Command("sh", "-c", `eval "$KANDEV_PREPARE_SCRIPT"`)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "KANDEV_PREPARE_SCRIPT="+snippet)
	out, err := cmd.CombinedOutput()
	// A non-zero exit is expected (git clone fails, checkout of a bogus branch
	// fails); the injected side effect is what we assert against.
	t.Logf("eval output (err=%v):\n%s", err, string(out))
}

// extractPostlude returns the kandev-managed postlude subshell block.
func extractPostlude(t *testing.T, script string) string {
	t.Helper()
	idx := strings.Index(script, "# ---- kandev-managed:")
	if idx < 0 {
		t.Fatalf("could not locate postlude in script:\n%s", script)
	}
	return script[idx:]
}

// extractLine returns the first line containing prefix.
func extractLine(t *testing.T, script, prefix string) string {
	t.Helper()
	for _, line := range strings.Split(script, "\n") {
		if strings.Contains(line, prefix) {
			return line
		}
	}
	t.Fatalf("could not find line with prefix %q in:\n%s", prefix, script)
	return ""
}
