---
id: "01-align-conversation-progress"
title: "Align conversation progress"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002
acceptance_criteria:
  - AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.12
system_design:
  - ../../specs/platform/system-design/session-subscription-recovery.md
---

# Task 01: Align conversation progress

## Summary

Align the grid spinner with conversation loading and retrying labels on desktop and phone.
Add rendered regression coverage before the local markup correction.

## In scope

- The spinner wrapper in `SessionHistoryFeedback`.
- Loading and retrying geometry in the existing desktop and phone recovery suites.
- Successful history recovery after the geometry assertions.

## Out of scope

Global spinner CSS, animation, copy, retry policy, message state, and other loading rows.

## Acceptance

1. The actual spinner and label centers differ by at most 1 CSS pixel during loading and retrying on desktop and phone.
2. The row retains its translated label, polite status region, and `aria-hidden` spinner wrapper.
3. History appears after the held response arrives, and the phone document has no horizontal overflow.

## ASCII UI preview

UI-01: Open an existing task, loading or retrying. Desktop and phone share this row.

```text
After:      [grid]   Loading conversation...
Retrying:   [grid]   Taking longer than usual. Retrying...
```

The grid and label share a vertical center. Existing chat composition, scroll ownership, and safe-area handling stay in place.
The phone label can wrap inside the row. Drawing spacing and text length are illustrative.
This preview covers AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.12.
See the [full before/after preview](plan.md#ascii-ui-preview).

## Implementation

1. Add `centers conversation progress during loading and retrying` to both existing recovery suites.
2. Seed a settled history task through the existing fixture.
3. Drop the first scoped `message.list` response before opening its session.
4. After the proxy records the drop, hold subsequent history responses.
5. Compare `.spinner-grid` and label bounding boxes during loading and automatic retry.
6. Release the held response and check that the conversation appears.
7. Run the scenarios against the unchanged component and record the expected alignment failure.
8. Add `className="flex"` to the existing `aria-hidden` spinner wrapper in `SessionHistoryFeedback`.
9. Run the required commands and record their results.

Use the existing response proxy and causal counters. Do not add fixed waits or a new proxy mode.
Keep existing recovery scenarios and touch-target checks intact.

## Verification

The workspace already has dependencies. For a fresh checkout, run `pnpm install --frozen-lockfile` from `apps/` first.
Run these commands from the repository root:

```bash
(cd apps/web && pnpm exec vitest run components/task/chat/session-entry-feedback.test.tsx components/task/chat/message-list-shared.test.tsx)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings components/task/chat/session-entry-feedback.tsx e2e/tests/session/session-entry-recovery.spec.ts e2e/tests/session/mobile-session-entry-recovery.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/session/session-entry-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-session-entry-recovery.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/conversation-loading-alignment
```

Run desktop and phone E2E commands sequentially. Managed runs rebuild production assets and use isolated backends.
Inspect the rendered desktop and phone rows against UI-01. Record the expected RED and final GREEN results.
Run the documentation coverage preflight with the planned component path and current documents:

```bash
node <<'NODE'
const fs = require("node:fs");
const { validateCoverage } = require("./.github/scripts/pr-docs.cjs");
const packagePath = "docs/plans/conversation-loading-alignment/";
const docs = [
  packagePath + "plan.md",
  packagePath + "task-01-align-conversation-progress.md",
  packagePath + "task-02-clarify-recovery-progress.md",
  "docs/specs/platform/requirements/session-subscription-recovery.md",
  "docs/specs/platform/system-design/session-subscription-recovery.md",
  "docs/specs/agents/requirements/session-recovery-failures.md",
  "docs/specs/agents/system-design/session-recovery-guidance.md",
];
const result = validateCoverage({
  changedFiles: [
    "apps/web/components/task/chat/session-entry-feedback.tsx",
    "apps/web/components/task/recovery-actions.tsx",
    ...docs,
  ]
    .map(filename => ({ filename, status: "modified" })),
  fileContents: Object.fromEntries(docs.map(filename => [filename, fs.readFileSync(filename, "utf8")])),
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
```

This check must resolve the plan, work order, requirement, acceptance criterion, and system design references.

## Files likely touched

- `apps/web/components/task/chat/session-entry-feedback.tsx`
- `apps/web/e2e/tests/session/session-entry-recovery.spec.ts`
- `apps/web/e2e/tests/session/mobile-session-entry-recovery.spec.ts`
- This work order and `plan.md` for execution status and results.

## Dependencies

None.

## Risks

The initial request must finish its dropped-response path before the test holds later responses.
Measure the spinner itself. Wrapper-only measurements cannot detect this defect.
Retain the accessibility wrapper and keep the correction local.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/platform/requirements/session-subscription-recovery.md), AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.12.
- [System design](../../specs/platform/system-design/session-subscription-recovery.md#presentation).
- `apps/web/components/task/chat/session-entry-feedback.tsx` and its existing unit tests.
- `apps/web/e2e/helpers/session-entry-recovery.ts` and both existing recovery suites.
- `apps/web/components/task/task-layout.tsx`, the dedicated phone chat exemplar.
- [Root-cause evidence](plan.md#evidence-and-root-cause).

## Results

The new browser check failed on the unchanged wrapper at desktop and phone widths. Both measured a 4.484375-pixel center offset. Adding `flex` to the existing `aria-hidden` wrapper reduced the difference to at most 1 CSS pixel in loading and retrying states.

Passed:

- `pnpm exec vitest run components/task/chat/session-entry-feedback.test.tsx components/task/chat/message-list-shared.test.tsx` (64 tests).
- Targeted oxlint for the component, geometry helper, and desktop/phone recovery specs.
- `pnpm run typecheck`.
- Desktop session-entry recovery E2E (5 tests) and phone recovery E2E (3 tests), with the managed production build run before the final full-spec checks.
- `python3 scripts/list-docs.py validate`.
- `python3 scripts/lint-spec-files.test.py` (36 tests).
- `python3 scripts/lint-spec-files.py --all`.
- Documentation coverage preflight: covered, no errors.
- `git diff --check`.
- PR CI fixup: the alignment test now drops the first history reply and holds later replies before opening the session, keeping loading and retrying states stable during geometry checks. The focused desktop and phone cases passed (one each); the phone case passed three consecutive repeats.
