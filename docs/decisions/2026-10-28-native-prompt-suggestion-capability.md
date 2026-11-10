# ADR-2026-10-28-native-prompt-suggestion-capability: Normalize native suggestion support at the adapter

**Status:** accepted
**Date:** 2026-10-28
**Area:** protocol

## Context

The landed suggestion feature equates an object at `_meta.claudeCode` with
support. That object currently advertises prompt queueing, which is a separate
contract. The adapter also combines support and the launch request into
`supports_prompt_suggestions`. Existing consumers rely on that effective value.

The pinned Claude ACP bridge supports the required options and raw notification.
It does not advertise this feature explicitly. Requiring an advertisement
without compatibility handling would disable the working pinned integration.
Continuing to accept a namespace would admit unrelated bridges.

## Decision

Agents owns one generic native-suggestion capability. The adapter determines
provider support before it combines support with the launch request.
Lifecycle, orchestration, and composer consumers remain provider-neutral.

Keep `supports_prompt_suggestions` as the effective session-enablement value.
Add the optional, support-only `native_prompt_suggestions_supported` field.
Absence means unknown for an older producer. It must not reinterpret the legacy
effective value.

Use two bounded Claude integration inputs:

1. A new, explicitly documented vendor extension,
   `agentCapabilities._meta.claudeCode.promptSuggestions: boolean`.
   Literal `true` declares the complete existing options/raw-notification
   contract. Literal `false` or a malformed present value prevents fallback.
2. When that field is absent, retain a compatibility exception for the exact
   official package identity and verified version `0.81.2`. Require its observed
   well-formed `claudeCode.promptQueueing: true` shape too. Queueing supplies no
   suggestion authority independently of that verified identity/version pair.

The extension is a Kandev opt-in contract introduced by this package. Neither
inspected upstream release advertises it. It is not a standard ACP capability.
Custom bridges must adopt the documented contract before their explicit
declaration qualifies.
Kandev-defined names and profiles remain unrestricted. A renamed wire bridge
with no explicit declaration cannot establish support from a namespace alone.

Do not infer a minimum version or an unbounded version range from one verified
release. Do not add a provider registry. A future bridge update must either
advertise the feature or supply newly verified compatibility evidence.

Keep Claude evidence parsing, session metadata construction, and notification
decoding inside the ACP integration. Use small pure functions under
`dialect_claude_prompt_suggestions.go`. Shared adapter code retains session
state, transport calls, and originating-generation validation under ADR 0044.

## Consequences

The pinned integration keeps native support. Arbitrarily named bridges can
declare the same contract without identity matching. Other providers can
supply the generic support/event contract through their adapters later.

Compatibility remains deliberately limited. Unadvertised renamed bridges and
unverified versions become unsupported until evidence exists. The bridge
identity/version is a compatibility assertion, not cryptographic attestation.
The SDK or account can still suppress suggestions.

Upstream work must add and test the feature advertisement. No upstream issue
or PR exists as a result of this design turn. Kandev delivery does not depend on
that publication because the verified pin has the narrow exception.

The accepted composer interaction, preference, scope, and origin-window rules
remain intact. There is no new process, model request, persistence schema, or
runtime feature flag.

## Alternatives Considered

- **Namespace or queueing alone:** admits agents that do not implement suggestions.
- **Agent/model name matching in core:** spreads protocol assumptions and excludes custom bridges.
- **Disable every unadvertised bridge:** removes verified Claude support.
- **Assume a new standard capability exists:** invents interoperability that providers do not implement.
- **Recognize every later version:** assumes compatible options, defaults, and notifications without evidence.
- **Probe by generating a suggestion:** costs a model turn, changes session context, and cannot distinguish suppression from lack of support.

## References

- [Agents requirements](../specs/agents/requirements/native-prompt-suggestion-capability.md)
- [Agents design and primary evidence](../specs/agents/system-design/native-prompt-suggestion-capability.md#primary-evidence)
- [ACP compatibility dialects](0044-acp-agent-compatibility-dialects.md)
- [Origin identity](0035-version-agent-ready-events-by-prompt-generation.md)
- [Implementation plan](../plans/native-prompt-suggestion-capability/plan.md)

