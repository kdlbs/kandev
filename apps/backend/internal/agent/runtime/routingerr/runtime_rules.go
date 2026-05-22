package routingerr

import "regexp"

// runtimeEnvironmentRules match failures caused by the local execution
// environment (npm/npx cache state, missing binaries' install steps,
// etc.) rather than by the provider or remote API. They are tried by
// Classify after provider-specific rules and before the phase-based
// fallback, so a specific environment fingerprint wins over the
// low-confidence "phase.prestart.unknown" verdict.
//
// Each entry pairs a regex matcher with a builder that can extract
// signal-specific metadata (e.g. RemediationPath) from the raw text.
var runtimeEnvironmentRules = []runtimeRule{
	{
		id:      "npm.enotempty.npx.v1",
		pattern: regexp.MustCompile(`(?s)npm error code ENOTEMPTY.*?_npx/[0-9a-f]+`),
		build: func(text string) *Error {
			path := extractNpxCachePath(text)
			if path == "" {
				return nil
			}
			return &Error{
				Code:            CodeNpxCacheCorrupted,
				Confidence:      ConfHigh,
				RemediationPath: path,
			}
		},
	},
}

type runtimeRule struct {
	id      string
	pattern *regexp.Regexp
	build   func(text string) *Error
}

// npxCachePathRe captures the cache root, e.g.
// `/Users/cfl/.npm/_npx/d820eb7d96bc2600` from any npm error line that
// references a file beneath it. The hash segment is hex (npm uses an
// 8-byte hex of the package spec). `[^\n]*?` lets the home prefix
// contain spaces (e.g. `/Users/John Doe/...`); RemediateNpxCache's
// `EvalSymlinks` + `$HOME/.npm/_npx/` prefix guard validates the path
// before any deletion, so we don't need the regex to do that work too.
var npxCachePathRe = regexp.MustCompile(`(/[^\n]*?/\.npm/_npx/[0-9a-f]+)`)

func extractNpxCachePath(text string) string {
	m := npxCachePathRe.FindStringSubmatch(text)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func matchRuntimeEnvironmentRules(text string) (*Error, bool) {
	if text == "" {
		return nil, false
	}
	for _, r := range runtimeEnvironmentRules {
		if !r.pattern.MatchString(text) {
			continue
		}
		if e := r.build(text); e != nil {
			e.ClassifierRule = r.id
			return e, true
		}
	}
	return nil, false
}
