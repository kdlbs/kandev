---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
created: 2026-09-11
updated: 2026-09-17
owners:
  - Kandev
---
# Provider Error Recovery System Design Part 4

## Purpose and boundaries

Part 1 defines the dynamic-routing continuation package's data model
(`## Data model`). This part carries one subsection relocated from Part 1 when
Part 1 reached its file-size limit, and extended since: the sanitization
tiers `routingerr` applies to the continuation package's carrier text before
persistence, and to the primary launch prompt `ContinuationPrompt` renders
ahead of that package at each downstream launch.

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

The primary launch prompt (`ConductorLaunch.Prompt`, rendered by
`ContinuationPrompt` ahead of the continuation package on every attempt,
including attempt 0, not only fallbacks) is the composed first-turn prompt
built by `orchestrator.Service` (`task_operations.go`): the user's own text
plus server-injected context — the `<kandev-system>` block carrying the
task/session IDs and the MCP tool list, and, for config-mode sessions,
injected config context. `Message.ToAPI` strips the `<kandev-system>` block
before it reaches the UI bubble, so this is not simply text already shown to
the user unredacted. It carries the same kind of long identifiers as
`TaskDescription`/`PlanSummary`/`RepositorySummary` above (a file path, a
commit SHA, a PR URL, a task UUID) plus the task/session UUIDs the
`<kandev-system>` block itself injects, so it receives the same narrow
credential-only tier, via `routingerr.SanitizeCredentialsUnbounded` rather
than `SanitizeCredentials` — unbounded, because unlike the continuation
fields it is not subject to `continuationFieldLimit` and must not be
silently truncated. This is required for correctness, not only fidelity: the
diagnostic tier's 32-plus-character catch-all would mangle the injected
task/session UUIDs and break the fallback provider's MCP tool calls, which
address the task by that UUID. Diagnostic-tier redaction (`routingerr.Sanitize`)
is reserved for provider output and `FailureReason`, never for this field.

#### Deferred provider prompt budget

This contract intentionally does not set a byte cap for `ConductorLaunch.Prompt`.
The direct message API limits one message to `MaxRenderedPromptBytes`, but the
orchestrator adds workflow, plan, repository, configuration, and
`<kandev-system>` context. Other launch paths can also provide a prompt. A
future change must define an explicit token or byte budget for the complete
composed prompt and the behavior when that budget is exceeded, such as
compaction or a user-visible rejection. The budget must come from provider
capability metadata, not from a provider-name branch in the routing conductor.
Until that contract exists, provider context-window errors remain the
downstream safety boundary. An arbitrary cap here could silently remove
instructions or identifiers and would reintroduce the defect this design fixes.

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
