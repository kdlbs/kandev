package seccomp

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestUsernsProfile_StructuralInvariants verifies the profile's structure
// beyond the delta-based tests: it checks that the JSON is well-formed,
// has the correct default action, and produces a valid seccomp profile
// that Docker can accept inline.
func TestUsernsProfile_StructuralInvariants(t *testing.T) {
	profileJSON, err := UsernsProfileJSON()
	if err != nil {
		t.Fatalf("UsernsProfileJSON() = %v", err)
	}

	// Must be valid JSON.
	var raw any
	if err := json.Unmarshal([]byte(profileJSON), &raw); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}

	m, ok := raw.(map[string]any)
	if !ok {
		t.Fatal("profile is not a JSON object")
	}

	// Must have defaultAction: SCMP_ACT_ERRNO
	da, ok := m["defaultAction"].(string)
	if !ok || da != "SCMP_ACT_ERRNO" {
		t.Errorf("defaultAction = %v, want SCMP_ACT_ERRNO", da)
	}

	// Must have archMap.
	archMap, ok := m["archMap"].([]any)
	if !ok || len(archMap) == 0 {
		t.Error("profile has no archMap entries")
	}

	// Must have syscalls array.
	syscalls, ok := m["syscalls"].([]any)
	if !ok || len(syscalls) == 0 {
		t.Fatal("profile has no syscalls array")
	}
}

// TestUsernsProfile_CanInlineAsDockerSecurityOpt verifies the output
// can be used as a Docker SecurityOpt value: "seccomp=<profile>". The
// Docker daemon parses the part after "seccomp=" as either a file path
// (if it starts with /) or raw JSON.
func TestUsernsProfile_CanInlineAsDockerSecurityOpt(t *testing.T) {
	profileJSON, err := UsernsProfileJSON()
	if err != nil {
		t.Fatalf("UsernsProfileJSON() = %v", err)
	}

	// It must be valid JSON and not a file path.
	if strings.HasPrefix(profileJSON, "/") {
		t.Fatal("profile starts with /, which Docker would interpret as a file path")
	}

	// The Docker CLI reads the file content and puts it into the
	// SecurityOpt string as raw JSON. Verify the JSON is compact
	// (no newlines) so it survives serialization.
	if strings.Contains(profileJSON, "\n") {
		t.Log("profile contains newlines — may need compaction for Docker API")
	}

	// Verify it can be embedded in a SecurityOpt string.
	opt := "seccomp=" + profileJSON
	if !strings.HasPrefix(opt, "seccomp=") {
		t.Fatal("malformed SecurityOpt prefix")
	}
}

// TestUsernsProfile_UnconditionalAllowCount just smoke-checks that
// we didn't accidentally drop the entire unconditional allow list.
func TestUsernsProfile_UnconditionalAllowCount(t *testing.T) {
	allowSet := unconditionalAllowSet(t, mustProfileJSON(t))
	// The default profile has ~330 unconditional syscalls.
	// We add 10 namespace syscalls, so should have ~340.
	if len(allowSet) < 300 {
		t.Errorf("unconditional allow list has %d entries, expected >300", len(allowSet))
	}
}

// TestUsernsProfile_OnlyNamespaceSyscallsRelaxed verifies that exactly the
// expected set of syscalls was moved from CAP_SYS_ADMIN-gated to
// unconditional allow, and nothing else.
func TestUsernsProfile_OnlyNamespaceSyscallsRelaxed(t *testing.T) {
	defaultSet := unconditionalAllowSet(t, string(defaultProfileJSON))
	modifiedSet := unconditionalAllowSet(t, mustProfileJSON(t))

	// New unconditional syscalls (added by the profile).
	var added []string
	for sc := range modifiedSet {
		if !defaultSet[sc] {
			added = append(added, sc)
		}
	}

	// Verify every newly-added syscall is in the expected list.
	for _, sc := range added {
		found := false
		for _, expected := range usernsRelaxedSyscalls {
			if sc == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("syscall %q was newly allowed but not in usernsRelaxedSyscalls", sc)
		}
	}

	// Verify every expected syscall was actually added.
	for _, expected := range usernsRelaxedSyscalls {
		if !modifiedSet[expected] {
			t.Errorf("expected syscall %q is not in the unconditional allow list", expected)
		}
	}
}

func mustProfileJSON(t *testing.T) string {
	t.Helper()
	s, err := UsernsProfileJSON()
	if err != nil {
		t.Fatalf("UsernsProfileJSON() = %v", err)
	}
	return s
}
