---
id: "01-guard-composition"
title: "Guard shortcut composition and prove Task Create"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-KEYBOARD-SHORTCUT-COMPOSITION-001
acceptance_criteria:
  - AC-UI-KEYBOARD-SHORTCUT-COMPOSITION-001.1
  - AC-UI-KEYBOARD-SHORTCUT-COMPOSITION-001.2
  - AC-UI-KEYBOARD-SHORTCUT-COMPOSITION-001.3
  - AC-UI-KEYBOARD-SHORTCUT-COMPOSITION-001.4
  - AC-UI-KEYBOARD-SHORTCUT-COMPOSITION-001.5
system_design:
  - ../../specs/ui/system-design/keyboard-shortcut-composition.md
---

# Task 01: Guard shortcut composition and prove Task Create

## Summary

Reject both composition signals at the reusable native/React hook boundary
without consuming the event. Prove real Task Create submit wiring remains idle
for composition and reaches the injected create boundary once for an ordinary
chord; verify the shipped desktop/phone path with disposable E2E data.

## In scope

One private hook predicate, two admission calls, independent causal RED/GREEN
regressions, ordinary compatibility controls, focused desktop/mobile E2E and
screenshots, the small public how-to clarification, and accurate package results.

## Out of scope

Matcher changes, consumer production edits, ProseMirror/TipTap, unused code,
other composition handlers, timing/state, geometry/copy, backend/schema,
dependencies, broad audits, unchanged validator unit suites, and delegation.

## Acceptance

1. Real native and React dispatch independently reject composing/keyCode13 and
   noncomposing/keyCode229, for Ctrl and Meta, leaving default and propagation
   unconsumed even with consumption options enabled.
2. Real setup/hook/submit integration observes zero create calls for rejection
   and exactly one ordinary eligible creation; disabled, compatibility,
   defaultPrevented, targets/capture, and modifier behavior retain their baseline.
3. Focused desktop/phone E2E leaves text/dialog intact for synthetic confirmation,
   creates exactly one disposable task on ordinary submission with reload proof,
   and preserves visible phone touch submission. Report physical IME limits.

## ASCII UI preview

Compact excerpts of [UI-01/UI-02 in the plan](plan.md#ascii-ui-preview), for
eligible creation with a prompt and manual title enabled:

```text
UI-01: Desktop modal (New Task)
+-------------------------------------------------------+
| Create Task                                           |
| [Source / branch/options]                             |
| Title: [Entered title]                                |
| Prompt: [Composed text]                               |
| [Workspace / Workflow / Step] [Agent / Executor]       |
| Existing advanced settings                            |
|-------------------------------------------------------|
|                         [Cancel] [Start Task] [v]     |
+-------------------------------------------------------+

UI-02: Phone full-height surface (FAB / navigation)
+---------------------------------+
| Create Task                     |
| [Source / branch/options]       |
| Title: [Entered title]          |
| Prompt: [Composed text]         |
| [Workspace / Workflow / Step]   |
| [Agent / Executor]              |
| Existing advanced settings      |
|---------------------------------|
| [            Cancel           ] |
| [          Start Task         ] |
| [          Plan Mode          ] |
| [         Create Only         ] |
+---------------------------------+
```

Composition leaves input text and dialog in place and does not consume the
event. An ordinary chord or visible touch action uses the existing submit
action. Preserve the existing body scroller, header/footer, localized labels,
conditional title/actions, safe-area and touch behavior. The desktop arrow
exposes the existing secondary actions. Spacing is illustrative; no layout/code
redesign is authorized. Capture both rendered views for
`AC-UI-KEYBOARD-SHORTCUT-COMPOSITION-001.4` and `001.5`.

## Files owned

- `apps/web/hooks/use-keyboard-shortcut.ts`
- `apps/web/hooks/use-keyboard-shortcut.test.ts`
- `apps/web/components/task-create-dialog-composition.test.tsx` (new)
- `apps/web/e2e/tests/task/task-create-composition.spec.ts` (new)
- `apps/web/e2e/tests/task/mobile-task-create-composition.spec.ts` (new)
- `apps/web/e2e/tests/task/task-create-composition-helpers.ts` (new)
- `docs/public/tasks-and-workflows.md` (Create a task sentence only)
- The four documents in this design package (statuses and actual results)

## Inputs and execution

Read the owning requirement/design, `/tdd` and `/e2e` before execution. Source
patterns: existing hook test, setup test, submit test, selectors test, and
`mobile-task-create-escape.spec.ts`. Do not copy their shortcut/submit mocks
into the actual-consumer test. Capture the injected `createTask` dependency of
the real submit hook and await its asynchronous chain. Use valid controlled
state and consent; external data, routing, and UI effects may be mocked.

Write regression tests before production changes. Run native, React, and
consumer RED separately, with independent cases for each composition signal
and ordinary positive controls. Configure events as bubbles/cancelable. For
ordinary `defaultPrevented` controls, native remains skipped and React retains
its existing dispatch behavior. Assert disabled hooks, explicit targets and
capture, nonmatching/plain/extra-modifier events, callback identity, and enabled
consumption options without changing those behaviors. Only then add the guard.
One desktop E2E RED runs against a fresh managed pre-fix build; selector/setup
failures do not count. Mobile runs only final GREEN unless an observed distinct
viewport failure justifies a further diagnostic. Browser evidence uses actual
dialog wiring and live create counters,
causal ordinary response waits, and persisted task uniqueness after reload.

## Verification

Commands are repo-root-relative unless wrapped with a directory. Implementation
requires the later explicit instruction and ROOT's single heavy lease. A fresh
worktree runs `(cd apps && pnpm install --frozen-lockfile)` once before package
commands. Use the supplied Node/pnpm environment; do not reinstall in design.

```bash
# RED before production edits; run serially and join each causal failure.
(cd apps/web && pnpm exec vitest run --maxWorkers=1 hooks/use-keyboard-shortcut.test.ts -t 'composition native')
(cd apps/web && pnpm exec vitest run --maxWorkers=1 hooks/use-keyboard-shortcut.test.ts -t 'composition React')
(cd apps/web && pnpm exec vitest run --maxWorkers=1 components/task-create-dialog-composition.test.tsx)

# RED rendered proof: one desktop run on a fresh managed pre-fix build.
(cd apps/web && pnpm e2e:run --project chromium tests/task/task-create-composition.spec.ts)

# GREEN and affected suites, after the minimal guard.
(cd apps/web && pnpm exec vitest run --maxWorkers=1 hooks/use-keyboard-shortcut.test.ts components/task-create-dialog-composition.test.tsx components/task-create-dialog-setup.test.ts components/task-create-dialog-submit.test.tsx components/task-create-dialog-selectors.test.tsx components/task-create-dialog.test.tsx)

# Final GREEN rendered proof: fresh managed builds, serial one-worker runs.
# Mobile has no RED command; its shared admission is covered by earlier REDs.
(cd apps/web && pnpm e2e:run --project chromium tests/task/task-create-composition.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-task-create-composition.spec.ts)

# Final scoped lint and web typecheck.
(cd apps/web && pnpm exec eslint --max-warnings 0 hooks/use-keyboard-shortcut.ts hooks/use-keyboard-shortcut.test.ts components/task-create-dialog-composition.test.tsx e2e/tests/task/task-create-composition.spec.ts e2e/tests/task/mobile-task-create-composition.spec.ts e2e/tests/task/task-create-composition-helpers.ts)
(cd apps/web && pnpm run typecheck)

# Stage only actual owned frontend files before ratchets to include new paths.
git add -- apps/web/hooks/use-keyboard-shortcut.ts apps/web/hooks/use-keyboard-shortcut.test.ts apps/web/components/task-create-dialog-composition.test.tsx apps/web/e2e/tests/task/task-create-composition.spec.ts apps/web/e2e/tests/task/mobile-task-create-composition.spec.ts apps/web/e2e/tests/task/task-create-composition-helpers.ts
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm run e2e:sleep-ratchet)

python3 scripts/list-docs.py validate
python3 scripts/list-docs.py specs --system ui --text keyboard-shortcut-composition --format paths
python3 scripts/lint-spec-files.py --all
node scripts/validate-public-docs.mjs
git diff --check
git diff --cached --check
git status --short -- docs/plans/keyboard-shortcut-composition

# Actual changed-path documentation-reference preflight, from repo root.
node <<'NODE'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/specs/ui/requirements/keyboard-shortcut-composition.md',
  'docs/specs/ui/system-design/keyboard-shortcut-composition.md',
  'docs/plans/keyboard-shortcut-composition/plan.md',
  'docs/plans/keyboard-shortcut-composition/task-01-guard-composition.md',
];
const changedFiles = [...new Set([
  ...execFileSync('git', ['diff', '--name-only', 'HEAD'], { encoding: 'utf8' }).trim().split('\n'),
  ...execFileSync('git', ['ls-files', '--others', '--exclude-standard'], { encoding: 'utf8' }).trim().split('\n'),
])].filter(Boolean);
const fileContents = Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')]));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify(result, null, 2));
if (!result.ok || result.errors.length) process.exitCode = 1;
NODE
```

Use the existing `.github/scripts/pr-docs.cjs` exported `validateCoverage` with
the actual changed paths and current contents of the four package documents
as shown in the local documentation-reference preflight. Require `ok=true` and no
reference errors; record whether the classification is covered or exempt.
Before publication, mark contract statuses active/current after actual ROOT
design review, finish this order and plan, and record every actual result.
No backend lint or unrelated test suites are local gates.

## Dependencies

None. One sequential order, no parallel workers.

## Risks

See the [plan](plan.md#risks). Preserve existing differences between hook paths.
Do not report synthetic-event evidence as physical OS IME verification.

## Parallelism

`sequential`

## Results

Implemented the private composition predicate and two early hook admission
checks; matcher and direct consumers remain unchanged. Native/React/real-consumer
REDs and the rendered desktop create-counter RED were causal. Final affected
Vitest: six suites, 103 tests passed; the corrected required-props fixture then
passed four focused tests and targeted ESLint. Desktop E2E: two passed. Phone
E2E: three passed, including the visible touch action. Persisted uniqueness after
reload is proved in disposable fixtures, not user data.

Final scoped ESLint, web typecheck (8192 MiB heap), staged i18n and E2E sleep
ratchets, catalog/discovery/specification lint, and public-doc validation passed.
Both current viewport captures were inspected/compressed. Final package-reference
preflight and diff results are recorded in the [plan](plan.md#verification-results).
The plan also records the renderer/selector diagnostics and synthetic OS IME
coverage limitation. No broader suites or unchanged validator unit tests ran.
