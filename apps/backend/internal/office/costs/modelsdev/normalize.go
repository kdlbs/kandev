// Package modelsdev implements model-id normalization + a lazy
// disk-cached pricing lookup against the public models.dev dataset.
//
// Normalize converts the wide-variety model id surface emitted by
// real ACP CLIs (probed in /tmp/acp-probe-*.jsonl) into a canonical
// lookup key, plus a Strategy that tells the caller whether to query
// the pricing dataset, skip the lookup entirely (logical aliases),
// or treat the row as estimated.
package modelsdev

import "strings"

// Strategy is the routing decision returned by Normalize.
type Strategy int

const (
	// StrategyLookup means the canonical key is a real model id that
	// the modelsdev.Client should query for pricing. Misses still fall
	// back to estimated.
	StrategyLookup Strategy = iota

	// StrategySkip is returned for claude-acp's logical aliases
	// (default / sonnet / haiku / opus, optionally with the [1m]
	// long-context suffix). Pricing is undefined for these names; the
	// subscriber must rely on Layer A (usage_update.cost.amount) for
	// claude-acp turns. Calling Lookup on a skipped key is undefined.
	StrategySkip

	// StrategyEstimated is returned for auto-router aliases and any
	// other shape that can't be canonicalised into a queryable id. The
	// subscriber records the row with cost_subcents=0, estimated=true.
	StrategyEstimated
)

// effortSuffixes catalogues the reasoning-effort segments observed at
// the end of codex-acp / opencode-acp model ids (e.g. "gpt-5.4/high",
// "github-copilot/claude-haiku-4.5/max"). Stripped at normalize time
// because pricing is per model, not per effort tier.
var effortSuffixes = map[string]struct{}{
	"high":    {},
	"low":     {},
	"medium":  {},
	"minimal": {},
	"xhigh":   {},
	"max":     {},
}

// knownRoutes lists the leading provider/route prefixes observed on
// opencode-acp model ids ("github-copilot/<model>", "openai/<model>").
// These describe how the call is *routed* through opencode's BYOK
// layer, not which model is billed; for Layer B pricing we strip them
// so the lookup hits the canonical model entry. Documented caveat:
// when usage_update.cost is absent (BYOK) the recorded cost reflects
// list price not what the wrapper actually billed.
var knownRoutes = map[string]struct{}{
	"github-copilot": {},
	"openai":         {},
	"anthropic":      {},
	"google":         {},
	"openrouter":     {},
}

// contextLengthSuffixes describes the trailing context-window flags
// seen on auggie model ids (e.g. "claude-opus-4-7-500k",
// "claude-sonnet-4-6-1m"). The base model id is the canonical lookup
// key; the context-length tier doesn't affect token pricing.
var contextLengthSuffixes = []string{"-200k", "-500k", "-1m"}

// Normalize converts a raw modelId into a canonical key plus a
// Strategy. See the package doc for the per-CLI shapes covered.
func Normalize(id string) (string, Strategy) {
	if id == "" {
		return "", StrategyEstimated
	}

	if isClaudeLogicalAlias(id) {
		return id, StrategySkip
	}

	if isRouterAlias(id) {
		return id, StrategyEstimated
	}

	id = stripEffortSuffix(id)
	id = stripRoutePrefix(id)
	id = stripContextLengthSuffix(id)

	return id, StrategyLookup
}

// isClaudeLogicalAlias matches the labels claude-acp emits on every
// frame: default / sonnet / haiku / opus, optionally with the [1m]
// long-context suffix. None of these are real model ids — claude-code
// owns the alias-to-real-model mapping internally and flips it without
// notice. Layer A (usage_update.cost.amount) is the only safe path.
func isClaudeLogicalAlias(id string) bool {
	base := strings.TrimSuffix(id, "[1m]")
	switch base {
	case "default", "sonnet", "haiku", "opus":
		return true
	}
	return false
}

// isRouterAlias catches "auto" and the gemini "auto-gemini-3" /
// "auto-gemini-2.5" router shorthands. Router resolution happens
// upstream; we can't pre-resolve them client-side.
func isRouterAlias(id string) bool {
	if id == "auto" {
		return true
	}
	return strings.HasPrefix(id, "auto-")
}

// stripEffortSuffix removes a trailing /<effort> segment when the
// effort label is one of the known reasoning tiers. Examples:
//
//	gpt-5.4-mini/medium  -> gpt-5.4-mini
//	github-copilot/claude-haiku-4.5/high -> github-copilot/claude-haiku-4.5
//
// Anything that doesn't match a known effort word is left alone — some
// canonical ids embed slashes legitimately.
func stripEffortSuffix(id string) string {
	i := strings.LastIndex(id, "/")
	if i <= 0 || i == len(id)-1 {
		return id
	}
	suffix := id[i+1:]
	if _, ok := effortSuffixes[suffix]; ok {
		return id[:i]
	}
	return id
}

// stripRoutePrefix removes a leading <route>/ prefix when the route is
// one of opencode-acp's known carriers (github-copilot, openai,
// anthropic, google, openrouter). Leaves slashes inside canonical ids
// alone — only the first segment is checked.
func stripRoutePrefix(id string) string {
	i := strings.Index(id, "/")
	if i <= 0 {
		return id
	}
	prefix := id[:i]
	if _, ok := knownRoutes[prefix]; ok {
		return id[i+1:]
	}
	return id
}

// stripContextLengthSuffix removes the trailing context-window tier
// flags seen on auggie ids (-200k, -500k, -1m). The base model id is
// what models.dev keys on.
func stripContextLengthSuffix(id string) string {
	for _, s := range contextLengthSuffixes {
		if strings.HasSuffix(id, s) {
			return strings.TrimSuffix(id, s)
		}
	}
	return id
}
