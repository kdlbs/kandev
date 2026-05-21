package share

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Rule names recorded in Snapshot.Redaction.AppliedRules.
const (
	RuleAbsPath         = "abs-path"
	RuleSecretSK        = "secret-sk"
	RuleSecretGHP       = "secret-ghp"
	RuleSecretGHO       = "secret-gho"
	RuleSecretGitHubPAT = "secret-github-pat"
	RuleSecretAWS       = "secret-aws"
	RuleEnvFile         = "env-file"
	RuleEnvVars         = "env-vars"

	redactedPlaceholder = "[redacted]"
	envFilePlaceholder  = "[redacted: .env contents]"
)

// secretRule pairs a regex with the rule name recorded when it fires.
type secretRule struct {
	name string
	re   *regexp.Regexp
}

// secretRules covers the obvious "looks like a credential" shapes we strip
// from any user-visible string in the snapshot. These are intentionally
// narrow — they catch unambiguous formats without trying to find every
// possible secret. Adding new rules: keep them anchored and high-precision.
var secretRules = []secretRule{
	{RuleSecretSK, regexp.MustCompile(`sk-[a-zA-Z0-9]{20,}`)},
	{RuleSecretGHP, regexp.MustCompile(`ghp_[A-Za-z0-9]{36,}`)},
	{RuleSecretGHO, regexp.MustCompile(`gho_[A-Za-z0-9]{36,}`)},
	{RuleSecretGitHubPAT, regexp.MustCompile(`github_pat_[A-Za-z0-9_]{36,}`)},
	{RuleSecretAWS, regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
}

// envFileRe matches paths that look like a dotenv file: ".env", ".env.local",
// ".env.production", etc. Used by RedactToolResult to scrub file contents.
var envFileRe = regexp.MustCompile(`(^|/)\.env(\..+)?$`)

// Redactor applies the snapshot redaction rules and records which ones fired.
// Construct with NewRedactor; reuse across one snapshot build (it accumulates
// the applied-rule set).
type Redactor struct {
	workspaceRoot string
	applied       map[string]struct{}
}

// NewRedactor returns a Redactor that rewrites absolute paths under
// workspaceRoot to repo-relative paths. workspaceRoot may be empty, in
// which case the abs-path rule is skipped.
func NewRedactor(workspaceRoot string) *Redactor {
	return &Redactor{
		workspaceRoot: strings.TrimRight(workspaceRoot, "/"),
		applied:       make(map[string]struct{}),
	}
}

// String applies the secret and absolute-path rules to s and returns the
// redacted result. Safe for nil receivers (returns s unchanged).
func (r *Redactor) String(s string) string {
	if r == nil || s == "" {
		return s
	}
	out := s
	for _, rule := range secretRules {
		if rule.re.MatchString(out) {
			out = rule.re.ReplaceAllString(out, redactedPlaceholder)
			r.record(rule.name)
		}
	}
	if r.workspaceRoot != "" && strings.Contains(out, r.workspaceRoot) {
		// Replace every occurrence of the workspace root with a relative form.
		// We split on the root and rejoin so each suffix has its leading slash
		// trimmed independently.
		parts := strings.Split(out, r.workspaceRoot)
		var b strings.Builder
		b.WriteString(parts[0])
		changed := false
		for _, part := range parts[1:] {
			changed = true
			b.WriteString(strings.TrimPrefix(part, "/"))
		}
		if changed {
			out = b.String()
			r.record(RuleAbsPath)
		}
	}
	return out
}

// JSON walks raw, applies String() to every string leaf, and drops any
// top-level "env" field (RuleEnvVars). Returns the re-marshaled bytes.
// If raw is not valid JSON, it is passed through String() as a fallback.
func (r *Redactor) JSON(raw json.RawMessage) json.RawMessage {
	if r == nil || len(raw) == 0 {
		return raw
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return json.RawMessage(r.String(string(raw)))
	}
	v = r.walk(v, true)
	out, err := json.Marshal(v)
	if err != nil {
		return raw
	}
	return out
}

// walk recursively redacts every string leaf in v. isRoot controls whether
// the "env" key is dropped — we only drop env at the top level of a tool-call
// args object so we don't accidentally erase legitimate nested fields named
// "env" deep in a payload.
func (r *Redactor) walk(v any, isRoot bool) any {
	switch t := v.(type) {
	case string:
		return r.String(t)
	case []any:
		for i, item := range t {
			t[i] = r.walk(item, false)
		}
		return t
	case map[string]any:
		if isRoot {
			if _, ok := t["env"]; ok {
				delete(t, "env")
				r.record(RuleEnvVars)
			}
		}
		for k, item := range t {
			t[k] = r.walk(item, false)
		}
		return t
	}
	return v
}

// RedactToolResult scrubs a tool-call result string. If the paired tool-call
// args reference a .env-style file, the entire output is replaced with a
// placeholder and the env-file rule is recorded.
func (r *Redactor) RedactToolResult(output string, argPath string) string {
	if r == nil {
		return output
	}
	if argPath != "" && envFileRe.MatchString(filepath.ToSlash(argPath)) {
		r.record(RuleEnvFile)
		return envFilePlaceholder
	}
	return r.String(output)
}

// Applied returns the set of redaction rules that fired, sorted lexically
// so the snapshot output is deterministic.
func (r *Redactor) Applied() []string {
	if r == nil || len(r.applied) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(r.applied))
	for k := range r.applied {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (r *Redactor) record(name string) {
	if r == nil {
		return
	}
	r.applied[name] = struct{}{}
}
