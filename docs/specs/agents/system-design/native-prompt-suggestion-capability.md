---
status: current
system: agents
requirements:
  - REQ-AGENTS-NATIVE-SUGGEST-001
  - REQ-AGENTS-NATIVE-SUGGEST-002
  - REQ-AGENTS-NATIVE-SUGGEST-003
---

# Native Prompt Suggestion Capability System Design

## Purpose and boundaries

Agents owns the provider support contract and its normalization at the adapter.
This design covers the launch request, stream projection, and consumer boundaries.
The [UI design](../../ui/system-design/composer-prompt-suggestions.md) owns the
existing composer interaction. Tasks retains session and turn authority.

This design records the implemented capability and its evidence limits. The
[work package](../../../plans/native-prompt-suggestion-capability/plan.md)
records delivery and validation results.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-AGENTS-NATIVE-SUGGEST-001` | Primary evidence; Provider evidence; Compatibility limits |
| `REQ-AGENTS-NATIVE-SUGGEST-002` | Generic contract; Producers and consumers; Session configuration |
| `REQ-AGENTS-NATIVE-SUGGEST-003` | Connection and session continuity; Notification flow; Preserved correctness |

## Landed baseline and current data flow

Investigation started from clean local and remote main at
`3f3aabc53f23cc8b08f0bf2b1b766b88fbeaf7f2` on 2026-10-28.
[PR #4306](https://github.com/kdlbs/kandev/pull/4306) landed as
`7351a48f44f996ea2c7dd91185ed889040323626`.
Its reviewed head was `abb7f0047fcec9a2762ed969bd12506c93d0eb68`.

The actual source confirms this flow:

1. `lifecycle/prompt_suggestions.go:promptSuggestionsForLaunch` derives a
   request from the user preference and interactive MCP scope. Fresh launch
   (`manager_launch.go`) and recreation (`manager_execution.go`) both call it.
2. Executor and instance configuration carry `prompt_suggestions` through
   `shared.Config.PromptSuggestions`. This is a request, not provider evidence.
3. `acp/adapter.go:Connect` receives `initialize`. Today
   `agentAdvertisesClaudeCode` accepts any object at `agentCapabilities._meta.claudeCode`.
   It caches `claudeCodeAgent` and emits
   `SupportsPromptSuggestions = cfg.PromptSuggestions && claudeCodeAgent`.
4. `adapter_session.go` builds metadata for `session/new` and `session/load`.
   `adapter_session_resume.go` passes the same metadata to `session/resume`,
   including its advertised-method fallback to load.
5. `adapter_prompt_suggestion.go` decodes `_claude/sdkMessage` with
   `message.type == "prompt_suggestion"`. An eligible origin window supplies
   `PromptGeneration` to the generic `prompt_suggestion` stream event.
6. `lifecycle/manager_events.go:handleAgentEventState` resolves that generation
   to the originating `TurnID`. Durable delivery uses the same execution mapping.
   `lifecycle/events.go` projects stream capability fields.
7. `orchestrator/event_handlers_streaming.go:handleAgentCapabilitiesEvent`
   persists effective support as `prompt_suggestion_source` and broadcasts
   `session.agent_capabilities`. `event_handlers_prompt_suggestion.go` checks the
   originating turn, conditionally stores the suggestion, and publishes
   `session.prompt_suggestion`.
8. `lib/ws/handlers/agent-capabilities.ts` mirrors the effective source marker.
   `lib/prompt-suggestion.ts` and `hooks/use-prompt-suggestion.ts` select eligible
   ghost text without requesting generation. The shared composer handles keys,
   touch acceptance, and submission.

The coupling is in step 3. Steps 6 through 8 already consume generic values.
The misleading field name must not cause a change in its existing meaning.

## Primary evidence

All external evidence was inspected on 2026-10-28.

| Source | Observed contract |
| --- | --- |
| [ACP v1 initialization](https://agentclientprotocol.com/protocol/v1/initialization) and [extensibility](https://agentclientprotocol.com/protocol/v1/extensibility) | ACP provides capabilities and vendor metadata. Neither establishes a standard native-suggestion capability. |
| [Pinned bridge source at 5dbb453c](https://github.com/agentclientprotocol/claude-agent-acp/blob/5dbb453c63a89746627799b2b06b31ba01a1b674/src/acp-agent.ts) | `initialize` at lines 2332-2367 reports package identity/version and `claudeCode.promptQueueing`. No suggestion-support advertisement exists. `NewSessionMeta` at lines 1344-1383 accepts SDK options and raw-message filters. Lines 4250-4258 forward matching raw messages. |
| [Pinned bridge package](https://github.com/agentclientprotocol/claude-agent-acp/blob/5dbb453c63a89746627799b2b06b31ba01a1b674/package.json) | Bridge `0.81.2` depends on Claude Agent SDK `0.3.280`. Kandev pins this bridge in `managed_npm_runtime_versions.json`. |
| [Official SDK 0.3.280 artifact](https://www.npmjs.com/package/@anthropic-ai/claude-agent-sdk/v/0.3.280) | Published `sdk.d.ts`, lines 1988-2004 and 5398-5406, declares `Options.promptSuggestions` and `SDKPromptSuggestionMessage`. The message contains text and session identity, without an originating turn reference. |
| [Bridge 0.99.0 source at 106e3031](https://github.com/agentclientprotocol/claude-agent-acp/blob/106e3031e393daebc7170c3fe9ed46d0e0ee6c40/src/acp-agent.ts) and [its suggestion projection](https://github.com/agentclientprotocol/claude-agent-acp/blob/106e3031e393daebc7170c3fe9ed46d0e0ee6c40/src/prompt-suggestions.ts) | Still no feature advertisement. It additionally projects suggestions through session-info metadata and sets the SDK option on by default before client overrides. These changes do not establish a compatible version range. |
| [ACP Go SDK PR #6](https://github.com/kdlbs/acp-go-sdk/pull/6) | Merged at `48a57e4292e88991b6e6e22342b4dd5c901c1258`. Current `go.mod` already uses the fork revision with extension-notification routing. No SDK change is required. |

The pinned bridge includes `promptSuggestions` and raw filters in its session
fingerprint. Changed options on load/resume rebuild the SDK query when required.
The existing [Task 05 evidence](../../../plans/composer-prompt-suggestions/task-05-review-fixes.md#evidence-gathered)
records the earlier bounded native probe and its origin limits.

No new live model turn or ACP probe was necessary for these source-level facts.
This investigation does not claim fresh runtime latency or generation evidence.
The historical hosted run of 4,390 passes and 40 skips validates the original
feature only. It provides no validation of this follow-up.

## Provider evidence

### Explicit Claude extension proposed by this package

New opt-in contract:

```json
{
  "agentCapabilities": {
    "_meta": {
      "claudeCode": {
        "promptSuggestions": true
      }
    }
  }
}
```

This field does not exist in either inspected upstream release. It is a
provider-specific extension proposed for adoption, not an existing or standard
ACP capability. This package makes that absence explicit.

Literal `true` asserts the entire existing Claude integration contract:

- The bridge accepts `options.promptSuggestions` as a boolean on new/load/resume.
- `false` disables generation, including when reusing a warm session.
- The existing raw-message filter delivers `prompt_suggestion` through
  `_claude/sdkMessage` with the active outer `sessionId` and SDK text.
- Normal prompt responses and notification ordering retain their existing meaning.
- The bridge can produce no suggestion for a turn.

Literal `false` means unsupported. A present value of any other type is invalid.
Both cases reject support without a legacy fallback. Do not coerce strings,
numbers, empty objects, arrays, or null into booleans.

The configured Kandev agent ID and reported `agentInfo.name` have no effect on
this explicit path. A custom bridge called `research-bridge` can qualify.
The model has no effect either.

### Verified legacy compatibility

When the feature field is absent, a small Claude compatibility function returns
support only when all these observed facts match:

- `agentInfo.name == "@agentclientprotocol/claude-agent-acp"`;
- `agentInfo.version == "0.81.2"`;
- `agentCapabilities._meta.claudeCode` is a non-null object;
- its `promptQueueing` value is literal `true`.

The identity/version pair supplies compatibility evidence from the inspected
implementation. Queueing remains an independent feature. A queueing namespace
on any other identity or version fails this rule.

Require exact strings. Do not accept a version prefix, prerelease, build suffix,
unknown version, alias, or `>= 0.81.2` range. The rule is local to the Claude
integration, with no shared provider registry.

A custom Kandev definition that launches the pinned official bridge qualifies
because the bridge still reports its own implementation identity. A fork that
renames its wire identity must adopt the explicit extension. Its source can be
correct while its handshake remains insufficient for automatic detection.

### Evidence decision table

| Evidence | Provider support |
| --- | --- |
| Valid explicit `true`, arbitrary configured and reported names | true |
| Explicit `false`, including verified legacy identity/version | false |
| Present malformed feature field, including verified legacy identity/version | false |
| Absent feature field and complete verified legacy tuple | true |
| Absent metadata, empty namespace, wrong namespace type, or unrelated metadata | false |
| Only `promptQueueing: true` with an arbitrary identity | false |
| Claude-looking configured name or model, without sufficient wire evidence | false |
| Official bridge name with an unverified version and no declaration | false |

## Generic contract

Use two generic fields at the existing capability-event boundary:

| Field | Meaning | Compatibility |
| --- | --- | --- |
| `NativePromptSuggestionsSupported *bool` / `native_prompt_suggestions_supported` | Provider can implement the native protocol contract, independently of preference | New optional field. A new ACP producer emits explicit true or false. Nil/absence means unknown from an older producer or another transport. |
| `SupportsPromptSuggestions bool` / `supports_prompt_suggestions` | Effective launch/session enablement | Existing field and omission behavior remain unchanged. False or absence means disabled. |
| `PromptSuggestions` / `prompt_suggestions` on configuration | User preference and scope request | Existing field remains a request. It cannot prove provider support. |

Inside the adapter, cache a generic support boolean instead of `claudeCodeAgent`.
Compute effective enablement as `support && cfg.PromptSuggestions`.
A separate generic provider framework or state machine is unnecessary.

The optional pointer preserves the difference between a new explicit false and
an older event with no raw support field. Forward it without deriving enablement.
An older agentctl can still send effective true with unknown raw support.
An older backend or browser ignores the added field and retains existing behavior.

The existing effective event describes negotiated launch intent. It is not an
acknowledgement that the provider will emit a suggestion. Keep its initialize-time
publication. Session setup errors retain normal error and UI recovery blocking.

## Producers and consumers

Every affected producer or consumer is accounted for here:

| Boundary | Existing value | Planned change |
| --- | --- | --- |
| `server/adapter/transport/shared.Config` and executor/instance DTOs | Request | Retain existing request plumbing; no new preference or executor fields |
| `acp/adapter.go:Connect` | Produces effective capability | Normalize support once per connection; publish raw support plus existing effective value |
| `agentctl/types/streams/agent.go:AgentEvent` | Stream serialization | Add optional raw support field; correct effective-field comment |
| `lifecycle/event_types.go:AgentStreamEventData` | Runtime stream DTO | Carry optional raw support unchanged |
| `lifecycle/events.go` | Stream-to-runtime projection | Copy both fields without provider logic |
| `lifecycle/event_types.go:AgentCapabilitiesEventPayload` | Browser broadcast DTO | Add the same optional raw support field |
| `orchestrator/event_handlers_streaming.go` | Records source and broadcasts capability | Continue source writes from effective value only; forward raw support |
| `orchestrator/event_handlers_prompt_suggestion.go` | Reads effective source history and stores suggestions | Retain source semantics and state/turn checks |
| `apps/web/lib/types/session-runtime-payloads.ts` | Browser capability payload | Add optional support-only boolean; remove Claude-specific effective-field comment |
| `apps/web/lib/ws/handlers/agent-capabilities.ts` | Mirrors effective source | Keep behavior based solely on existing effective field |
| `apps/web/lib/prompt-suggestion.ts` and `hooks/use-prompt-suggestion.ts` | Composer selection | Keep existing generic source/preference rules |
| `cmd/mock-agent/main.go` | Synthetic initialize evidence | Advertise the new explicit feature independently from queueing |
| Existing capability/suggestion fixtures and tests | Namespace-based setup | Use explicit feature evidence or the exact legacy fixture, according to the case |

No raw support is persisted in session metadata. `prompt_suggestion_source`
continues to mean effective enablement, with `native` or `none`.
There is no schema migration and no boot-payload shape change.

## Adapter integration

Place pure Claude parsing, metadata construction, and SDK-message decoding in
`acp/dialect_claude_prompt_suggestions.go`, alongside `dialect_claude.go`.
These functions receive wire values and return normalized values or request data.
They do not receive `*Adapter` or perform RPCs.

Keep support selection independent of the existing `newACPDialect(agentID)`
identity switch. Selecting only the built-in Claude dialect would exclude custom
definitions. Call the small feature recognizer on the actual initialize response.

Keep generic enablement, active-session checks, generation windows, and event
publication in `adapter_prompt_suggestion.go` and the existing adapter lifecycle.
The ACP integration can dispatch its one implemented extension without a registry.

Another transport later supplies the same generic support/effective fields and
`prompt_suggestion` event. Its adapter must establish originating generation
from its protocol. It does not change composer business logic.

## Session configuration

| Support | Request | Effective | Session metadata |
| --- | --- | --- | --- |
| false | false or true | false | No suggestion metadata |
| true | false | false | `claudeCode.options.promptSuggestions: false`; no raw suggestion filter |
| true | true | true | `claudeCode.options.promptSuggestions: true` and `emitRawSDKMessages: [{type: "prompt_suggestion"}]` |

An explicit false is a disable instruction, not a generation request. It avoids
stale warm-session enablement and handles provider defaults within the known contract.
Unknown providers receive no unsupported configuration.

Apply the same builder to `session/new` and the existing load request.
`restoreSessionState` forwards it unchanged to advertised resume, or to load
after the existing method-not-found fallback. The live `ResumeSession` used for
idle MCP reconfiguration also carries it, so a rebuilt provider query keeps the
session's negotiated suggestion option. Keep serialization and restore
identity/error rules intact. No new fallback session or model request is added.

Launch and recreate continue to read the preference in
`promptSuggestionsForLaunch`. Task, task-title-pending, and empty MCP modes are
the existing eligible modes. Utility and passthrough paths do not opt in.
Do not add the feature to host model-capability probes.

## Connection and session continuity

A fresh connection computes support from its own initialize response.
Recreated executions reevaluate the preference and scope. A reset on an existing
connection uses that connection's negotiated support and current instance request.

Do not mutate a running provider on every preference save. The existing browser
preference hides ghost text immediately. Native settings follow the next launch
or resume. Existing recovery/adoption rules remain authoritative: retained
agentctl adoption does not re-run ACP initialize or load.

On support or request loss, emit effective false. Existing source recording
replaces an earlier `native` with `none`, even if false is omitted on the wire.
The browser mirrors that change. Stored text can remain because it is unusable
without the effective source and current turn.

Keep stale resume-attempt and execution filtering before capability effects.
A capability event from an obsolete execution must not restore the source.
On failed setup, existing session state/error/recovery checks block display.

Session replacements must not reuse a suggestion origin window from a different
ACP session. Preserve transition locking and prompt ownership invalidation.
Only a successful transition that produces a different provider session ID
proves a replacement and may discard the prior window. A failed transition or a
same-ID load/resume keeps the old conversation and its window, so the next
prompt's origin stays ambiguous. Cover successful new/load/reset boundaries,
failed and same-ID restores, and delayed old notifications. Do not change the
conservative overlap algorithm.

## Notification flow

1. Reject notifications when effective enablement is false.
2. Decode the Claude extension into normalized session ID and text.
3. Reject malformed, blank, unrelated, or foreign-session notifications.
4. Claim the existing suggestion window. Reject an active prompt, unknown
   generation, ambiguous predecessor, consumed window, or missing window.
5. Emit `prompt_suggestion` with its original `PromptGeneration`.
6. Resolve the original generation to `TurnID` in lifecycle, never the current turn.
7. Preserve orchestrator rejection and conditional storage/publication.
8. Preserve frontend source, preference, turn, and idle-draft selection.

Support alone never admits a notification. A provider can remain silent after
every eligible turn, without an error or a generated replacement.

## Preserved correctness

The existing adapter origin tests, lifecycle propagation tests, orchestrator
negative-write tests, and literal-text tests remain authoritative regressions.
Capability normalization must not weaken their assertions.

The orchestrator continues to reject unknown, foreign, or stale originating
turns and preserve `SetSessionMetadataKeyIfState(..., WAITING_FOR_INPUT)`.
It never substitutes the latest turn for an unknown origin.

The browser retains queue, active-turn, clarification, delivery-recovery, task-move,
editable-draft, and empty-draft guards. Accept/send retain literal angle brackets,
entities, and line breaks. Typing never requests suggestions.

## Desktop and phone behavior

No layout, control, navigation, copy, or keyboard change is planned.
The existing composer remains the nearest shipped mobile example:
`mobile-prompt-suggestions.spec.ts` proves the 44px Use reply control.

Use the existing task and Quick Chat composers, their existing scroll owners,
and their shared selection hook. Unsupported sessions retain the ordinary
placeholder and shortcuts. Existing desktop and phone E2E provide rendered proof;
no new preview or duplicate browser suite is required for data normalization.

## Compatibility limits and upstream work

The legacy exception preserves Kandev's current pin. It does not certify every
historical or later release. Bridge `0.99.0` is evidence of protocol/default drift,
not an approved runtime bump in this package.

To remove the identity/version exception, upstream must advertise the explicit
feature only when its options and notification contract are implemented.
Its tests must cover true/false configuration on new/load/resume and retained
query reconstruction. A custom bridge must implement those same promises before
setting the bit. Upstream can choose another advertisement shape; Kandev must
then reconcile this proposed extension before relying on the released contract.

No external publication or upstream dependency change is authorized here.
Implementation preserves the pinned bridge and recognizes the documented opt-in
extension while upstream adoption remains separate. Do not claim upstream
adoption from a passing mock test.

## Observability and trust

Keep existing rejection debug logs. Add a bounded debug reason for support
selection: explicit, verified legacy, or unsupported. Do not log full metadata,
prompts, credentials, or suggestion text. No new metrics are required.

Wire declarations are provider assertions. They are not security attestations.
User preference, task scope, turn ownership, and explicit user submission remain
independent controls.

## Related decisions and delivery

- [Native capability decision](../../../decisions/2026-10-28-native-prompt-suggestion-capability.md)
- [ACP compatibility dialects](../../../decisions/0044-acp-agent-compatibility-dialects.md)
- [Prompt generation identity](../../../decisions/0035-version-agent-ready-events-by-prompt-generation.md)
- [Provider-neutral background work](../../../decisions/2026-09-27-provider-neutral-background-work.md)
- [Custom ACP restoration](../requirements/custom-acp-agents.md)
- [Implementation plan](../../../plans/native-prompt-suggestion-capability/plan.md)
- [Original delivered work](../../../plans/composer-prompt-suggestions/plan.md)

