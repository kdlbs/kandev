# Background-work protocol evidence

## Baseline

Design review only, 2026-09-27. PR #3916 head:
`b3ead0d0376317215ddcadc48b316b3743782744`.
Pinned schema: `apps/backend/pkg/codexappserver/schema/v0.154.0/`.
No new live-provider or runtime test was executed during this design revision.
Task 03 fills fixture evidence; Task 06 records executor and optional live results.

| Capability | Evidence available at design time | Implementation/evidence still required | Initial fallback |
| --- | --- | --- | --- |
| Background discovery | Pinned ThreadBackgroundTerminalsListParams has cursor/limit; response has nextCursor; current Go DTO omits pagination | Typed pagination, complete-snapshot/race tests, real list observations | Unknown on partial/error; never false completion |
| Targeted command stop | Pinned ThreadBackgroundTerminalsTerminateParams has threadId/processId; response has terminated | Typed RPC, exact ownership, fake round trip and separately labelled live result | Unsupported until implemented/verified |
| Child interrupt | Pinned TurnInterruptParams requires threadId and turnId | Active child-run binding, delayed-stop regression and child/root/sibling isolation | Unsupported when exact active run is unknown |
| Command output | Native adapter currently handles final aggregatedOutput but not output-delta notifications | Owned output-delta fixture, final/delta reconciliation, bounded stream transport | Labelled retained snapshot or unavailable |
| Background stdin | CommandExecWriteParams addresses the original client-created command/exec processId | No evidence that thread-owned background terminals share this input contract | Unsupported for initial Codex adapter |
| Config feature keys | Existing client has experimentalFeature/list; experimentalApi is a separate initialize capability | Inspect pinned supported config and effective restrictions; do not assert collab/background_terminals keys | No blind config injection |
| Child usage | Existing adapter/ledger normalize scoped usage; PR says exact usage and child correlation were not verified live | Attribution and duplicate-accounting fixtures; available/unavailable live evidence | Estimated or unavailable with provenance |
| Native foreground steering | Pinned TurnSteerParams requires threadId/input/expectedTurnId; response returns turnId; branch has no native mapping | Tasks 07/08 implement same-turn dispatch, receipts, queue-bypass UI, and single-turn fake/live evidence | Unsupported until implemented and gated; never infer from ACP promptQueueing |
| ACP observations | Existing BackgroundWorkPayload, subagent normalization and detached-shell recognizers | Shared projection/transport/browser conformance from actual normalized frames | Observation only; controls unavailable |
| Claude native | User-confirmed future integration direction | Future adapter outside this package | No support claim |

[Official app-server documentation](https://learn.chatgpt.com/docs/app-server#clean-background-terminals)
corroborates the distinction between background-terminal termination and standalone
command/exec input. The pinned schema, not a moving documentation page, determines
this package's wire contract. `analysis.md` remains non-authoritative research.

## Implementation evidence record

### Codex 0.154.0 Protocol Implementation Record (Task 03)

- **Target Binary & Schema**: `codex-cli 0.154.0`, schema `apps/backend/pkg/codexappserver/schema/v0.154.0/codex_app_server_protocol.v2.schemas.json`.
- **Background Discovery**: `ThreadBackgroundTerminalsListParams` with cursor and limit pagination implemented and tested in `TestCodexBackgroundPaginationAndPartialFailure`. Partial snapshot errors preserve existing terminal records instead of triggering false completion.
- **Background Polling Lifecycle**: Poller starts when background terminals exist and stops when terminals list is empty (`TestCodexBackgroundSnapshotGeneration`).
- **Targeted Command Termination**: `thread/backgroundTerminals/terminate` with exact `processId` tested in `TestCodexChildInterruptExactTurn`.
- **Child Subagent Interrupt**: `turn/interrupt` targeted to child thread ID tested in `TestCodexChildInterruptExactTurn`.
- **Early Child Event Binding**: `TestCodexChildBeforeBinding` proves buffering and draining of early child activity before tool call arrival.
- **Nested Child Calls**: `TestCodexNestedChildAndMultipleCalls` proves isolated parentage and tracking.
- **Output Reconciliation**: `TestCodexOutputDeltaFinalReconciliation` proves command execution output capture without duplicate emission.
- **Capability Gating**: `TestCodexBackgroundCapabilities` proves background terminal and subagent capability maps and unsupported action rejections (`write_input`, `close_input`).
