---
id: "01-retain-answer-drafts"
title: "Retain clarification answer drafts"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001
  - REQ-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-002
acceptance_criteria:
  - AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001.4
  - AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-002.1
  - AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-002.2
  - AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-002.3
  - AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-002.4
  - AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-002.5
  - AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-002.6
  - AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-002.7
system_design:
  - ../../specs/tasks/system-design/clarification-response-reliability.md
---

# Task 01: Retain clarification answer drafts

## Summary

Move active question-answer retention from task-layout lifetime to application-store
lifetime. Deliver the regression fix, auth isolation, desktop/phone navigation
proof, and public guidance as one focused change.

Read the [plan](plan.md),
[requirements](../../specs/tasks/requirements/clarification-response-reliability.md),
and [draft design](../../specs/tasks/system-design/clarification-response-reliability.md#unsubmitted-drafts-across-navigation).
The confirmed loss is the provider's task-ID cache reset and task-page unmount.

## In scope

- Add the bounded cache and one store-lifetime authority observer. Use the
  application's actual store as owner. Replace auth cache generations even
  while no task page is mounted.
- Adapt the provider, draft hook, and overlay restoration path. Retain exact
  selected options, raw custom text, and question index across client navigation.
- Keep local resize ownership and mounted task-layout identity.
- Prove exact source isolation, eviction, settlement, failed submission retention,
  and stale completion fencing with focused tests.
- Add navigation scenarios to existing desktop and mobile clarification specs.
  Use the plan's scenario matrix and real task navigation controls.
- Add a small retention note under the existing public clarification section.

## Out of scope

- Backend or wire changes, durable storage, new controls, and new copy in the app.
- New Quick Chat, Inbox, or historical late-answer retention behavior.
- Panel sizing or task-navigation policy changes.

## Acceptance

1. The permanent provider regressions fail before implementation for lost text,
   choices, and position. They pass after integration without relying on a
   component that remains mounted throughout navigation.
2. The actual overlay restores only eligible source answers. Auth transitions,
   settlement, capacity eviction, and delayed callbacks cannot expose another
   source draft or revive a cleared generation. Existing submission behavior passes.
3. Chromium and Pixel 5 scenarios retain and submit the original answers through
   real client navigation. All commands below pass, and their results replace Pending.

## ASCII UI preview

`UI-01: Desktop pending answers`, excerpt from the
[combined preview](plan.md#ascii-ui-preview). Covers REQ-002 `.1`, `.3`, and `.7`:

```text
Task A -> Task B -> Task A
+--------------+--------------------+
| Task A       | Question 2 of 3    |
| Task B       | Q1: PostgreSQL     |
|              | Custom: [Use Go   ]|
|              | Back / Next       |
+--------------+--------------------+
```

`UI-02: Phone pending answers`, same criteria:

```text
+----------------------------+
| Task A [task picker]        |
| Conversation               |
| Question 2 of 3            |
| Custom: [Use Go           ]|
| [Back] [Next]              |
| Existing composer/actions  |
+----------------------------+
```

Keep the shipped picker drawer, internal question scroller, safe-area clearance,
and touch targets. These structures are requirements. Text and spacing are illustrative.
Compare the rendered phone state with `UI-02` during the existing E2E check.

## Verification

Start at the repository root. On a fresh worktree without dependencies, first run
`(cd apps && pnpm install --frozen-lockfile)`.

During RED, add permanent tests named `retains unsubmitted answers across task
navigation` and `retains unsubmitted answers after task-page unmount` to
`clarification-draft-context.test.tsx`. Keep one actual `StateProvider` across
the transitions, while task providers mount and unmount. Run:

```bash
(cd apps/web && pnpm exec vitest run components/task/chat/clarification-draft-context.test.tsx)
```

Record failures caused by lost state. Replace the old test that explicitly
expects task changes to erase answers. Keep its assertion that the workbench
does not remount, and preserve resize tests that still expect task-local resets.
After the minimum implementation, run these commands sequentially:

```bash
(cd apps/web && pnpm exec vitest run \
  lib/clarification/answer-draft-cache.test.ts \
  components/task/chat/clarification-draft-context.test.tsx \
  components/task/chat/clarification-input-overlay.test.tsx \
  components/task/chat/clarification-custom-input.test.tsx \
  components/task/chat/clarification-panel-section.test.tsx \
  hooks/domains/session/use-clarification-group.test.ts \
  hooks/domains/session/use-clarification-group.regressions.test.ts \
  hooks/domains/session/use-clarification-group.timeout.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 \
  lib/clarification/answer-draft-cache.ts \
  lib/clarification/answer-draft-cache.test.ts \
  lib/state/store.ts \
  components/task/chat/clarification-draft-context.tsx \
  components/task/chat/clarification-draft-context.test.tsx \
  components/task/chat/clarification-input-overlay.tsx \
  components/task/chat/clarification-input-overlay.test.tsx \
  e2e/tests/chat/clarification.spec.ts \
  e2e/tests/chat/mobile-clarification.spec.ts)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/clarification.spec.ts -- --grep 'question draft navigation')
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-clarification.spec.ts -- --grep 'question draft navigation')
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short
```

Managed E2E builds fresh production assets and isolates test data. Do not run
desktop and mobile commands concurrently. Confirm that both select the intended
new scenarios and record actual test counts. Retain existing submit-failure
and choice-only coverage rather than replacing those tests.

Run this delivery-package preflight from the repository root. Its planned
source path forces cross-reference validation even in a documentation-only diff:

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const base = 'docs/plans/clarification-answer-drafts';
const workOrder = `${base}/task-01-retain-answer-drafts.md`;
const paths = [
  `${base}/plan.md`, workOrder,
  'docs/specs/tasks/requirements/clarification-response-reliability.md',
  'docs/specs/tasks/system-design/clarification-response-reliability.md',
];
const result = validateCoverage({
  changedFiles: [workOrder, 'apps/web/components/task/chat/clarification-draft-context.tsx'],
  fileContents: Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')])),
});
console.log(JSON.stringify({ status: result.status, errors: result.errors }));
if (!result.ok || result.status !== 'covered') process.exitCode = 1;
NODE
```

## Files likely touched

- New `apps/web/lib/clarification/answer-draft-cache.ts` and its `.test.ts`.
- `apps/web/lib/state/store.ts` for the one-time authority observer.
- `apps/web/components/task/chat/clarification-draft-context.tsx` and its `.test.tsx`.
- `apps/web/components/task/chat/clarification-input-overlay.tsx` and its `.test.tsx`.
- `apps/web/e2e/tests/chat/clarification.spec.ts`.
- `apps/web/e2e/tests/chat/mobile-clarification.spec.ts`.
- `docs/public/tasks-and-workflows.md`, under Answer clarification questions.
- This work order and `plan.md` for statuses and results.

## Dependencies

None. Existing clarification normalization, response authority, and late-message
admission remain dependencies without implementation changes.

## Risks

- Empty initial hook state can replace a retained snapshot before restoration.
- An auth observer tied to task mounting misses logout/login while offscreen.
- Uncaptured keys or generations can let delayed settlement clear another draft.
- Sharing resize state with answers can change workbench focus or panel sizing.
- `page.goto` during the navigation regression would test the excluded reload
  boundary. Seed both tasks before entering answers, then use shipped UI navigation.

## Parallelism

`sequential`

## Inputs

- [Draft design](../../specs/tasks/system-design/clarification-response-reliability.md#unsubmitted-drafts-across-navigation).
- [Ownership decision](../../decisions/2026-11-03-clarification-answer-draft-ownership.md).
- `apps/web/lib/rich-output/view-state.ts` for weak application-store ownership.
- `apps/web/lib/state/settings-agent-removals.ts` for store-lifetime observation.
- `apps/web/e2e/helpers/clarification.ts` for `clarification-multi` seeding.
- `apps/web/e2e/pages/session-page.ts` for active-chat and sidebar locators.
- `apps/web/e2e/tests/task/mobile-archived-task-navigation.spec.ts` for phone
  client task navigation and document-request observation.

## Results

Implemented on 2026-11-03.

- Added `apps/web/lib/clarification/answer-draft-cache.ts` with store isolation,
  eight-entry recency eviction, generation fencing, and the store-lifetime auth
  authority observer. `createAppStore` installs the observer once.
- Reworked `clarification-draft-context.tsx` so answer drafts come from the
  application store while panel sizes stay local to the mounted task layout.
  Restoration filters removed questions and options, clamps the question index,
  does not persist empty mounts, and ignores the previous bundle's answer map.
- Added `answer-draft-cache.test.ts`, replaced the task-change erasure test with
  the two navigation regressions, and extended the overlay, desktop, and phone
  suites.
- Added the public navigation-retention note to `docs/public/tasks-and-workflows.md`.

Verification passed:

- `cd apps/web && pnpm exec vitest run` over the eight work-order files: 161 tests
  and 166 after the review remediation and regression additions.
- `cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck`.
- `cd apps/web && pnpm exec eslint --max-warnings 0` over every changed source,
  test, and spec file.
- `cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet`.
- `cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/chat/clarification.spec.ts -- --grep 'question draft navigation'`:
  2 passed. The full file also passed, 30 tests.
- `cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/chat/mobile-clarification.spec.ts -- --grep 'question draft navigation'`:
  1 passed. The full file also passed, 16 tests.
- `cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/chat/clarification-resize.spec.ts`:
  4 passed.
- `node --test scripts/validate-public-docs.test.mjs`, `node scripts/validate-public-docs.mjs`,
  `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.test.py`,
  `python3 scripts/lint-spec-files.py --all`.
- The delivery-package coverage preflight returned `covered` with no errors.
- `git diff --check` and the final status review passed.

The E2E runs reused the managed build produced by the first run; desktop and
mobile runs were sequential. No commit, push, or PR was created.

## Review remediation

Review round on 2026-11-03 addressed two P2 findings in the draft retention path:

- The overlay now keeps the retained raw custom draft byte-for-byte when it
  restores a bundle. `answer.custom_text` only fills a question that has no raw
  draft of its own, including an explicitly empty one; late-answer snapshots
  keep their existing overwrite behavior. A new overlay regression enters
  `"  Use Go\n"`, unmounts and remounts the form, asserts the unchanged raw
  value, then submits and asserts the trimmed payload.
- An authority-generation change no longer disables retention for the mounted
  form. The draft hook resets the same-scope form to the current generation and
  quarantines the previous answer map; `useClarificationAuthorityReset` clears
  the group answers so fresh interaction can persist. A real-`StateProvider`
  overlay regression edits a bundle, changes the signed-in role without
  unmounting, verifies the old draft is cleared, enters fresh answers, and
  verifies only the fresh draft survives a provider remount.

Code review follow-up on the PR also:

- Retained answers for questions that have not arrived yet while a bundle is
  still loading, instead of filtering them out and writing the partial list back.
- Limited shared navigation retention to active pending forms; late-answer forms
  keep their bundle-local scope and host-owned admission state.
- Skip cache recency refreshes for unchanged writes.
- Reset the shared overlay test cache between tests and use the exported draft
  identity helper rather than duplicating the key format.
- Corrected the design's restoration-validation wording.
- Removed explicit UI assertion budgets from the navigation E2E tests and
  counted clarification responses during phone navigation.

Post-remediation gate set passed:

- `cd apps/web && pnpm exec vitest run` over the eight work-order files: 166 tests.
- `cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck`.
- `cd apps/web && pnpm exec eslint --max-warnings 0` over every changed source,
  test, and spec file, including `lib/state/store.ts`.
- `cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet`.
- `cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/chat/clarification.spec.ts`:
  30 passed.
- `cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/chat/mobile-clarification.spec.ts`:
  16 passed.
- `cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/chat/clarification-resize.spec.ts`:
  4 passed.
- `node --test scripts/validate-public-docs.test.mjs`, `node scripts/validate-public-docs.mjs`,
  `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.test.py`,
  `python3 scripts/lint-spec-files.py --all`.
- The delivery-package coverage preflight returned `covered` with no errors.
- `git diff --check`.
