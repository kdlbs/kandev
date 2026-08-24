// Package seccomp provides the Kandev user-namespace seccomp profile.
//
// The base profile (default.json) is vendored verbatim from
// https://github.com/moby/profiles at commit v29.7.1
// (https://raw.githubusercontent.com/moby/profiles/main/seccomp/default.json).
//
// UsernsProfileJSON returns a modified copy that relaxes the namespace-related
// syscall restrictions Docker's default profile imposes on processes without
// CAP_SYS_ADMIN. Specifically:
//
//   - clone: the SCMP_CMP_MASKED_EQ restriction on CLONE_NEWUSER and related
//     namespace flags is removed; clone is allowed unconditionally.
//   - clone3: the SCMP_ACT_ERRNO (ENOSYS) fallback is removed; clone3 is
//     allowed unconditionally.
//   - mount, mount_setattr, move_mount, open_tree, umount, umount2, setns,
//     unshare: moved out of the CAP_SYS_ADMIN-gated group into the
//     unconditional allow list.
//   - pivot_root: added to the unconditional allow list. Docker's default
//     profile does not list it at all, so it falls through to the profile's
//     SCMP_ACT_ERRNO default. bwrap calls pivot_root after creating its
//     namespace and aborts when it fails, so relaxing clone/unshare alone is
//     not enough to make user-namespace sandboxing work.
//
// The only changes are the namespace-related syscalls listed above. Every
// other syscall restriction in Docker's default profile is preserved.
package seccomp

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"

	_ "embed"
)

//go:embed default.json
var defaultProfileJSON []byte

const (
	seccompActionAllow = "SCMP_ACT_ALLOW"
	seccompActionErrno = "SCMP_ACT_ERRNO"
)

// usernsRelaxedSyscalls lists the syscalls that the Kandev userns profile
// allows unconditionally. Docker's default profile either gates them behind
// CAP_SYS_ADMIN or (pivot_root) omits them entirely. Each of these is safe
// to allow because it remains capability-gated by the kernel — seccomp merely
// no longer pre-empts the kernel check. Processes without CAP_SYS_ADMIN can create user namespaces
// and perform mounts inside them, but gain no host-level privilege.
var usernsRelaxedSyscalls = []string{
	"clone",
	"clone3",
	"mount",
	"mount_setattr",
	"move_mount",
	"open_tree",
	"pivot_root",
	"setns",
	"umount",
	"umount2",
	"unshare",
}

// seccompRule models one entry in the "syscalls" array of a Docker seccomp
// profile. It contains only the fields we need to inspect and mutate.
type seccompRule struct {
	Names    []string          `json:"names"`
	Action   string            `json:"action"`
	Comment  string            `json:"comment,omitempty"`
	ErrnoRet *uint             `json:"errnoRet,omitempty"`
	Args     []json.RawMessage `json:"args,omitempty"`
	Includes *struct {
		Caps      []string `json:"caps,omitempty"`
		Arches    []string `json:"arches,omitempty"`
		MinKernel string   `json:"minKernel,omitempty"`
	} `json:"includes,omitempty"`
	Excludes *struct {
		Caps   []string `json:"caps,omitempty"`
		Arches []string `json:"arches,omitempty"`
	} `json:"excludes,omitempty"`
}

// seccompProfile is the top-level Docker seccomp profile structure.
type seccompProfile struct {
	DefaultAction   string            `json:"defaultAction"`
	DefaultErrnoRet *uint             `json:"defaultErrnoRet,omitempty"`
	ArchMap         []json.RawMessage `json:"archMap,omitempty"`
	Syscalls        []json.RawMessage `json:"syscalls"`
}

// UsernsProfileJSON returns the modified seccomp profile as a JSON string
// suitable for passing as a Docker SecurityOpt value ("seccomp=<json>").
func UsernsProfileJSON() (string, error) {
	var profile seccompProfile
	if err := json.Unmarshal(defaultProfileJSON, &profile); err != nil {
		return "", fmt.Errorf("seccomp: unmarshal default profile: %w", err)
	}

	processedRules, err := processSyscallRules(profile.Syscalls)
	if err != nil {
		return "", err
	}

	// Merge every relaxed namespace syscall into the unconditional allow list.
	// Seeding with the full list (rather than only the names lifted out of the
	// CAP_SYS_ADMIN group) is what covers syscalls the base profile never
	// mentions, such as pivot_root.
	finalSyscalls, err := mergeUnconditionalAllow(processedRules, usernsRelaxedSyscalls)
	if err != nil {
		return "", err
	}
	profile.Syscalls = finalSyscalls

	out, err := json.Marshal(&profile)
	if err != nil {
		return "", fmt.Errorf("seccomp: marshal userns profile: %w", err)
	}
	return string(out), nil
}

// processSyscallRules returns a transformed copy of the profile's syscall
// rules. It drops the clone MASKED_EQ and clone3 ERRNO rules, and strips the
// relaxed namespace syscalls out of the CAP_SYS_ADMIN-gated rules.
func processSyscallRules(rules []json.RawMessage) ([]json.RawMessage, error) {
	processed := make([]json.RawMessage, 0, len(rules))
	for i, raw := range rules {
		var rule seccompRule
		if err := json.Unmarshal(raw, &rule); err != nil {
			return nil, fmt.Errorf("seccomp: unmarshal rule %d: %w", i, err)
		}

		// Drop SCMP_CMP_MASKED_EQ clone rule for non-CAP_SYS_ADMIN.
		if ruleHasName(rule, "clone") && hasMaskedEqArg(rule) {
			continue
		}

		// Drop clone3 ENOSYS fallback rule.
		if ruleHasName(rule, "clone3") && rule.Action == seccompActionErrno {
			continue
		}

		// CAP_SYS_ADMIN-gated group — split out namespace syscalls.
		if rule.Action == seccompActionAllow && hasCap(rule, "CAP_SYS_ADMIN") {
			var remaining []string
			for _, name := range rule.Names {
				if !slices.Contains(usernsRelaxedSyscalls, name) {
					remaining = append(remaining, name)
				}
			}
			if len(remaining) > 0 {
				sort.Strings(remaining)
				processed = append(processed, makeRule(remaining, seccompActionAllow, capInclude("CAP_SYS_ADMIN")))
			}
			continue
		}

		processed = append(processed, slices.Clone(raw))
	}
	return processed, nil
}

// mergeUnconditionalAllow adds the namespace syscalls to the first
// unconditional allow rule, or prepends a new one. It never modifies rules.
func mergeUnconditionalAllow(rules []json.RawMessage, addedSyscalls []string) ([]json.RawMessage, error) {
	final := make([]json.RawMessage, len(rules))
	for i, raw := range rules {
		final[i] = slices.Clone(raw)
	}
	if len(addedSyscalls) == 0 {
		return final, nil
	}
	addedSyscalls = slices.Clone(addedSyscalls)
	sort.Strings(addedSyscalls)
	for i, raw := range final {
		var rule seccompRule
		if err := json.Unmarshal(raw, &rule); err != nil {
			return nil, fmt.Errorf("seccomp: unmarshal rule %d: %w", i, err)
		}
		if isUnconditionalAllowRule(rule) {
			allNames := make([]string, 0, len(rule.Names)+len(addedSyscalls))
			allNames = append(allNames, rule.Names...)
			allNames = append(allNames, addedSyscalls...)
			sort.Strings(allNames)
			allNames = slices.Compact(allNames)
			final[i] = makeRule(allNames, seccompActionAllow, nil)
			return final, nil
		}
	}
	newRule := makeRule(addedSyscalls, seccompActionAllow, nil)
	return append([]json.RawMessage{newRule}, final...), nil
}

func isUnconditionalAllowRule(rule seccompRule) bool {
	return rule.Action == seccompActionAllow &&
		rule.Includes == nil && rule.Excludes == nil &&
		len(rule.Args) == 0 && len(rule.Names) > 0
}

func ruleHasName(rule seccompRule, name string) bool {
	return slices.Contains(rule.Names, name)
}

func makeRule(names []string, action string, includes *struct {
	Caps      []string `json:"caps,omitempty"`
	Arches    []string `json:"arches,omitempty"`
	MinKernel string   `json:"minKernel,omitempty"`
}) json.RawMessage {
	v := map[string]any{
		"names":  names,
		"action": action,
	}
	if includes != nil {
		v["includes"] = includes
	}
	raw, _ := json.Marshal(v)
	return raw
}

func capInclude(cap string) *struct {
	Caps      []string `json:"caps,omitempty"`
	Arches    []string `json:"arches,omitempty"`
	MinKernel string   `json:"minKernel,omitempty"`
} {
	return &struct {
		Caps      []string `json:"caps,omitempty"`
		Arches    []string `json:"arches,omitempty"`
		MinKernel string   `json:"minKernel,omitempty"`
	}{Caps: []string{cap}}
}

func hasCap(rule seccompRule, cap string) bool {
	if rule.Includes == nil || len(rule.Includes.Caps) == 0 {
		return false
	}
	return slices.Contains(rule.Includes.Caps, cap)
}

func hasMaskedEqArg(rule seccompRule) bool {
	if len(rule.Args) == 0 {
		return false
	}
	for _, raw := range rule.Args {
		var arg struct {
			Op string `json:"op"`
		}
		if json.Unmarshal(raw, &arg) == nil && arg.Op == "SCMP_CMP_MASKED_EQ" {
			return true
		}
	}
	return false
}
