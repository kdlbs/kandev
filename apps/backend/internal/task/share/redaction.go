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
	// roots are the workspace prefixes to rewrite, sorted longest-first so a
	// nested root (e.g. "/workspace/repo1") gets matched before its parent.
	// rootPatterns is the parallel slice of compiled matchers.
	roots        []string
	rootPatterns []*regexp.Regexp
	applied      map[string]struct{}
}

// rootBoundary matches the characters that may follow a workspace root WITHOUT
// extending it into a sibling path. We accept:
//   - "/" — path separator (the common "rewrite to repo-relative" case)
//   - end-of-string anchor `$` — bare root at the tail of a string
//   - whitespace — bare root in the middle of prose, `pwd` output, …
//
// We explicitly do NOT use `\b` here: `\b` fires on any word↔non-word transition
// so "/workspace.bak" or "/workspace-old" would falsely match. Restricting to
// `/`, EOS, or whitespace keeps every realistic case while never corrupting
// sibling paths that differ only in punctuation.
const rootBoundary = `(/|\s|$)`

// NewRedactor returns a Redactor that rewrites absolute paths under any of
// workspaceRoots to repo-relative paths. Passing no roots (or only empty
// strings) skips the abs-path rule. Multiple roots are supported so multi-repo
// sessions don't leak paths from secondary worktrees.
func NewRedactor(workspaceRoots ...string) *Redactor {
	roots := make([]string, 0, len(workspaceRoots))
	for _, root := range workspaceRoots {
		trimmed := strings.TrimRight(root, "/")
		if trimmed != "" {
			roots = append(roots, trimmed)
		}
	}
	sort.Slice(roots, func(i, j int) bool {
		return len(roots[i]) > len(roots[j])
	})
	patterns := make([]*regexp.Regexp, 0, len(roots))
	for _, root := range roots {
		patterns = append(patterns, regexp.MustCompile(regexp.QuoteMeta(root)+rootBoundary))
	}
	return &Redactor{
		roots:        roots,
		rootPatterns: patterns,
		applied:      make(map[string]struct{}),
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
	for i, re := range r.rootPatterns {
		if !re.MatchString(out) {
			continue
		}
		// Strip root + path separator entirely so paths become repo-relative;
		// for whitespace/EOS boundaries, preserve the boundary char so prose
		// like "cd /workspace && ls" loses only the root, not the surrounding
		// space.
		rootLen := len(r.roots[i])
		out = re.ReplaceAllStringFunc(out, func(match string) string {
			boundary := match[rootLen:]
			if boundary == "/" {
				return ""
			}
			return boundary
		})
		r.record(RuleAbsPath)
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
