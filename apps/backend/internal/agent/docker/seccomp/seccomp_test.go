package seccomp

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// usernsProfileAllowedSyscalls returns the set of syscalls that appear in a
// SCMP_ACT_ALLOW rule with no capability gating, no arch specifics, no
// kernel version gating, no argument restrictions, and no excludes — i.e.
// the unconditional allow list.
func unconditionalAllowSet(t *testing.T, profileJSON string) map[string]bool {
	t.Helper()
	var profile struct {
		Syscalls []json.RawMessage `json:"syscalls"`
	}
	if err := json.Unmarshal([]byte(profileJSON), &profile); err != nil {
		t.Fatalf("unmarshal profile: %v", err)
	}

	allowSet := make(map[string]bool)
	for _, raw := range profile.Syscalls {
		var rule struct {
			Names    []string          `json:"names"`
			Action   string            `json:"action"`
			Includes *json.RawMessage  `json:"includes,omitempty"`
			Excludes *json.RawMessage  `json:"excludes,omitempty"`
			Args     []json.RawMessage `json:"args,omitempty"`
		}
		if err := json.Unmarshal(raw, &rule); err != nil {
			continue
		}
		if rule.Action == "SCMP_ACT_ALLOW" && rule.Includes == nil && rule.Excludes == nil && len(rule.Args) == 0 {
			for _, name := range rule.Names {
				allowSet[name] = true
			}
		}
	}
	return allowSet
}

// capAdminGated returns the syscalls that are allowed only with CAP_SYS_ADMIN.
func capAdminGated(t *testing.T, profileJSON string) map[string]bool {
	t.Helper()
	var profile struct {
		Syscalls []json.RawMessage `json:"syscalls"`
	}
	if err := json.Unmarshal([]byte(profileJSON), &profile); err != nil {
		t.Fatalf("unmarshal profile: %v", err)
	}

	gated := make(map[string]bool)
	for _, raw := range profile.Syscalls {
		var rule struct {
			Names    []string `json:"names"`
			Action   string   `json:"action"`
			Includes *struct {
				Caps []string `json:"caps,omitempty"`
			} `json:"includes,omitempty"`
		}
		if err := json.Unmarshal(raw, &rule); err != nil {
			continue
		}
		if rule.Action == "SCMP_ACT_ALLOW" && rule.Includes != nil && slices.Contains(rule.Includes.Caps, "CAP_SYS_ADMIN") {
			for _, name := range rule.Names {
				gated[name] = true
			}
		}
	}
	return gated
}

func TestUsernsProfile_IsValidJSON(t *testing.T) {
	profileJSON, err := UsernsProfileJSON()
	if err != nil {
		t.Fatalf("UsernsProfileJSON() returned error: %v", err)
	}
	if !strings.HasPrefix(profileJSON, "{") {
		t.Fatalf("profile does not start with {, got: %s", profileJSON[:20])
	}
}

func TestUsernsProfile_NamespaceSyscalls_Unconditional(t *testing.T) {
	profileJSON, err := UsernsProfileJSON()
	if err != nil {
		t.Fatalf("UsernsProfileJSON() returned error: %v", err)
	}

	allowSet := unconditionalAllowSet(t, profileJSON)
	capAdminSet := capAdminGated(t, profileJSON)

	for _, sc := range usernsRelaxedSyscalls {
		if !allowSet[sc] {
			t.Errorf("namespace syscall %q is not in the unconditional allow list; in CAP_ADMIN-gated: %v", sc, capAdminSet[sc])
		}
		if capAdminSet[sc] {
			t.Errorf("namespace syscall %q is still in the CAP_SYS_ADMIN-gated group", sc)
		}
	}
}

func TestUsernsProfile_DockerBlockedSyscalls_StayBlocked(t *testing.T) {
	profileJSON, err := UsernsProfileJSON()
	if err != nil {
		t.Fatalf("UsernsProfileJSON() returned error: %v", err)
	}

	allowSet := unconditionalAllowSet(t, profileJSON)

	// Syscalls Docker blocks by default (not in the allow set, not in
	// any conditional allow) must remain blocked.
	//
	// Note: some of these like mount, setns, unshare etc are now in the
	// allow set — they are the whole point of the change. We check only
	// the ones Docker denies even to CAP_SYS_ADMIN.
	//
	// The key check is that high-risk syscalls that Docker blocks even
	// with capabilities stay blocked.
	syscallsThatMustStayBlocked := []string{
		"add_key",
		"io_uring_enter",
		"io_uring_register",
		"io_uring_setup",
		"kexec_file_load",
		"kexec_load",
		"nfsservctl",
		"userfaultfd",
	}

	for _, sc := range syscallsThatMustStayBlocked {
		if allowSet[sc] {
			t.Errorf("high-risk syscall %q is now unconditionally allowed — it should remain blocked", sc)
		}
	}
}

func TestUsernsProfile_DefaultActionIsErrno(t *testing.T) {
	profileJSON, err := UsernsProfileJSON()
	if err != nil {
		t.Fatalf("UsernsProfileJSON() returned error: %v", err)
	}
	var profile struct {
		DefaultAction   string `json:"defaultAction"`
		DefaultErrnoRet *uint  `json:"defaultErrnoRet"`
	}
	if err := json.Unmarshal([]byte(profileJSON), &profile); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if profile.DefaultAction != "SCMP_ACT_ERRNO" {
		t.Errorf("defaultAction = %q, want SCMP_ACT_ERRNO", profile.DefaultAction)
	}
	if profile.DefaultErrnoRet == nil || *profile.DefaultErrnoRet != 1 {
		t.Errorf("defaultErrnoRet should be 1")
	}
}

func TestUsernsProfile_CloneMaskedEqRemoved(t *testing.T) {
	profileJSON, err := UsernsProfileJSON()
	if err != nil {
		t.Fatalf("UsernsProfileJSON() returned error: %v", err)
	}

	var profile struct {
		Syscalls []json.RawMessage `json:"syscalls"`
	}
	if err := json.Unmarshal([]byte(profileJSON), &profile); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for i, raw := range profile.Syscalls {
		var rule struct {
			Names []string          `json:"names"`
			Args  []json.RawMessage `json:"args"`
		}
		if err := json.Unmarshal(raw, &rule); err != nil {
			continue
		}
		if slices.Contains(rule.Names, "clone") && len(rule.Args) > 0 {
			for _, argRaw := range rule.Args {
				var arg struct {
					Op string `json:"op"`
				}
				if json.Unmarshal(argRaw, &arg) == nil && arg.Op == "SCMP_CMP_MASKED_EQ" {
					t.Errorf("rule %d still has SCMP_CMP_MASKED_EQ clone restriction (index %d)", i, rule.Args)
				}
			}
		}
	}
}

func TestUsernsProfile_Clone3ErrnoRemoved(t *testing.T) {
	profileJSON, err := UsernsProfileJSON()
	if err != nil {
		t.Fatalf("UsernsProfileJSON() returned error: %v", err)
	}

	var profile struct {
		Syscalls []json.RawMessage `json:"syscalls"`
	}
	if err := json.Unmarshal([]byte(profileJSON), &profile); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for i, raw := range profile.Syscalls {
		var rule struct {
			Names  []string `json:"names"`
			Action string   `json:"action"`
		}
		if err := json.Unmarshal(raw, &rule); err != nil {
			continue
		}
		if slices.Contains(rule.Names, "clone3") && rule.Action == "SCMP_ACT_ERRNO" {
			t.Errorf("rule %d still has clone3 ERRNO restriction (action=%s)", i, rule.Action)
		}
	}
}

func TestUsernsProfile_OnlyChangedSyscalls(t *testing.T) {
	// Parse both the default and the userns profile.
	var defaultProfile struct {
		Syscalls []json.RawMessage `json:"syscalls"`
	}
	if err := json.Unmarshal(defaultProfileJSON, &defaultProfile); err != nil {
		t.Fatalf("unmarshal default: %v", err)
	}

	profileJSON, err := UsernsProfileJSON()
	if err != nil {
		t.Fatalf("UsernsProfileJSON(): %v", err)
	}
	var modifiedProfile struct {
		Syscalls []json.RawMessage `json:"syscalls"`
	}
	if err := json.Unmarshal([]byte(profileJSON), &modifiedProfile); err != nil {
		t.Fatalf("unmarshal modified: %v", err)
	}

	// Convert both to allow sets for comparison.
	defaultUncond := unconditionalAllowSet(t, string(defaultProfileJSON))
	modifiedUncond := unconditionalAllowSet(t, profileJSON)

	// The expected changes: the namespace syscalls moved from CAP_ADMIN-gated
	// to unconditional allow.
	for _, sc := range usernsRelaxedSyscalls {
		if !defaultUncond[sc] && !modifiedUncond[sc] {
			t.Errorf("expected syscall %q to be added to unconditional allow, but it is not present", sc)
		}
	}

	// No other syscalls should have changed from default to unconditional allow.
	for sc := range defaultUncond {
		if !modifiedUncond[sc] {
			t.Errorf("syscall %q was in default unconditional allow but is missing from modified", sc)
		}
	}
	for sc := range modifiedUncond {
		if !defaultUncond[sc] && !slices.Contains(usernsRelaxedSyscalls, sc) {
			t.Errorf("syscall %q was added to unconditional allow but is not in usernsRelaxedSyscalls", sc)
		}
	}
}
