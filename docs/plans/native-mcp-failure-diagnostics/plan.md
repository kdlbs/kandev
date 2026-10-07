---
created: 2026-10-07
status: done
requirements:
  - REQ-AGENTS-MCP-PREP-002
  - REQ-AGENTS-MCP-PREP-003
  - REQ-AGENTS-MCP-PREP-004
system_design:
  - ../../specs/agents/system-design/agent-mcp-preparation.md
legacy_specs: []
---

# Implementation Plan: Native MCP failure diagnostics

## Overview

Retain the sanitized cause of native MCP preparation failures in saved task
results and backend logs, and show it in desktop/phone preparation details.
Deliver the backend contract first, then consume it in the UI and prove recovery.
Agents owns this repair because native command preparation and readiness are
agent runtime contracts; Tasks supplies persistence and presentation.

## Evidence and root cause

Task `fc663aba-e1dd-4668-a817-ea9fef7cee49`, session
`0cd8ea64-471f-4a07-9928-3f8aa650571f`, recorded an approval failure for
`plugin-atlassian-atlassian` at 12:27:31.752 UTC on 2026-10-07, after 559 ms.
Its saved step contains only `connection_failed`; verification was skipped.
The original native runner error is unavailable. A later read-only native
`list-tools` returned 19 tools. An isolated approval probe succeeded, so neither
probe establishes the incident's underlying command failure.

The diagnostic defect is confirmed in code: `nativeMCPRunnerFailure` discards
the Go cause, launch/retry progress saves only readiness codes, and the command
runner can overwrite a wait failure with a cleanup error. Frontend live and
hydration mappers deliberately discard legacy MCP error text. Separately,
recovery handlers return `error` while `ApiError.errorCode` reads `error_code`,
so a session-busy rejection produces generic failure feedback.

A deterministic regression injects `exec.ErrWaitDelay` into the runner and
asserts that the failed approval row retains its safe cause, stage and observed
zero exit code. Today those assertions fail because no diagnostic exists.
This is a retention reproduction, not attribution of the live incident.

## Scope

### In scope

- Safe typed runner diagnostics, primary/secondary causes and exit evidence.
- Launch/retry progress, session metadata, backend diagnostic logs and HTTP DTOs.
- Stage-accurate preparation text, safe detail rendering, busy recovery feedback,
  desktop/phone tests, localization and concise operator documentation.

### Out of scope

- Recovering already-discarded historical causes, raw stdout/stderr retention,
  arbitrary command/credential logging, and diagnosing the original failure.
- Changing timeout limits, process ownership, retry eligibility, prompt dispatch,
  readiness success criteria, OAuth mechanics or support for other agents.

## Technical approach

Implement the amended [system design](../../specs/agents/system-design/agent-mcp-preparation.md#retained-native-command-diagnostics-2026-10-07-draft).
Use optional `mcp_diagnostic` metadata instead of relaxing the frontend's legacy
MCP `error`/`output` suppression. Existing readiness codes remain compatible.
Normalize at the adapter boundary and carry the same object to failed rows,
logs and retry responses. No database migration or new diagnostic export source
is needed: backend logs use the existing diagnostic bundle source.

| Provider / transport | Identity and capability | Intended behavior | Evidence / fallback |
| --- | --- | --- | --- |
| Cursor ACP, eligible local/worktree | Exact owned native ID | Retain initial/retry command cause; same-session recovery | Injected runner, lifecycle, HTTP and browser tests |
| Cursor terminal, eligible local/worktree | Same approval/verification adapter | Initial command diagnostics retained; live reload remains unsupported | Shared adapter/lifecycle tests; existing recovery fallback |
| Cursor remote/container/different HOME | Ineligible host import | Existing isolation; no native host command | Existing gating tests |
| Other agents / historical rows | No diagnostic contract | Existing behavior; generic fallback without invented cause | Legacy hydration tests |

## ASCII UI preview

UI-01: Task conversation > expanded preparation > failed native approval.
AC-AGENTS-MCP-PREP-004.1/004.4. Current behavior is supported by the supplied
screenshot; the specific proposed error below is illustrative.

```text
Before, desktop and phone:
  x Approve server: plugin-atlassian-atlassian
    Could not connect to this MCP server.
    [Retry connection]
    The recovery action failed. Try again.

After, desktop:
  x Approve server: plugin-atlassian-atlassian
    Could not complete MCP server approval.
    Error details
    Operation: Approval    Stage: Waiting for command
    Exit status: 0
    exec: WaitDelay expired before I/O complete
    [Retry connection]
    Wait for the current turn to finish before retrying.

After, phone:
  x Approve server:
    plugin-atlassian-atlassian
    Could not complete MCP
    server approval.
    Error details
    Operation: Approval
    Stage: Waiting for command
    Exit status: 0
    exec: WaitDelay expired
    before I/O complete
    [      Retry connection      ]
    Wait for the current turn to
    finish before retrying.
```

Required structure: summary, selectable safe details, then existing recovery
controls and feedback. Details remain inside the existing preparation surface;
phone text wraps and actions stack, with no new overlay or vertical scroller.
Use task mobile conversation and `AgentMcpPrepareActions` as the shipped
exemplars. Keep the conversation's scroll/safe-area behavior, 28px desktop
actions and at least 44px phone/coarse-pointer targets. Spacing and sample cause
are illustrative, not pixel or localization specifications. No diagnostic means
the existing generic explanation with no empty details block. Successful retry
removes that server's current diagnostic; other rows remain intact.

## Tests

| Acceptance | Regression evidence |
| --- | --- |
| 004.1 | `cursor_native_mcp_diagnostic_test.go`: `TestNativeMCPDiagnosticRetainsWaitDelay`, exit errors, executable failures, unknown output and truncation |
| 004.2 | `cursor_mcp_diagnostic_test.go`: `TestCursorMCPDiagnosticSurvivesPreparation`, launch/retry events, serialization, logs and superseded attempts |
| 004.3 | `cursor_native_mcp_diagnostic_test.go`: `TestNativeMCPDiagnosticRedactsAndBoundsCauses`, secret/URL/path/control/invalid-UTF-8 cases and excluded native streams |
| 004.4 | `agent-mcp-prepare-actions.test.tsx`, WS/hydration tests and `session-mcp-actions.test.ts`: stage explanation, safe details, old fields suppressed, real busy envelope |
| 003.3 | `cursor_mcp_recovery_handlers_test.go`: preserve `error`, add `error_code`, keep guards and no command on busy rejection |

Each implementation work order records red/green results. Do not infer the
original incident was `exec.ErrWaitDelay`; the test's injected cause is deliberate.

## E2E tests

Extend `tests/session/agent-mcp-preparation.spec.ts` (`chromium`) and
`tests/session/mobile-agent-mcp-preparation.spec.ts` (`mobile-chrome`) using the
existing sanitized preparation/recovery fixtures. For AC-004.2/004.4, prove
details arrive live and survive reload, busy retry explains why it was rejected,
successful retry clears only the affected server, and long detail text stays
inside the phone viewport with reachable actions. Use transport/event-driven
waits. Preserve existing OAuth-terminal and conversation-identity scenarios.

## Work orders

- [x] [Task 01: Retain safe native command diagnostics](task-01-retain-command-diagnostics.md)
- [x] [Task 02: Show retained errors and truthful recovery feedback](task-02-show-command-diagnostics.md)

Run sequentially. Task 02 depends on Task 01's additive backend contract.
This follow-up extends completed agent-MCP preparation and setup-recovery work;
their historical validation results and completed scope remain unchanged.

## Verification results

Design-package checks on 2026-10-07:

- `python3 scripts/list-docs.py validate`: passed, 359 decisions and 1423 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage` against the actual design diff:
  passed as exempt; against the proposed backend/UI paths and new work orders:
  passed as covered, with both work orders and no reference errors.
- `git diff --check` and package path/status inspection: passed.

Task 01 backend implementation and race checks passed. Task 02 frontend
implementation, rendered verification and public documentation updates passed.
The requirements and system design remain draft as scoped by the design package.

Implementation checks on 2026-10-07:

- Frontend unit/component tests: 8 files and 42 tests passed; the focused
  stale-attempt diagnostic regression also passed.
- Frontend typecheck and targeted ESLint passed.
- `pnpm run i18n:zh-hant`, `pnpm run i18n:pseudo`, and `pnpm run i18n:check`
  passed.
- Desktop Chromium and mobile Chrome preparation E2E suites passed, 2 tests
  each, including reload, busy feedback, per-server clearing, phone wrapping,
  overflow, and measured action targets.
- `pnpm --filter @kandev/web build:vite` passed.
- Public docs tests and validation passed (62 tests; 47 pages).
- `python3 scripts/list-docs.py validate`,
  `python3 scripts/lint-spec-files.py --all`, and `git diff --check` passed.

PR review follow-up on 2026-10-07:

- Context-ended lifecycle fences now retain the safe native command diagnostic
  while stale-generation fences continue to discard it. Cleanup errors no
  longer prevent command exit/output classification, and their safe detail is
  retained. Targeted race tests pass for MCP config, lifecycle and handlers.
- The cleanup-error separator is localized, with component and phone rendered
  coverage. Desktop E2E checks that legacy raw error/output fields stay hidden
  before and after reload.
- The first PR CI run's three failed jobs were not started because their hosted
  runners repeatedly failed acquisition after five attempts. Other jobs were
  still pending at inspection time, so that run does not count as successful
  validation. A fresh PR run is required after this fixup.

## Risks

- Sanitizing too late leaks a cause into logs or metadata. Bound and redact it
  before it leaves the adapter, including injected runner results.
- Trusting legacy `error` fields defeats deliberate MCP output suppression.
- A cleanup error can erase the primary cause or fabricate an exit code unless
  the runner captures process state before cleanup.
- Stale progress or a busy retry must not overwrite current failure details.
- Lost historical causes cannot be restored by this change.
