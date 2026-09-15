---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
created: 2026-09-11
updated: 2026-09-11
owners:
  - Kandev
---
# Provider Error Recovery System Design Part 4

## Purpose and boundaries

Part 1 defines the dynamic-routing continuation package's data model
(`## Data model`). This part carries one subsection relocated from Part 1
verbatim, unchanged, when Part 1 reached its file-size limit: the two
sanitization tiers `routingerr` applies to the continuation package's carrier
text before persistence.

It does not change classification rules, policy values, retry ownership, or
candidate ordering.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001` | [Continuation package sanitization tiers](#continuation-package-sanitization-tiers) |

### Continuation package sanitization tiers

The dynamic-routing continuation package (`dynamic.BuildBoundedContinuation`,
persisted as `dynamic_route_states.continuation_json` and rendered into the
successor's prompt by `ContinuationPrompt`) mixes two kinds of carrier text,
and `routingerr` sanitizes them with two different rule sets:

- **Provider diagnostics and the full `Conversation`** (`ToolSummary`,
  `FailureReason`, and both the user and agent halves of `Conversation`) run
  through `routingerr.Sanitize`, the full rule set. In addition to credential
  patterns it collapses any 32-plus-character run, rewrites URLs down to
  scheme and host, and normalizes home paths. `Conversation` is sanitized
  before `sanitizedTail` applies its byte budget, so an identifier cannot
  split a credential rule at the retained-tail boundary.
- **User/agent-authored carrier text** (`TaskDescription`, `PlanSummary`, and
  `RepositorySummary`) runs through `routingerr.SanitizeCredentials`, a
  narrower tier covering only credential-shaped patterns (`sk-`, `ghp_`,
  `github_pat_`, `kandev_pat_`, `Bearer`, `Authorization:`, `--api-key`,
  `password|secret|token|api_key` assignments, and URL userinfo). Each field is
  bounded to `continuationFieldLimit` after redaction, with the head retained.
  The narrow tier excludes
  the 32-plus-char catch-all, the scheme-and-host URL rewrite, and home-path
  normalization, because this text is already shown to the user unredacted
  and commonly carries legitimate long identifiers — commit SHAs, UUIDs, file
  hashes — that the full rule set would render as `***`.

Both tiers close the credential shapes they explicitly pattern-match: a `sk-`
key, a GitHub/Kandev PAT, a bearer/auth header, a `--api-key` flag, a
`password`/`secret`/`token`/`api_key` assignment — including a qualified
env-var key where the keyword is a prefix or suffix rather than the whole
name (`AWS_SECRET_ACCESS_KEY=`, `SECRET_KEY=`), and including a value that
contains an embedded quote character — or a URL's `user:pass@` userinfo is
redacted before persistence or a cross-provider fallback, in either tier. A
quoted value not terminated on the same line (an embedded, unescaped newline
before the closing quote) is redacted only up to the line break — unchanged
from the plain-text matching this replaced. Neither tier is a general secret
scanner: a credential shaped like something not on that list (a
vendor-specific token prefix, for example) survives the narrow tier unless it
also matches a listed pattern.

The key match is a substring match, not exact-name, so it also matches a key
merely containing a keyword without naming a credential (`max_tokens`,
`tokenizer`); an exact-name allowlist would drop the qualified env-var keys
(`AWS_SECRET_ACCESS_KEY=`) this match exists to catch. A bare decimal integer
value is left untouched only for the explicit count-key allowlist
(`tokens`, `max_tokens`, `input_tokens`, `output_tokens`, `total_tokens`,
`prompt_tokens`, and `completion_tokens`). Numeric values under credential keys,
such as `password: 123456` or `api_key=123456`, are redacted like other
credential values. A non-numeric value (`tokenizer: cl100k_base`) is also
redacted.
