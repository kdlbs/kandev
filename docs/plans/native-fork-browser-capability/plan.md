---
created: 2026-10-18
status: done
requirements:
  - REQ-AGENTS-CODEX-NATIVE-004
  - REQ-AGENTS-CODEX-NATIVE-006
system_design:
  - ../../specs/agents/system-design/codex-app-server.md
legacy_specs: []
---

# Implementation Plan: Browser-compatible native conversation forks

## Overview

Restore the existing native fork outcome when the browser lacks
`crypto.randomUUID`. One sequential work order first adds independently authored
consumer regressions, then substitutes the existing shared UUID helper and runs
bounded affected checks. ROOT reviewed this package and later released implementation to the same primary
session on 2026-10-18.

## Scope

### In scope

- AC-AGENTS-CODEX-NATIVE-004.1 and AC-AGENTS-CODEX-NATIVE-006.1 through
  the existing desktop dialog and touch drawer.
- One browser-compatible request ID retained across failed attempts and retries,
  exact fork request payload, existing failure feedback, and returned-session selection.
- Reconcile the owning requirement/design pair without new acceptance IDs.

### Out of scope

- Generic conversation forking, new API fields, backend or adapter changes,
  provider/native protocol changes, and lifecycle hardening.
- Layout, touch geometry, copy, localization, settings-save behavior, registry
  changes, or promotion of native Codex. The existing opt-in remains off in all
  shipped profiles.
- New tasks, sessions, delegates, model switches, broad Go/full-web suites, and
  a new E2E gate for this helper substitution.

## Evidence and assumption check

Confirmed scope is repair of existing fork behavior for opted-in native Codex
users. No material product decision is unresolved. The requirement criteria
remain unchanged; their existing document status is preserved.

At admitted main `3bb7206a3cb99e6192ad08eb6a0925cc98cfae9c`,
`ForkConversationAction.handleFork` initializes its request ref with raw
`crypto.randomUUID()` before its `try` and pending/error flow. With that
capability omitted, confirmation reaches zero fork RPCs. The existing
`generateUUID()` helper succeeds under the same omission.

ROOT qualification reports two causal failures (real MessageActions desktop
and touch consumers) and three passing controls (modern capability success and
returned-session selection, helper fallback, disabled-feature absence).
Evidence: `/tmp/kandev-root-native-fork-capability-7ovohw19/qualified-proof.json`,
`receipt.json`, `admission-source-recheck.json`, and
`supported-access-supplement.json` in that directory. The original proof was
parked; the later ROOT admission authorizes design only after source, impact,
and open-PR rechecks. Qualification used mocked transport and explicit capability
omission; it proves neither a live remote-browser run nor a real native fork.
Do not read, copy, replay, or delete ROOT's private candidate fixture. Do not
rerun qualification against unchanged inputs.

The public [remote-access guide](../../public/mobile-remote-access.md) already
accepts plain HTTP inside a single-user encrypted VPN. The repair conforms to
that documented access path; it adds no browser-security setting or operator step.

## Technical approach

In `apps/web/components/task/chat/messages/message-actions.tsx`, import
`generateUUID` from `@/lib/uuid` and use it only in the existing native
`ForkConversationAction` request-ref initialization. Keep lazy initialization,
retry lifetime, reset after accepted success, and all surrounding control flow.
Do not mock the helper or the state actions in new regression coverage.

`forkConversation` in `apps/web/lib/services/session-launch-service.ts` remains
the real consumer of the transport mock. Assert one `session.fork` request with
exact `task_id`, `session_id`, `turn_id`, a valid UUID `request_id`, and the
existing timeout. Observe real store active task/session after response.
The production service, UUID helper, backend, and adapter need no edits.

PR3897 at admission tip `da5e4b790b6fe9f01d3699683c774b3045f35b66`
shares `message-actions.tsx` and `session-launch-service.ts`; its read patch adds
separate generic `MessageForkAction` and optional launch request fields.
It does not repair or replace the existing native action. Reassess the actual
current patch before implementation/integration if its head moves, preserve
that feature, and never claim disjoint files. The original native support
[plan](../codex-app-server/plan.md) and completed
[Task 06](../codex-app-server/task-06-conversation-forks.md) remain historical
implementation evidence; this package owns only the capability regression.

Compatibility boundaries:

- Native Codex enabled, completed root turn, randomUUID present or absent:
  unchanged native fork envelope and selected destination.
- Native feature disabled: native action absent; no fork submission.
- Failed RPC: existing localized alert, source remains selected, same request
  ID on explicit retry. Accepted success clears it for a later operation.
- Other providers and generic fork: no new support or behavior implied.

## Tests

Author `apps/web/components/task/chat/messages/message-actions-native-fork.test.tsx`
independently from public source and existing tests. Render real MessageActions,
StateProvider/store, shared UUID helper, and fork service. Mock only transport
for the fork path; use deterministic pointer/media inputs to select existing
composition and normal external test scaffolding. Restore original crypto
property descriptors, media listeners, timers, mocks, and mounted state.

Proposed test names and traceability:

- `forks without randomUUID and selects the returned session on touch=%s`
  (desktop and touch): AC-AGENTS-CODEX-NATIVE-004.1, 006.1. Assert dialog/drawer
  confirmation, exactly one request, valid UUID, exact envelope and store result.
  These must fail causally before the helper substitution.
- `preserves the native fork outcome with randomUUID available`:
  AC-AGENTS-CODEX-NATIVE-004.1, 006.1; real capability-present control.
- `hides native fork when the feature is disabled`: AC-AGENTS-CODEX-NATIVE-006.1;
  preserve the existing gate even with a completed native turn.
- `retains the request ID and error feedback through a failed request and retry`:
  AC-AGENTS-CODEX-NATIVE-004.1, 006.1; rejected transport leaves source selected,
  alert visible and confirmation usable; retry reuses the exact ID, then selects
  the accepted destination. A subsequent accepted operation gets a new ID.

Run affected existing `message-actions.test.tsx` and
`action-message-actions.test.tsx` together with the new suite. Keep request-ID
and retry assertions meaningful at the real consumer boundary.

## E2E tests

Existing `apps/web/e2e/tests/chat/codex-app-server.spec.ts` (`chromium`) and
`apps/web/e2e/tests/chat/mobile-codex-app-server.spec.ts` (`mobile-chrome`)
cover AC-AGENTS-CODEX-NATIVE-004.1 and 006.1 dialog/drawer confirmation and
selected-session outcomes. They do not establish the missing-capability case.
For this state-only request-ID substitution, focused real-consumer component
coverage of both compositions supplies the missing regression under the
mobile-parity state/data exception. Rendered layout, touch behavior, scrolling,
navigation and viewport routing do not change. No new E2E or local build gate
is required; configured hosted E2E remains a delivery gate.

## Work orders

- [x] [Task 01: Enable browser-compatible native fork](task-01-enable-browser-compatible-fork.md)

One work order, wave 1, sequential in this primary session. No delegates.

## Verification results

Design checkpoint (2026-10-18):

- `python3 scripts/list-docs.py validate`: exit 0, 388 decisions and 1652 specifications.
- `python3 scripts/lint-spec-files.py --all`: exit 0, all specification files passed.
- Repository `validateCoverage` preflight from the work order, using the four
  artifacts plus the planned runtime path: exit 0, `covered`, zero errors;
  both owning requirements accepted through the referenced design. This is a
  planning-input check, not a claim that runtime code changed.
- `git diff --check` and the scoped plan diff/status checks: exit 0; exactly
  the two owning spec files and the new two-file plan directory are dirty.
- The first preflight shell lacked `node` (exit 127). Calling the task-local
  Node 24.21.0 binary completed the preflight; no global environment change.

Implementation results (2026-10-18; ROOT lease granted):

- Fresh frozen workspace install: exit 0 (`pnpm` 9.15.9, Node 24.21.0).
- Permanent causal RED: corrected new suite, exit 1, exactly two missing-capability
  failures (desktop/touch zero requests, raw UUID TypeError) and three controls
  passing. Original command session 25344 joined; result `/tmp/child145-native-fork-red.json`.
- GREEN new suite: exit 0, 5 passed, original session 18274 joined. The planned
  `browser-locales` filter selected only this new suite; existing reviewed
  message-action tests belong to another project. Their separate unfiltered run
  exited 0, 2 files / 24 tests passed, original session 88878 joined. The work-order
  combined command now omits the project filter so it selects all three files.
- Scoped ESLint: exit 0, original session 58959 joined. Prettier reported the new
  file unchanged.
- Single web typecheck: exit 0, original session 92293 joined, unchanged
  rich-output check plus `tsc --noEmit`, 8 GiB heap, GNU 300-second timeout and
  10-second kill grace. No OOM, timeout or repeat typecheck.
- Final catalog/spec lint and diff checks: exit 0. Actual six-path
  `validateCoverage` preflight: exit 0, `covered`, zero errors, receipt
  `/tmp/child145-native-fork-coverage.json`.
- PR3897 remains open at admitted `da5e4b790b6fe9f01d3699683c774b3045f35b66`.
  Runtime diff is the native UUID import/call plus the independently authored
  test. No private qualification fixture was accessed or replayed.

All package commands use task-local Node 24 PATH; Vitest/ESLint use 2 GiB heaps.
An initial root-level `pnpm --dir` invocation selected the wrong Corepack pnpm
and exited before tests; running from `apps/web` resolved it without any
configuration change. The initial retry-test expectation was corrected to the
existing locale copy before the recorded causal RED. Normal active commit hooks
remain a delivery requirement; their actual receipt accompanies the published
head. No local E2E/build/Go or live-provider claim is made.

## Documentation impact

Internal requirement linkage, technical design, plan and work order only.
Public docs already describe the supported private-VPN HTTP path. No public
instructions, defaults, APIs, terminology, screenshots or localization change.
No ADR is needed: this reuses the established browser capability boundary.

## Risks

- Shared-file overlap with PR3897 requires current-patch compatibility assessment.
- Test isolation must restore crypto descriptors and browser listeners/timers.
- Transport-mocked coverage cannot establish live upstream fork availability.
- Local heavy checks require ROOT's global lease, task-local Node 24 PATH and
  original process handles/exits. Resource/auth/transport/incomplete or
  out-of-scope failures require a ROOT checkpoint before expanding budgets.
