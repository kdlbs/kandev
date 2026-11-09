---
created: 2026-10-29
status: complete
requirements:
  - REQ-PLATFORM-TURN-CONTINUITY-001
  - REQ-PLATFORM-TURN-CONTINUITY-002
  - REQ-PLATFORM-TURN-CONTINUITY-004
  - REQ-PLATFORM-TURN-CONTINUITY-005
system_design:
  - ../../specs/platform/system-design/acp-prompt-error-continuity.md
  - ../../specs/platform/system-design/transient-turn-runtime-continuity.md
legacy_specs: []
---

# Implementation plan: ACP prompt error continuity

## Overview

End a failed prompt with an inline error while preserving a usable ACP runtime for later user input.
Apply this behavior across ACP providers, without coupling runtime lifetime to retry classification.
Platform owns the shared runtime and prompt-settlement boundary.

Deliver SDK response provenance, shared runtime retention, durable task attention, then UI acknowledgement and browser proof.
All work orders are complete. The implementation ran sequentially in the primary conversation.

## Evidence and assumptions

Confirmed intent: failed turns leave usable ACP conversations available for follow-up prompts across agents.
Confirmed separation: preserving a runtime does not authorize automatic replay or declare the failed work successful.
Confirmed feedback: retain the task error icon until explicit dismissal or accepted manual input with client-observed same-session error evidence.
Opening or focusing the task does not acknowledge the error. Right-click and visible phone task actions provide dismissal.

The observed OpenCode session returned `Internal error: Failed to execute statement` after a SQLite lock timeout.
Kandev marked its execution failed at 2026-10-29T20:32:58Z.
Its saved native conversation restored later. A later isolated reproduction on
OpenCode 2.2.0 proved same-process follow-up acceptance after the same lock
timeout (Task 02 results).

At the time of diagnosis, the adapter restricted retained prompt errors by provider and transient classification, and lifecycle added another provider-name restriction.
The pinned SDK fork at `48a57e4292e8` also created `RequestError` for local failures, so removing only the allowlists could not establish remote-response provenance.
Implementation replaced both gates with the SDK response-origin marker.

The smallest host regression uses a real SDK pipe connection and a previously rejected generic error.
It expects `retain_runtime` and a second accepted prompt on that exact connection.
This currently fails the application-error gate.
The lifecycle regression uses a previously unlisted ACP identity with valid host attestation.

Scope follows the concrete-profile interactive owner, including task-backed Quick Chat.
Quick Chat receives retention, durable attention, inline Dismiss, and same-session observed-prompt acknowledgement.
It gains no normal sidebar row or task menu.
Autonomous execution policy and non-ACP transports remain outside this package.
No new retry policy, setting, schema, or flag is necessary.

## Scope

### In scope

- Typed origin for accepted remote error responses in the maintained ACP SDK fork.
- A reviewed immutable SDK revision pinned in `apps/backend/go.mod` and `go.sum`.
- Provider-neutral retention after a correlated remote prompt error settles on a usable runtime.
- Generic internal, database, auth, quota, invalid-model, empty-message, and unknown errors under the same lifetime rule.
- Sanitized diagnostics, current-turn ownership, durable failed-turn settlement, and usable follow-up input.
- Existing provider diagnostics and automatic recovery safety without expanded admission.
- Durable task attention, an inline Dismiss control, task-menu dismissal, and acknowledgement on accepted manual input.
- Native OpenCode lock-timeout evidence in disposable data.
- Desktop/phone inline-error behavior, identity proof, history persistence, and actual-loss recovery regressions.
- Public guidance updated during implementation.

### Out of scope

- ACP v2 migration or changes to the protocol wire format.
- Native Codex app-server or other non-ACP transports.
- Office, dynamic routing, automation, utility, or passthrough lifetime policy.
- Error recovery for startup, restore, configuration operations, or unacknowledged cancellation.
- New automatic retries, fallback rules, model changes, budgets, or continuation providers.
- Repairs to OpenCode's database locking or changes to live user data/processes.

## Technical approach

### 1. SDK origin and host dependency

In `kdlbs/acp-go-sdk`, add `RequestError.IsRemoteResponse()` with a private, nonserialized marker.
Set it only for accepted matching error replies in `Connection.handleResponse`.
Keep local constructors, context failures, send failures, decode errors, and disconnects unmarked.
Retain the existing ordered notification barrier and error wrapping behavior.
Pin the reviewed fork revision in Kandev. Do not use module-cache edits or a local replace path.

This prerequisite is a source change in an existing dependency repository.
Its exact commit must be recorded before host implementation consumes the method.
An unavailable immutable revision blocks this dependency, rather than authorizing a weaker evidence check.

### 2. Shared failure settlement

In `adapter_prompt_failure.go`, replace provider/transient gates with the SDK origin and current usable-runtime checks.
Keep cancellation and actual transport loss ahead of this path in `adapter_prompt.go`.
Remove the obsolete mock retention dialect hook and all lifecycle provider-name exceptions.
Mock errors establish origin through real SDK replies. Independent mock capacity/continuation capabilities remain.
Route richer Codex quota diagnostics through the common retained-turn emission instead of an early terminal return.
Use existing sanitized projection, including fallback text for empty messages.

Remove the redundant lifecycle provider-name gate for the trusted ACP disposition.
Keep task-owner, initialized-session, open-stream, generation, startup, and exit checks.
Reuse `AgentTurnFailed`, managed-input settlement, durable status messages, and successor admission fences.
Existing classifications still decide retry/continuation eligibility.
Previously unsupported error classes gain manual continuity only.
Invert or retarget the legacy generic/internal/auth/quota and unlisted-provider rejection tests to remote-origin positives and genuine evidence-loss negatives.
Suppress successful-turn notifications for authoritative error-terminated completions. Keep existing success, clarification, and unread semantics otherwise.

### 3. Durable task attention

Persist a stamped `last_turn_error` pointer in task metadata after manual failed-turn settlement.
Project a bounded optional `turn_error` in `TaskStatusSummary` without writing blocking recovery metadata.
Use task metadata admission, conditional writes, and existing task/summary publication.
A failure from any eligible session can set the latest task-level pointer while its siblings remain independent.

Add `POST /api/v1/tasks/:id/turn-error/dismiss` with a required observed stamp.
The inline action and task context/overflow action use this same authorized operation.
A new manual prompt carries optional `observed_turn_error_stamp` from the client.
Acknowledgement requires that stamp to match current attention from the same target session, after durable acceptance.
Omitted or stale evidence does not block input, but leaves attention unchanged.
Queue drains retain original acceptance-time intent and cannot acknowledge failures created after enqueue.
Queue edits, retry, and Send now on existing entries gain no new acknowledgement authority.
Rejected sends, duplicate submissions, automatic recovery, focus, read state, and unrelated output do not clear attention.
A stale action cannot dismiss a newer failure. History and runtime state remain unchanged.

### 4. Compatibility matrix

| Provider/context | Transport and evidence | Expected behavior | Verification and fallback |
| --- | --- | --- | --- |
| Claude, Codex, OpenCode, Cursor, Gemini and other registered ACP agents | Proven remote prompt error, same initialized native session, current generation, live transport | Retain runtime and fail the turn across error classes | Table-driven real-wire tests; missing proof retains existing failure handling |
| Previously unknown ACP identity | Same generic host proof | Same manual continuity | New provider identity in adapter and lifecycle tests; no name-based exemption |
| Codex quota wrapper | Correlated quota diagnostic plus proven prompt error | Keep detailed guidance and usable Chat | Wrapper-preservation regression; no new retry |
| Known capacity/terminal stream diagnostic | Existing provider-specific evidence | Existing retained and automatic recovery policy | Preserve automatic-recovery assertions; retarget old lifetime negatives in Task 02 |
| Local SDK failure or malformed reply | No remote error-origin marker | Existing uncertain or terminal handling | Negative pipe, decode, deadline, and cancellation tests |
| Actual process or transport death | Authoritative exit or SDK disconnect | Recovery handling | Causal close/crash tests and browser recovery control |
| Older agentctl | No new generic retention attestation | Existing conservative behavior | Disposition omission and invalid-value regressions |
| Newer agentctl with older backend | Valid attestation reaches the older lifecycle name gate | Unlisted providers keep existing terminal/recovery behavior | Mixed-version boundary; no claim of generic host support |
| Office, dynamic, utility, automation, passthrough, native app-server | Existing execution owner or another transport | Existing policy | Owner exclusion and transport-boundary regressions |

Shared tests prove the generic protocol contract. They do not claim every native agent implementation survives every internal error.
Record native compatibility separately, including actual process loss when it occurs.

### 5. History and browser surfaces

Reuse `runtime_retained`, `failure_scope=turn`, and the existing inline error renderer.
The normal composer remains available after durable settlement.
Add the localized current-error Dismiss control and task-menu Dismiss error action.
Use existing compact task summary and row state for the badge. Preserve the existing Chat composition and composer.
Phone task actions need a visible touch trigger. Right-click, hover, and long press cannot be its only dismissal path.
Localize new labels, accessible names, and feedback in all required catalogs.

Add generic mock errors that do not use the capacity marker or transient keywords.
Run representative cases under production OpenCode and another ACP identity.
Use the existing opt-in trace to prove process, connection, native session, initialize/new/load/resume, and prompt counts.
A Data-only disconnect-shaped error while ACP stays open is an application error, not actual transport death.
Use a causal crash/close for true runtime-loss tests and update affected fixture guidance.

## ASCII UI preview

### UI-01: Failed prompt, usable Chat

Entry point: open the affected task/session Chat after its prompt fails.
Current evidence: the reported session showed a recovery card with Resume and Start fresh actions.
The shipped retained-capacity path already supplies the target composition.

```text
Before: reported generic error
  Chat transcript
  [Session recovery failed]
  [Resume session] [Start fresh session]

After: desktop and phone share the same hierarchy
  Chat transcript
  Sidebar: [!] Task title
  [!] <sanitized prompt error>   [Technical details] [Dismiss]
  [Follow-up message                         ] [Send]
```

The error stays inside the transcript's existing scroll region.
The composer stays in its existing fixed region and preserves its draft.
Focusing Chat keeps the sidebar badge. Dismiss acknowledges its stamp.
A new prompt acknowledges only a client-observed error from that same session.
After acknowledgement, retain readable historical error content without active alert treatment.
Phone details wrap within the chat width. Existing Send and model controls remain reachable by touch.
Phone safe-area, keyboard, and scroll behavior use `mobile/session-mobile-layout.tsx`.
Desktop retains compact controls. No new drawer or navigation is necessary for a passive error entry.
Hierarchy, inline scope, and available input are requirements. Wording and spacing here are illustrative.
Mappings: AC-PLATFORM-TURN-CONTINUITY-002.1, 002.4, 002.5, 004.3, 004.7, and 005.1-.3.

### UI-02: Later actual runtime loss

```text
  Chat transcript
  [!] <earlier prompt error>     [Technical details]
  [Runtime recovery explanation]
  [Resume session] [Start fresh session]
```

Existing recovery eligibility decides which actions appear.
When both errors remain current, runtime recovery and Dismiss turn error are separate controls.
Acknowledging turn attention does not clear the higher-priority runtime-error badge.
A later crash can require recovery without changing the earlier error's turn scope.
The same existing responsive recovery surface serves desktop and phone.
Mappings: AC-PLATFORM-TURN-CONTINUITY-001.6, 004.4, and 004.5.

### UI-03: Task action dismissal

```text
Desktop: right-click task or open More options
  [!] Task title
  Task menu
    ...existing actions...
    Dismiss error

Phone: Task 04 adds a visible per-row More options control
  [!] Task title                 [More options]
  Existing touch menu surface
    ...existing actions...
    Dismiss error
```

Use the shared task menu action and the same observed error stamp on both surfaces.
The current phone task-switcher sheet has no per-row trigger; Task 04 adds it using the existing touch menu primitives.
Dismissal keeps the current route and draft. If acknowledgement fails, retain the badge and show localized feedback.
A new error re-arms the badge. A stale open menu cannot dismiss that newer error.
Phone reuses the shipped task-action menu and its safe-area scroller, with targets of at least 44px.
Desktop keeps compact controls and keyboard activation. Menu grouping follows the existing mark/attention actions.
Mappings: AC-PLATFORM-TURN-CONTINUITY-005.4-.8.

## Tests

| Acceptance criteria | Planned regression and existing evidence |
| --- | --- |
| 004.1, 004.2, 004.4 | SDK `TestRemoteRequestErrorOrigin`, `TestLocalRequestErrorOrigin`, `TestRequestErrorOriginNotOnWire` |
| 001.1-.3, 004.1-.4 | ACP `TestPromptErrorRetainsACPConnectionForFollowUp`, with provider/error matrix and repeated failures |
| 004.2, 004.6 | ACP `TestPromptErrorPreservesCodexQuotaDiagnostic`, existing provider projection/sanitization and capacity tests |
| 001.6-.8, 004.4-.5 | Lifecycle `TestPromptErrorRetentionUsesHostAttestation`, existing turn-failure eligibility, duplicate, exit, and settlement-fence tests |
| 004.9 | Gateway `TestErrorTerminatedTurnDoesNotNotify`: retained failure, runtime failure, successful turn, and real clarification controls |
| 001.7, 004.5, 004.8 | Orchestrator `TestAgentTurnFailedGenericPromptError`, existing durable owner, delivery outcome, blocking caller, and autonomous-policy tests |
| 004.6 | Orchestrator `TestGenericRetainedPromptErrorDoesNotRetry`, existing capacity/transient recovery tests |
| 001.3, 004.2-.4 | Native `TestOpenCodeLockTimeoutNativeCompatibility`, isolated database and local provider |
| 005.1, 005.5-.7 | `TestTaskTurnErrorAttentionProjection`: boot/live/restart, stale deltas, and sibling sessions |
| 005.2, 005.4, 005.6, 005.8 | `TestDismissTaskTurnErrorIfStamp`: authorized, idempotent, stale, foreign, and storage failures |
| 005.3, 005.6 | `TestAcceptedManualPromptAcknowledgesTaskTurnError`: observed same-session input, unseen sibling error, queued-before-failure drain, rejected/duplicate/internal input |
| 002.1-.5, 004.3, 004.7, 005.1-.8 | Desktop and phone Playwright scenarios below |

New test names are implementation targets. No new permanent tests ran during planning.
Existing completed packages remain historical: [continuity](../transient-turn-runtime-continuity/plan.md),
[capacity continuation](../model-capacity-continuation/plan.md),
[delayed retries](../delayed-capacity-retries/plan.md), and [OpenCode capacity](../opencode-capacity-retry/plan.md).
Their manifests and work orders were inventoried as complete/done. Do not rewrite their historical results.

## E2E tests

Create `apps/web/e2e/tests/session/acp-prompt-error-continuity.spec.ts` for `chromium`.
Create `mobile-acp-prompt-error-continuity.spec.ts` for `mobile-chrome`.
Use `apps/web/e2e/helpers/acp-prompt-error-continuity.ts` for shared fixtures and causal waits.
Reuse existing trace assertions without their capacity-specific message assumptions.

Required scenarios:

1. A generic/storage error after a completed effect shows one inline error and accepts a distinct follow-up on the original runtime.
2. Repeated errors remain separate failed turns. Reload and another viewer preserve history without initialization or duplicate dispatch.
3. Auth/quota/invalid-model errors preserve sanitized guidance and manual input without gaining automatic recovery.
4. A later real crash uses existing recovery controls while earlier turn errors remain historical.
5. Focusing/opening Chat leaves the sidebar icon. Inline Dismiss clears it across reload/viewers while history remains.
6. A newly accepted prompt with a client-observed same-session stamp acknowledges that error without waiting for output.
   Missing observation, unseen sibling errors, failed sends, and drafts retain attention.
   Input accepted before failure must not clear that failure when it drains, retries, or is sent now.
7. Desktop task right-click and visible phone task actions dismiss the observed error without navigation or runtime restart.
8. A stale dismissal racing a newer sibling error preserves the newer badge.
   Sending in another session cannot clear an unseen failure. Genuine runtime/pending-input indicators remain independent.
9. Phone details wrap without horizontal overflow. The new per-row menu trigger, Dismiss, and Send have usable touch targets.
10. Task-backed Quick Chat supports inline Dismiss and observed same-session sends without adding a normal task-menu/sidebar entry.
11. Error completion emits no successful-turn or fabricated needs-answer notification. Read cursors and error acknowledgement remain independent.

Both projects run sequentially through the managed runner with fresh production builds.
Keep the existing transient continuity and OpenCode capacity files in the focused regression selection.

## Work orders

- [x] [Task 01: Identify received ACP error responses](task-01-sdk-error-origin.md)
- [x] [Task 02: Preserve runtimes after ACP prompt errors](task-02-preserve-prompt-error-runtime.md)
- [x] [Task 03: Persist task turn-error attention](task-03-task-error-attention.md)
- [x] [Task 04: Expose error acknowledgement and prove continuity](task-04-chat-proof-and-guidance.md)

Dependency order: 01 -> 02 -> 03 -> 04. Every work order is sequential.

## Verification results

Implementation completed in the primary conversation.

Dependency and SDK:

- `kdlbs/acp-go-sdk` revision `5e124db622e9e7ed37d1e49b78a28ffcdb58a444`
  (branch `kandev/acp-prompt-error-origin`) is pinned in `apps/backend/go.mod`.
- SDK focused origin, notification-barrier, request-cancel, and
  peer-disconnect tests passed. The fork's pre-existing
  `TestConnectionReceivesLargeSingleLineNotification` timeout is unchanged.

Backend:

- `go test -tags fts5` passed for the changed packages:
  `agentctl/server/adapter/transport/acp`, `agent/runtime/lifecycle`,
  `orchestrator`, `orchestrator/handlers`, `backendapp`, `task/models`,
  `task/statussummary`, `task/service`, `task/repository/sqlite`,
  `task/handlers`, and `cmd/mock-agent`.
- `go run ./cmd/sqlguard ./internal` and
  `go test ./internal/persistence/storeconformance` passed.
- `bash scripts/lint-go-changed` reported 0 issues for every changed Go file.
- Native OpenCode 2.2.0 lock-timeout reproduction passed in 14.03s with an
  isolated `XDG_DATA_HOME`: the turn failed with
  `Internal error: Failed to execute statement` and `retain_runtime`, then a
  distinct follow-up completed on the same process and native session.

Frontend:

- `pnpm run typecheck` and `pnpm --filter @kandev/web build:vite` passed.
- `pnpm run i18n:check` passed after pseudo and Traditional Chinese generation.
- Focused Vitest suites for selectors, the API client, the acknowledgement hook,
  sidebar/sheet rows, the action message, and the message handler passed.
- Scoped ESLint for the new E2E files passed.

Documentation:

- `python3 scripts/list-docs.py validate` and
  `python3 scripts/lint-spec-files.py --all` passed during design; they were not
  invalidated by implementation.
- `node scripts/validate-public-docs.mjs` passed (54 pages).

Browser proof:

- `pnpm e2e:run --project chromium tests/session/acp-prompt-error-continuity.spec.ts`:
  4 passed (generic follow-up continuity, focus plus inline dismissal across
  reload, task-menu dismissal, and task-backed Quick Chat with empty
  board/overview collections).
- `pnpm e2e:run --project mobile-chrome tests/session/mobile-acp-prompt-error-continuity.spec.ts`:
  2 passed (inline error, wrapped details, 44px inline dismissal, visible
  per-row task actions with 44px trigger and Dismiss turn error).
- `pnpm e2e:run --project chromium tests/session/transient-retry-transport-lost.spec.ts`:
  2 passed after retargeting the file to a genuine `/crash` subprocess exit.
- All three ran sequentially through the managed runner with fresh builds. The
  Kubernetes `containers` project was updated to `/crash` but not run here
  (no container daemon/Kind cluster).

One full-suite run of `internal/task/repository/sqlite` exceeded its 30-minute
test timeout in this environment; the interrupted test and every changed
repository regression passed in focused runs.

## Review remediation

Static review findings were fixed in the implementation:

1. Codex quota retention now requires SDK remote-response provenance; quota
   evidence after a local deadline stays on the conservative diagnostic path.
2. Full-row task writes preserve the live acknowledged `last_turn_error` value
   through the existing locked-row splice, and ordinary metadata merge excludes
   it. A SQLite regression covers stale snapshots and nil metadata; the matching
   PostgreSQL regression is present but was not run without a DSN.
3. Task-backed Quick Chat reads the bounded live summary cache and hydrates it
   from task detail on reopen/reload; inline dismissal and prompt acknowledgement
   work without a sidebar row. A real Quick Chat Playwright case covers empty
   board/overview collections.
4. Attention ordering uses the persisted failed-turn message's `CreatedAt`;
   delayed older settlement cannot replace or re-arm newer attention.
5. Existing-summary boot/list reconciliation compares and repairs `turn_error`
   from task metadata under the normal CAS revision guard.
6. Accepted direct and queued prompts return nonfatal acknowledgement-failure
   feedback; the prompt stays accepted exactly once and the badge remains.
7. Runtime-loss browser scenarios now use a genuine `/crash` subprocess exit;
   `/transport-lost` remains the response-shaped continuity fixture. The
   Kubernetes `containers` scenario was updated but not run here.

## Risks

- SDK origin must distinguish every local error path without trusting error Data.
- The SDK prerequisite spans the maintained dependency repository and needs an immutable reviewed revision.
- Native agent internals can exit after returning an error. Process/transport loss must override retention.
- Removing provider restrictions must not broaden capacity continuation or automatic retries.
- Existing mock errors can imitate transport loss while leaving the connection open. Recovery tests need causal loss evidence.
- Durable settlement and cancellation races can otherwise admit a successor too early or stop a later turn.
- Task attention must not enter blocking recovery selectors or disappear through legacy read/agent-output acknowledgements.
- Accepted input and its stamped acknowledgement must not clear a newer failure or cause a duplicate prompt after metadata failure.
- Native lock injection needs isolated data, explicit barriers, bounded waits, and complete child-process cleanup.

## Documentation coverage preflight

Run this block from the repository root after artifact changes:

```bash
node <<'NODE'
const fs = require('node:fs');
const cp = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const changed = [...new Set([
  ...cp.execFileSync('git', ['diff', '--name-only', '-z', 'HEAD']).toString().split('\0'),
  ...cp.execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z']).toString().split('\0'),
].filter(Boolean))];
const dir = 'docs/plans/acp-prompt-error-continuity';
const documents = [
  ...fs.readdirSync(dir).filter(p => p.endsWith('.md')).map(p => `${dir}/${p}`),
  'docs/specs/platform/requirements/transient-turn-runtime-continuity.md',
  'docs/specs/platform/system-design/transient-turn-runtime-continuity.md',
  'docs/specs/platform/system-design/acp-prompt-error-continuity.md',
  'docs/specs/platform/system-design/bounded-task-status-delivery.md',
];
const fileContents = Object.fromEntries(documents.map(p => [p, fs.readFileSync(p, 'utf8')]));
const planned = [
  'apps/backend/go.mod',
  'apps/backend/go.sum',
  'apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt.go',
  'apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt_failure.go',
  'apps/backend/internal/agentctl/server/adapter/transport/acp/dialect.go',
  'apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_mock.go',
  'apps/backend/internal/agent/runtime/lifecycle/manager_events.go',
  'apps/backend/internal/orchestrator/event_handlers_turn_failure.go',
  'apps/backend/internal/backendapp/gateway.go',
  'apps/backend/internal/task/models/turn_errors.go',
  'apps/backend/internal/task/handlers/task_handlers.go',
  'apps/backend/internal/task/handlers/task_http_handlers.go',
  'apps/backend/internal/task/handlers/message_handlers.go',
  'apps/backend/internal/task/service/reserved_metadata.go',
  'apps/backend/internal/task/service/service_task_turn_error.go',
  'apps/backend/internal/task/service/service_turns.go',
  'apps/backend/internal/task/service/service_status_summary_rebuild_input.go',
  'apps/backend/internal/task/service/service_status_summary_reconcile.go',
  'apps/backend/internal/task/repository/sqlite/metadata_launch_error_cas.go',
  'apps/backend/internal/task/statussummary/model.go',
  'apps/backend/internal/task/statussummary/projector.go',
  'apps/backend/internal/task/statussummary/projector_events.go',
  'apps/backend/internal/task/statussummary/projector_derive.go',
  'apps/backend/internal/task/statussummary/rebuild.go',
  'apps/backend/internal/orchestrator/handlers/queue_handlers.go',
  'apps/backend/internal/orchestrator/handlers/queue_operations_handlers.go',
  'apps/backend/internal/orchestrator/messagequeue/admission_metadata.go',
  'apps/backend/cmd/mock-agent/prompt_error_continuity.go',
  'apps/web/lib/types/task-status-summary.ts',
  'apps/web/lib/task-status-summary.ts',
  'apps/web/lib/api/domains/task-turn-error-api.ts',
  'apps/web/lib/api/domains/session-api.ts',
  'apps/web/lib/api/domains/queue-api.ts',
  'apps/web/hooks/use-task-turn-error.ts',
  'apps/web/hooks/use-task-actions-menu.ts',
  'apps/web/hooks/domains/session/use-session-prompts.ts',
  'apps/web/hooks/domains/session/use-queue.ts',
  'apps/web/hooks/domains/session/use-queue-admission.ts',
  'apps/web/components/task/task-session-sidebar-item.ts',
  'apps/web/components/task/task-switcher-context-menu-items.tsx',
  'apps/web/components/task/mobile/session-task-switcher-sheet.tsx',
  'apps/web/lib/kanban/task-actions-menu-entries.ts',
  'apps/web/components/quick-chat/quick-chat-session-view.tsx',
  'apps/web/components/quick-chat/quick-chat-content.tsx',
  'apps/web/e2e/helpers/acp-prompt-error-continuity.ts',
  'apps/web/e2e/tests/session/acp-prompt-error-continuity.spec.ts',
  'apps/web/e2e/tests/session/mobile-acp-prompt-error-continuity.spec.ts',
  'docs/public/sessions-and-review.md',
];
for (const [label, paths] of [['actual', changed], ['planned', [...changed, ...planned]]]) {
  const result = validateCoverage({ changedFiles: paths, fileContents });
  console.log(JSON.stringify({ label, status: result.status, errors: result.errors }));
  if (!result.ok) process.exitCode = 1;
}
NODE
```
