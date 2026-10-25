---
id: "03-browser-evidence"
title: "Prove desktop and phone recovery"
status: done
wave: 3
depends_on:
  - 02-recovery-resolution
plan: "plan.md"
requirements:
  - REQ-TASKS-PROMPT-ATTACHMENTS-002
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002
acceptance_criteria:
  - AC-TASKS-PROMPT-ATTACHMENTS-002.1
  - AC-TASKS-PROMPT-ATTACHMENTS-002.2
  - AC-TASKS-PROMPT-ATTACHMENTS-002.3
  - AC-TASKS-PROMPT-ATTACHMENTS-002.4
  - AC-TASKS-PROMPT-ATTACHMENTS-002.5
  - AC-TASKS-PROMPT-ATTACHMENTS-002.6
  - AC-TASKS-PROMPT-ATTACHMENTS-002.7
  - AC-TASKS-PROMPT-ATTACHMENTS-002.8
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.16
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.17
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.19
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.33
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002.3
system_design:
  - ../../specs/tasks/system-design/prompt-attachments.md
  - ../../specs/agents/system-design/session-startup-failure-explanations.md
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
---

# Task 03: Prove desktop and phone recovery

## Summary

Exercise failed-start recovery with real uploaded attachments in disposable
browser fixtures. Prove agent delivery and resolved recovery presentation together.

## In scope

- Add deterministic failed-bootstrap and captured-attachment evidence fixtures.
- Cover PNG plus path-delivered file, attachment-only input, unavailable delivery,
  read-only restoration, reload/reconnect, and one original user message.
- Prove the restored composer retains drafts and selected attachments.
- Compare desktop and native phone results against UI-01 and UI-02.
- Update public recovery guidance when implementation passes.
- Record results and synchronize all three work orders with the plan.

## Out of scope

Live task manipulation, full-suite verification, new provider claims, real
provider credentials, and new persistent Kandev tasks or sessions.

## Acceptance

1. Both browser projects prove captured replay descriptors/materialized bytes,
   one original user message, dated history, and no stale recovery controls.
2. Failed delivery owns its own error; workspace-only restoration retains the
   agent warning. Phone controls remain reachable with targets of at least 44px.
3. Public guidance matches delivered behavior. Every listed validation passes,
   and the plan records actual command results before implementation completion.

## ASCII UI preview

UI-01: Task Chat after failed initial startup. Source evidence supports the before state.

```text
Desktop, before retry
  Transcript: [Screenshot] original request
  [Startup failure record]
  [Recovery card: Start fresh session | Restore workspace]

Desktop, after successful retry
  Transcript: [Screenshot] original request (one user row)
  [Original startup failure: historical details]
  [Agent response]
  [Editable composer: existing draft and selected attachments]

Phone, after successful retry
  [Task header / Chat]
  [Screenshot and file labels]
  [Original request: one row]
  [Historical error / details]
  [Agent response]
  [Editable composer]
  [Phone navigation]
```

UI-02: Pending recovery and successor failure use the current composer region.

```text
Desktop pending: [Recovery cause | Working... | disabled alternatives]
Phone pending:
  [Recovery cause]
  [Working...]
  [Disabled alternatives, stacked]

Delivery failure after readiness:
  [New delivery error, its own details and eligible actions]
```

The transcript owns vertical message scrolling. The composer/recovery region and
phone navigation retain their current layout and safe-area clearance. Historical
errors retain readable details without recovery controls. These structural
outcomes are required; spacing and example copy are illustrative.
Phone targets remain at least 44px. No new picker, drawer, or route is introduced.
The existing image preview uses its current dismissal and focus-return behavior.
Maps to AC-TASKS-PROMPT-ATTACHMENTS-002.6/.8 and agent criteria 006.16/.17/.19/.33.

Full view: [plan](plan.md#ascii-ui-preview). Phone entry is the existing task Chat
view, not the desktop workbench squeezed into a viewport. Use
`mobile/session-mobile-layout.tsx` and current image-dialog behavior.

## Verification

From the repository root, run sequentially:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/session/fresh-start-submission-recovery.spec.ts tests/session/session-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/session/mobile-fresh-start-submission-recovery.spec.ts tests/task/mobile-launch-failure-recovery.spec.ts)
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

The managed runner builds current backend and Vite assets. Never overlap full
suites or override its worker budget. Use causal HTTP/WS waits. A screenshot
visible before recovery does not prove agent delivery. Use the mock agent's
existing authenticated fixture boundary for captured-input or byte evidence.

## Files likely touched

- `apps/web/e2e/tests/session/fresh-start-submission-recovery.spec.ts` (new)
- `apps/web/e2e/tests/session/mobile-fresh-start-submission-recovery.spec.ts` (new)
- `apps/web/e2e/helpers/session-error-recovery-ui.ts` only for shared fixture needs
- `apps/backend/cmd/mock-agent/` only for necessary deterministic capture behavior
- `apps/backend/cmd/mock-agent/AGENTS.md` if its scenario contract changes
- `docs/public/tasks-and-workflows.md`
- This plan and its work-order result sections

## Dependencies

Tasks 01 and 02. Read `/e2e` and `/mobile-parity` before test implementation.
Read the mock-agent scoped guide before fixture changes.

## Risks

Mock fixtures must inspect actual delivery, rather than echo the submitted UI
preview. Isolate seed data and never use the reported task as a browser fixture.

## Parallelism

`sequential`

## Inputs

Current session-recovery tests, preparation attachment preview tests, mock-agent
scenarios, public recovery guidance, and UI-01/UI-02 in this package.

## Results

Done. The desktop run passed all 9 tests: two fresh-start cases and seven existing
session-recovery cases. The phone run passed all 4 tests: fresh-start recovery and
three existing task launch-recovery cases. Both managed runs built the backend and
production web bundle before Playwright.

The new cases prove that image and path-delivered file bytes reach the mock agent.
They also prove one original user message, retained startup history, unavailable-file
refusal, read-only restoration, and composer draft plus attachment persistence.
Phone assertions cover touch target size and horizontal overflow.

Public recovery guidance is updated in `docs/public/sessions-and-review.md` and
`docs/public/tasks-and-workflows.md`.
