---
status: current
system: ui
requirements:
  - REQ-UI-CONTEXT-COMPACTION-COUNT-001
---

# Context Compaction Count System Design

## Purpose and boundaries

This design completes the existing [context count requirement](../requirements/context-compaction-count.md).
It covers normalized usage, session metadata, and the context-window explanation as one capability.
The existing requirement remains canonical. This bounded repair creates no parallel agent or presentation requirement.

The agent runtime owns execution identity and provider observations.
Task storage owns the session row and atomic metadata updates.
The frontend consumes the persisted count and reporting metadata without implementing provider policy.
The [Copilot native design](../../agents/system-design/copilot-native-compaction.md) owns the additional explicit counter and root-usage path.
The [ACP session-events design](../../agents/system-design/acp-session-events.md) owns explicit compaction timeline entries.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-CONTEXT-COMPACTION-COUNT-001` | Eligibility policy, data flow, persistence, recovery, and presentation |

## Eligibility policy

`EventPublisher.PublishContextWindow` receives the trusted `AgentExecution` for every accepted context-window event.
`AgentExecution.AgentID` is the provider type. `AgentExecution.ID` is the execution identifier.
The current payload's `agent_id` contains the execution identifier and must not select provider policy.

The publisher excludes Copilot ACP observations using `agents.CopilotACPAgentID`.
This is backend counting policy. The ACP adapter continues to normalize and account for every usable usage update.
No provider parsing, dialect hook, raw agentctl field, or inferred child identity is required.

The exclusion covers all current Copilot ACP usage observations, including periods with no open subagent card.
Parallel work, delayed notifications, and restoration prevent reliable attribution from tool lifetimes.
An arbitrary metadata agent identifier cannot enable inference.
Future support requires separately verified context attribution and a policy change.
Other providers retain existing inference, without a new guarantee of parent-only attribution.

## Data and contracts

Add `CompactionInferenceDisabled bool` to `lifecycle.ContextWindowEventPayload` and `watcher.ContextWindowData`.
Both use the optional JSON key `compaction_inference_disabled` with `omitempty`.
The handler stores this boolean in the context-window sample.

| Metadata | Meaning |
| --- | --- |
| `context_window` | Latest usable size, used, remaining, efficiency, source, and exclusion marker |
| `context_window.compaction_inference_disabled` | `true` excludes this sample. Missing or `false` preserves legacy eligibility |
| `context_compaction_count` | Non-negative session lifetime count. Absence means zero |

The `source` field retains `acp` or `api`; it does not encode provider identity or eligibility.
The existing usage-inference path needs no table, repository interface, or HTTP route changes.
The native provider path defines its additional ledger and normalized events in its owning design.

## Control flow

1. `acp.Adapter.convertUsageUpdate` updates usage/cost trackers and emits the latest context-window observation.
2. `Manager.handleContextWindowEvent` forwards it through `EventPublisher.PublishContextWindow`.
3. The publisher derives exclusion from the trusted execution provider and includes it in the internal event.
4. `Watcher.createContextWindowHandler` decodes the full payload, including the boolean.
5. `Service.handleContextWindowUpdated` includes the marker in the persisted sample.
6. `Repository.UpdateSessionContextWindow` atomically stores the sample and computes the count.
7. The existing `session.state_changed` metadata patch publishes sample and count after successful persistence.

The current arrival order, synchronous persistence, and context-reset generation guard remain authoritative.
No new goroutine, time window, or independent read/modify/write operation is added.

## Atomic persistence

Both branches of `updateSessionContextWindowQuery` require every condition below for an increment:

- The incoming sample is eligible.
- The previous persisted sample is eligible.
- The previous used-token value is numeric.
- The incoming used-token value is strictly smaller.

The incoming eligibility value is a bound query parameter.
The previous marker comes from the same stored metadata expression as the baseline.
The statement always stores the new sample, including excluded observations, and returns the count.
Existing metadata defaults and preservation of unrelated keys remain intact.

An excluded sample replaces visible usage but cannot become a countable baseline.
The next eligible sample establishes a baseline without incrementing.
A following eligible decrease can increment normally.

## Persistence, failure, and recovery

The marker lives beside the baseline, so restart cannot restore an excluded baseline as eligible.
Successful reset and model change clear the complete sample and preserve the lifetime count.
Failed resets retain both values under the existing freshness contract.
Duplicate eligible drops compare equal after the first write. Excluded samples never increment.

New Copilot observations remain excluded when the old sample has no marker.
An old unmarked non-Copilot sample retains legacy behavior.
Previously inflated counts cannot be reconstructed from the stored total and remain unchanged.

Classification occurs in the current backend, so retained older agentctl processes need no new event field.
Restoration must retain the actual provider type on `AgentExecution`, as normal runtime recovery already requires.
Tests cover normalized agentctl events without any exclusion property.

A persistence error or stale reset generation publishes neither the new sample nor its count.
Existing session-scoped logs remain the diagnostic surface. No metric or provider-file reader is added.

## Presentation

The frontend consumes `compactionCount` and optional `context_compaction_reporting` through its parser and session-runtime store.
Missing reporting metadata preserves legacy inference help. Native reporting comes from the provider-owned backend projection.
It does not interpret the exclusion marker as proof that all counting is unavailable.
Native reporting availability and coverage select the explanation and any partial/unavailable status.
The native provider design defines those states and keeps old inferred totals intact.
The existing count row and accessible info control retain their composition.

### Generic help and provider details

The generic explanation satisfies criteria `.14` and `.15` under `REQ-UI-CONTEXT-COMPACTION-COUNT-001`.
`compactionHelpKey` in `apps/web/components/task/chat/token-usage-display.tsx` already separates inferred and native reporting.
Missing reporting metadata and `source: "usage_drop"` select `common:contextCompactionCountHelp`.
Keep that dispatch and its render-time `t()` call.
The shared explanation requires no agent-profile lookup or new provider check in React.

The generic key explains inference and accuracy without the Copilot exclusion sentence.
Its English text is: "Kandev infers this count from context usage drops. Missing samples or provider resets can affect accuracy."
The exclusion remains counting policy, documented in this design and the public session guide.

Native Copilot reporting retains the existing `contextCompactionCountHelpNative`, `contextCompactionCountHelpPartial`, and `contextCompactionCountHelpUnavailable` keys.
Their source, coverage, and retained-history explanations remain unchanged.
The existing status selection and count row also remain unchanged.

Update the generic key in `en`, `pt-pt`, `zh-cn`, `ja`, and `ko`.
Generate `zh-tw` and `zh-hk` with `i18n:zh-hant`, then regenerate `pseudo` with `i18n:pseudo`.
Only the equivalent Copilot sentence leaves each catalog value.
Review generator output for unrelated changes.

Desktop pointer/keyboard access and the phone tap-pinned surface retain their existing behavior.
The closest mobile exemplar is `mobile-context-window-source.spec.ts`.
This content-only correction uses the same count row and help region on both viewports.
It introduces no new control, overlay, scroll owner, or responsive branch.

The context-window paragraph in `docs/public/sessions-and-review.md` already explains the provider policies and retained history accurately.
It does not quote the generic help, so this copy correction needs no public documentation change.

## Verification boundaries

The main regression passes the issue's observations through the real publisher, watcher decoding, handler, and SQLite repository.
It asserts zero counts, retained usage, and matching session metadata patches.
Repository tests separately cover eligibility transitions, persisted restart baselines, duplicates, and PostgreSQL parity.

Browser tests use the established context-window fixture to prove zero/non-zero count display and the revised explanation.
Those fixture tests prove presentation, not backend inference.
The inference-policy correction requires both the real backend regression and desktop/mobile presentation checks.

For the generic-help correction, update the existing component assertions before changing the catalogs.
Cover missing reporting metadata and explicit `usage_drop` reporting without Copilot text.
Retain the inference disclosure, accuracy limits, zero/non-zero totals, and all native coverage-state assertions.
The desktop and mobile source-help scenarios assert the revised translated explanation after hover or tap.
The phone scenario retains its second-tap dismissal and horizontal-overflow assertions.
Existing backend inference tests remain authoritative; this copy-only package does not change or rerun those unrelated paths.

## Related decisions and plans

- [ACP compatibility dialects](../../../decisions/0044-acp-agent-compatibility-dialects.md).
- [Executor and session ownership](../../../decisions/0003-executors-running-as-execution-id-source-of-truth.md).
- [Original implementation](../../../plans/context-compaction-count/plan.md).
- [Copilot inference correction](../../../plans/copilot-compaction-inference/plan.md).
- [Generic compaction help](../../../plans/generic-compaction-help/plan.md).

## Native-event follow-up

The [native counting package](../../../plans/copilot-native-compaction-count/plan.md) extends this presentation contract.
It preserves the existing `UpdateSessionContextWindow` guards and adds confirmed increments through canonical projection.
A first gauge reading must retain any explicit increments already stored before it arrived.
An explicit count-only patch updates adjacent reporting/count state without inventing a zero-token gauge sample.
The same parser/hook merge behavior applies to live metadata, page hydration, and recovery.

Lifetime counts merge monotonically when a delayed gauge patch or HTTP snapshot follows a newer explicit increment.
Native reporting revisions prevent older availability/coverage patches from replacing newer reporting state.
These consumer freshness rules do not move authoritative increments into the browser.
