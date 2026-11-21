---
created: 2026-11-12
status: done
requirements:
  - REQ-OFFICE-AUTOMATION-RUNS-001
  - REQ-UI-EMPTY-TURN-NOTICE-001
system_design:
  - ../../specs/office/system-design/automation-run-output.md
  - ../../specs/office/system-design/automation-runs.md
  - ../../specs/ui/system-design/empty-turn-notice.md
legacy_specs: []
---

# Implementation plan: Automation passthrough output guidance

## Overview

[Issue #5291](https://github.com/kdlbs/kandev/issues/5291) reports successful
automation runs with an empty transcript and a false no-output warning.
The authenticated GitHub account is `carlosflorencio`. The issue was assigned
to that account and the assignment was verified on 2026-11-12.

This package delivers the issue's accepted minimum: truthful capture guidance
and suppression of false empty-turn feedback. Historical terminal output remains
unavailable. It does not deliver terminal replay or a recorded final summary.

Implement the frontend correction first. Then add persisted-mode fixtures and
focused desktop/phone browser regressions. Both work orders are sequential.
The package is implemented and verified. Changes remain unstaged and uncommitted.

## Confirmed cause and reproduction

- `RunTranscript` always mounts `MessageList` and the reply composer. It never
  checks the selected session's passthrough mode or explains absent capture.
- PTY output does not create the stored agent messages that the list reads.
  `Service.turnHadOutput` measures those stored messages, so a passthrough
  completion can correctly carry `had_output=false` despite terminal output.
- `maybeEmitEmptyTurnNotice` supplies no mode to `computeEmptyTurnNotice`.
  A passthrough completion therefore creates the same warning as an empty
  structured turn.
- The existing terminal WebSocket calls `EnsurePassthroughExecution`.
  Embedding it in this reading surface can start or restart a process.
  `GetPassthroughBuffer` requires an existing execution and process. It does
  not provide durable, run-scoped history.

A temporary test used the real component/store with only session API and
settings-loading mocks. A parked session had `is_passthrough=true`. The component
rendered no capture explanation. A real-store completion with `had_output=false`
created one synthetic warning when zero were expected. The identical structured
session control correctly created one warning.

Command, run from `apps/web` on the unchanged implementation:

```bash
pnpm exec vitest run components/runs/issue-5291-repro.test.tsx \
  components/runs/run-transcript.test.tsx \
  lib/ws/handlers/empty-turn-notice.test.ts --reporter=dot
```

Result: two expected regression failures, 33 passing tests, exit 1.
The temporary reproduction file was removed after the run. Its command records
diagnostic evidence, not a command for the completed work orders.
The existing transcript fixture also logged an incomplete `listTaskSessions`
mock. The two failures were the missing explanation and the incorrect warning count.

No live user instance, provider account, or scratch-workspace trust prompt was
used. The reported folder-trust and early-exit state faults were not reproduced.

## Scope

### In scope

- Selected-session capture guidance for running and finished passthrough runs.
- Shared notice suppression at creation and rendering, including late hydration.
- Retained prompts, errors, ordinary messages, run status, and reply controls.
- All locale catalogs, public automation guidance, and desktop/phone regressions.

### Out of scope

- Capturing PTY bytes, terminal replay, provider JSONL ingestion, or old-output recovery.
- Live terminal attachment, new buffer endpoints, and agent lifecycle changes.
- Scratch-folder trust handling and settlement after early process exit.
- Run summaries, feed placeholders, automation scheduling, and continuation policy.
- Commit, push, PR creation, or persistent follow-up tasks.

## Technical approach

Office owns the run-reading outcome. Extend its existing requirement with
AC-OFFICE-AUTOMATION-RUNS-001.13 through .18. The reusable notice exclusion belongs
to UI under AC-UI-EMPTY-TURN-NOTICE-001.11. No new ADR or persistence contract is needed.

`RunTranscript` retains `useEnsureTaskSession` and its current chat primitives.
Use `isPassthroughSession(panelState.session)` for the informational block.
Keep stored messages and the composer below it. Add two translated keys in
`automations.json`; use the owning design's English copy as the translation source.

At creation, resolve the event's own session and pass `isPassthrough` to the
pure notice decision. At rendering, carry the same mode through
`ProcessedMessagesOptions` to `filterVisibleMessages`. Hide only entries marked
`metadata.empty_turn === true`. Include mode in memo dependencies so hydration
retracts an earlier warning without a new message array or store deletion.
Missing mode retains current notice behavior until the session loads.

Mode must come from the session, with explicit false precedence. Do not read
the automation's current profile, the selected global task, or run status as mode.

| Session/transport | Behavior | Evidence and fallback |
| --- | --- | --- |
| Claude Code with `is_passthrough=true` | Capture guidance, no empty-turn warning | Component, real-store completion, persisted browser fixture |
| Another provider with passthrough | Same behavior from the session flag | Provider-neutral helper tests, no provider transcript assumptions |
| Legacy row without the flag, snapshot `cli_passthrough=true` | Same guidance | Unit/component tests for the existing snapshot fallback |
| Explicit false with conflicting snapshot true | Structured transcript and normal notice rules | Helper and handler precedence tests |
| Structured ACP or native structured transport | Existing messages and empty-turn feedback | Existing notice tests and structured browser control |
| Session row not loaded | Existing loading behavior, then reevaluate | Deferred hydration and direct URL tests |

The parent automation design is near its size limit. Its new link delegates
capture guidance to a companion design under the same Office requirement.
The linked sidebar-definition package owns different acceptance criteria and
needs no status or scope changes. Existing failure-recovery notice criteria
remain covered by their current regressions.

## ASCII UI preview

UI-01: Selected passthrough run, `/automations/<id>?run=<run-id>`.
The source confirms the current transcript lacks capture guidance.

```text
CURRENT CONTENT                  PROPOSED CONTENT
+---------------------------+    +-------------------------------+
| Stored prompt             |    | CLI passthrough output is not |
| Empty-output warning      |    | captured in this transcript.  |
|                           |    | Guidance for future sessions  |
| [Reply to this run]       |    +-------------------------------+
+---------------------------+    | Stored prompt / messages      |
                                 | Real errors remain visible    |
                                 | [Reply to this run]           |
                                 +-------------------------------+
```

UI-02: Phone composition, same selected run, running or parked.

```text
+-------------------------------+
| Automation name        [Runs] |
| Selected run time / status    |
+-------------------------------+
| Capture explanation          |
| Future-session guidance      |
+-------------------------------+
| Stored prompt / messages     |
|                              |
|       transcript scroll      |
+-------------------------------+
| [Reply to this run]    [Send] |
+-------------------------------+
```

Desktop retains its runs rail. Phone retains the existing runs drawer and
full-height reader. The explanation wraps above the message region. The message
region owns scrolling, and the composer retains its safe-area behavior.
The notice is informational and has no new control. Loading retains the current
surface until mode is known. Structured runs omit the capture block.

These structures implement AC-OFFICE-AUTOMATION-RUNS-001.13 through .18.
Spacing and shortened preview copy are illustrative. The design specifies the
localized text. The phone adds no stacked desktop pane or terminal key bar.

## Tests

| Criteria | Permanent regression evidence to add |
| --- | --- |
| Office .13, .14, .18 | `run-transcript.test.tsx`: `explains uncaptured output for a parked passthrough run`, running-state variant, delayed hydration, profile-edit independence, and selection between modes |
| Office .15, UI .11 | `empty-turn-notice.test.ts`: pure passthrough exclusion; `turns.test.ts`: event-session isolation and explicit false precedence |
| Office .15, .16, UI .11 | `processed-message-filtering-passthrough.test.ts`: hide synthetic empty-turn entries while retaining user text, agent text, and genuine errors |
| Office .15, UI .11 | `use-processed-messages.test.ts`: changing only mode retracts a warning; component test covers the full late-hydration path |
| Office .16 | `run-transcript.test.tsx`: preserve composer/status/errors and never construct a terminal connection during viewing |
| Office .18 | `is-passthrough-session.test.ts`: retain existing explicit false and snapshot fallback cases |
| Office .13, .18 fixture | `e2e_task_session_passthrough_test.go`: omitted, true, and false fixture modes persist and round-trip |

## E2E tests

Work order 02 adds `automations-passthrough-output.spec.ts` (`chromium`) and
`mobile-automations-passthrough-output.spec.ts` (`mobile-chrome`, configured Pixel 5).
Use the existing API seed path with an optional mode field. Persist hidden task
origin and mode before navigation; do not patch the browser store or intercept
session reads to manufacture the result.

Both projects cover direct load/reload, passthrough versus structured run selection,
retained prompts/status/composer, and separate running and parked fixtures.
Component/handler tests own real completion dispatch and hydration races.
Observe terminal sockets and start/resume requests before navigation. Use causal
hydration waits and the sanctioned negative-observation helper for absence checks.
The test must not connect to a provider or start a PTY merely to read guidance.

Phone coverage opens the existing runs drawer with touch and selects a different
run. It verifies dismissal/focus, long translated guidance, and composer containment
without horizontal page overflow. These flows cover Office .13
through .18. Existing desktop and phone detail specs run alongside the additions.

## Work orders

- [x] [Task 01: Explain passthrough capture and exclude false notices](task-01-explain-passthrough-output.md)
- [x] [Task 02: Prove persisted run guidance on desktop and phone](task-02-cover-run-output-in-browsers.md) (`done`)

Work order 02 depends on work order 01. Both work orders are `done`.
No subagents are authorized.

## Verification results

Implementation and design checks on 2026-11-12 passed:

- Focused frontend suite: 171 tests passed across seven files.
- Backend fixture persistence test passed; changed Go files reported 0 lint issues.
- Web typecheck, focused Oxlint, `i18n:check`, and `i18n:ratchet` passed.
- Managed production builds completed for the backend and Vite web assets.
- Desktop Chromium run tests: 9 passed, including the passthrough output regression.
- Pixel 5 mobile run tests: 8 passed, including the passthrough output regression.
- The captured desktop and phone views show guidance above the stored conversation.
  Phone guidance wraps within the reader; the drawer remains usable, the composer
  stays inside the viewport, and the document has no horizontal overflow.
- Passive transcript reading created no terminal WebSocket or session
  launch/resume/restart request during the browser observation windows.
- Public documentation tests: 63 passed; all 55 published docs pages validated.
- Catalog validation: 450 decisions and 1996 specifications validated.
- Specification-linter tests: 36 tests passed.
- Full specification lint: all files passed.
- Documentation coverage preflight: actual implementation paths were `covered`
  with no reference errors.
- `git diff --check` passed. Package files remain unstaged and uncommitted.

The work orders contain exact implementation commands. Design-package gates,
run from the repository root:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/automation-passthrough-output
git status --short -- docs/plans/automation-passthrough-output
```

Run this local coverage preflight from the repository root. It checks actual
documentation changes and projected implementation coverage independently.

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/specs/office/requirements/automation-runs.md',
  'docs/specs/office/system-design/automation-runs.md',
  'docs/specs/office/system-design/automation-run-output.md',
  'docs/specs/ui/requirements/empty-turn-notice.md',
  'docs/specs/ui/system-design/empty-turn-notice.md',
  'docs/plans/automation-passthrough-output/plan.md',
  'docs/plans/automation-passthrough-output/task-01-explain-passthrough-output.md',
  'docs/plans/automation-passthrough-output/task-02-cover-run-output-in-browsers.md',
];
const changedFiles = paths.map(filename => ({ filename, status: 'modified', additions: 1, changes: 1 }));
const fileContents = Object.fromEntries(paths.map(path => [path, fs.readFileSync(path, 'utf8')]));
const docs = validateCoverage({ changedFiles, fileContents });
const projected = validateCoverage({
  changedFiles: [...changedFiles,
    { filename: 'apps/web/components/runs/run-transcript.tsx', status: 'modified' },
    { filename: 'apps/backend/internal/backendapp/e2e_reset.go', status: 'modified' },
  ],
  fileContents,
});
console.log(JSON.stringify({ docs: { status: docs.status, errors: docs.errors },
  projected: { status: projected.status, errors: projected.errors } }, null, 2));
if (!docs.ok || docs.status !== 'exempt' || !projected.ok || projected.status !== 'covered') process.exitCode = 1;
NODE
```

At implementation completion, run the same evaluator with the actual changed
production/test paths as well. Projected coverage does not prove runtime behavior.

## Risks

- This repair explains missing capture. It does not make a final PTY summary readable.
- Existing reusable passthrough sessions can retain their mode after a profile edit.
  Guidance must refer to future sessions, without promising that the next firing changes mode.
- A completion event can precede hydration. Creation-only suppression leaves an
  existing warning visible, so rendering needs its own tested guard.
- Filtering every status would conceal real failures. Filter only the synthetic marker.
- `useProcessedMessages` is near the frontend file-size limit. Extract a focused
  helper if necessary, without changing message ordering or unrelated filters.
- The browser fixture must persist session mode. Client-only injection cannot prove reload behavior.

## Public documentation impact

During work order 01, update `docs/public/automation-and-mcp.md` near run-reading
guidance. This is a how-to clarification: passthrough output is not recorded,
and future recorded sessions need a profile with CLI passthrough disabled.
The root README and screenshot catalog were searched. Neither needs a new
image or navigation change for this informational block.
