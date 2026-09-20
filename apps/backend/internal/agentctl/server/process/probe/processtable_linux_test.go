//go:build linux

package probe

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// spawnChildWithEnv starts a real, long-lived child with exactly the given
// extra environment entries appended to a copy of this process's own
// environment, having first stripped any existing KANDEV_SESSION_ID so a
// test's own ambient value (this test binary may itself be running inside a
// Kandev-managed session) can never masquerade as the first occurrence.
func spawnChildWithEnv(t *testing.T, extraEnv ...string) int {
	t.Helper()
	base := os.Environ()
	env := make([]string, 0, len(base)+len(extraEnv))
	for _, kv := range base {
		if strings.HasPrefix(kv, "KANDEV_SESSION_ID=") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, extraEnv...)

	cmd := exec.Command("sleep", "30")
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		t.Fatalf("spawn child: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	return cmd.Process.Pid
}

// AC-DW-ORPHAN-001.10: exact equality against the real environ blob.
func TestLinuxHasSessionID_ExactMatch(t *testing.T) {
	pid := spawnChildWithEnv(t, "KANDEV_SESSION_ID=exact-match-value")

	reader := linuxProcessTableReader{}
	matched, err := reader.HasSessionID(pid, "exact-match-value")
	if err != nil {
		t.Fatalf("HasSessionID: %v", err)
	}
	if !matched {
		t.Errorf("expected an exact KANDEV_SESSION_ID match to succeed")
	}
}

// AC-DW-ORPHAN-001.10: the environment is parsed as discrete NAME=VALUE
// entries, never scanned as raw text — a similarly-named variable must not
// match. Go's os/exec de-duplicates a Cmd.Env slice before exec, so the
// parsing edge cases (similarly-named variables, duplicate entries) are
// exercised directly against parseFirstEnvVar's synthetic
// /proc/<pid>/environ-shaped input below, rather than through a real
// subprocess.
func TestParseFirstEnvVar_DoesNotMatchSimilarlyNamedVar(t *testing.T) {
	data := []byte("MY_KANDEV_SESSION_ID=exact-match-value\x00KANDEV_SESSION_ID_OLD=exact-match-value\x00")

	if _, ok := parseFirstEnvVar(data, kandevSessionIDEnvVar); ok {
		t.Errorf("expected a similarly-named variable to never match KANDEV_SESSION_ID")
	}
}

// AC-DW-ORPHAN-001.10: when KANDEV_SESSION_ID appears more than once, the
// first occurrence decides and the rest are ignored.
func TestParseFirstEnvVar_DuplicateEntryUsesFirstOccurrence(t *testing.T) {
	data := []byte("KANDEV_SESSION_ID=first\x00KANDEV_SESSION_ID=second\x00")

	value, ok := parseFirstEnvVar(data, kandevSessionIDEnvVar)
	if !ok {
		t.Fatalf("expected a match")
	}
	if value != "first" {
		t.Errorf("got %q, want %q — the first occurrence must decide", value, "first")
	}
}

// A trailing empty field after the final NUL is not an entry, and must not
// be mistaken for an empty-named variable.
func TestParseFirstEnvVar_TrailingEmptyFieldIsNotAnEntry(t *testing.T) {
	data := []byte("KANDEV_SESSION_ID=value\x00")

	value, ok := parseFirstEnvVar(data, kandevSessionIDEnvVar)
	if !ok || value != "value" {
		t.Errorf("got (%q, %v), want (%q, true)", value, ok, "value")
	}
}

// AC-DW-ORPHAN-002.2: a candidate that has already exited yields an error,
// not a match — the caller treats this as "skip", never "unknown".
func TestLinuxHasSessionID_ExitedProcess_ReturnsError(t *testing.T) {
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatalf("run child: %v", err)
	}

	reader := linuxProcessTableReader{}
	if _, err := reader.HasSessionID(cmd.Process.Pid, "anything"); err == nil {
		t.Errorf("expected an error reading an exited process's environment")
	}
}

// StartTimeDatum returns the same raw ticks value on repeated reads of an
// unchanged, still-running process — the invariant AC-DW-ORPHAN-001.11's
// re-validation depends on.
func TestLinuxStartTimeDatum_StableAcrossRepeatedReads(t *testing.T) {
	pid := spawnChildWithEnv(t)

	reader := linuxProcessTableReader{}
	first, err := reader.StartTimeDatum(pid)
	if err != nil {
		t.Fatalf("StartTimeDatum: %v", err)
	}
	second, err := reader.StartTimeDatum(pid)
	if err != nil {
		t.Fatalf("StartTimeDatum: %v", err)
	}
	if first != second {
		t.Errorf("expected a stable start-time datum for an unchanged process, got %d then %d", first, second)
	}
}
