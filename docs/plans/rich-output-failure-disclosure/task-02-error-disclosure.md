---
id: "02-error-disclosure"
title: "Disclose failed rich-output errors"
status: done
wave: 2
depends_on:
  - "01-status-discovery"
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RICH-OUTPUT-001
  - REQ-AGENTS-AGENT-RICH-OUTPUT-003
acceptance_criteria:
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.13
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.21
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.22
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.23
  - AC-AGENTS-AGENT-RICH-OUTPUT-003.13
system_design:
  - ../../specs/agents/system-design/agent-rich-output.md
  - ../../specs/agents/system-design/agent-rich-output-failures.md
---

# Task 02: Disclose failed rich-output errors

## Summary

Make the existing failed visual-output cross an accessible disclosure.
Desktop gets a short hover/focus tooltip and click/keyboard Popover;
phones and coarse pointers get a tap-operated Drawer.

## In scope

- Begin with a failing full-consumer component regression through
  `KandevToolMessage`, using the retained null-structured-content/text envelope.
- Add a rich-output error text helper and disclosure component. Select nonempty
  strings or string message/error fields only; use a translated missing-details
  fallback for unsupported data. Render plain text, never submitted arguments.
- Add the optional status-control slot to `KandevRow`, preserving permission
  precedence and every caller that does not supply it. Wire only failed rich output.
- Use the existing parsed result, Tooltip/Popover/Drawer primitives, local state,
  and the phone-or-coarse-pointer decision from the design.
- Localize host labels in en, pt-pt, zh-cn, zh-hk, zh-tw, ja and ko.
  Generate Traditional Chinese with the existing helper.
- Add desktop/mobile E2E, long-error/geometry coverage and reload behavior.
- Add one concise error-inspection instruction to the existing public MCP guide.

## Out of scope

Success-card redesign, grouping changes, retry inference, general tool-error
redesign, transport/database changes, and agent-triggered callbacks.

## Acceptance

- The red cross remains the only persistent error indicator. Hover/focus exposes
  the short explanation; activation opens the complete retained diagnostic.
  Missing details have a localized fallback and permission indicators keep precedence.
- Phone/coarse-pointer interaction uses a Drawer with one internal scroll owner,
  safe-area clearance, wrapping, dismissal/focus return and actual targets at least 44px.
- A failed call followed by a successful corrected call remains two outcomes
  live and after reload. Error disclosure neither invokes tools nor resets the
  successful report's file/chart state. Pending/running and other tools remain unchanged.

## ASCII UI preview

UI-01: Failed call, desktop resting state (AC 001.21).
The visible row stays compact:

```text
[chart] Kandev: visual output  [x]
[successful report card stays unchanged]
```

UI-02: Desktop cross hovered/focused, then activated (AC 001.21).
The tooltip shows a short error. Click or Enter/Space opens the popup:

```text
[chart] Kandev: visual output  [x]
                              +---------------------------+
                              | Visual output error [Close]|
                              | Retained validation error |
                              | (wrap; scroll if long)    |
                              +---------------------------+
```

UI-03: Phone cross tapped (AC 001.22).

```text
+--------------------------------+
| Conversation behind backdrop   |
|                                |
| +----------------------------+ |
| | Visual output error [Close]| |
| | Retained validation error  | |
| | One wrapping scroll region | |
| |                            | |
| +------ safe-area space -----+ |
+--------------------------------+
```

The title/close row stays fixed. Only the diagnostic body scrolls.
Closed rows have no inline error copy. Cross and close targets measure at least
44px on phones/coarse pointers; desktop retains compact sizing.
Popover versus inset Drawer, error ownership and content order are required.
Spacing and illustrative English labels are not pixel/copy specifications.
Use localized host labels and existing primitives.

Full context: [combined plan previews](plan.md#ascii-ui-preview).
Capture desktop/phone screenshots and compare structure with UI-02/03.

## Verification

Run from the repository root after Task 01's dependency bootstrap.

```bash
(cd apps/web && pnpm exec vitest run components/task/chat/messages/kandev-tool-message-rich-output-errors.test.tsx components/task/chat/messages/kandev-tool-message-rich-output.test.tsx components/task/chat/messages/kandev-tool-message.test.tsx components/task/chat/messages/kandev/rich-output/rich-output-error.test.ts)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/chat/messages/kandev/shared.tsx components/task/chat/messages/kandev/rich-output/rich-output-renderer.tsx components/task/chat/messages/kandev/rich-output/rich-output-error.ts components/task/chat/messages/kandev/rich-output/rich-output-error.test.ts components/task/chat/messages/kandev/rich-output/rich-output-error-disclosure.tsx components/task/chat/messages/kandev-tool-message-rich-output-errors.test.tsx e2e/tests/chat/rich-output-errors.spec.ts e2e/tests/chat/mobile-rich-output-errors.spec.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/rich-output-errors.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-rich-output-errors.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The component and helper test paths above are planned new files. Run the new
full-consumer regression red before implementation. Managed E2E commands build
fresh assets and run sequentially; do not overlap suites or override worker limits.
Record every command/result and reconcile renamed or additional test files.

## Files likely touched

- `apps/web/components/task/chat/messages/kandev/shared.tsx`
- `apps/web/components/task/chat/messages/kandev/rich-output/rich-output-renderer.tsx`
- New `rich-output-error.ts`, `rich-output-error.test.ts`, and
  `rich-output-error-disclosure.tsx` beside that renderer.
- New `apps/web/components/task/chat/messages/kandev-tool-message-rich-output-errors.test.tsx`
- Existing `kandev-tool-message-rich-output.test.tsx` and
  `kandev-tool-message.test.tsx` only where assertions need extension.
- New `apps/web/e2e/tests/chat/rich-output-errors.spec.ts` and
  `mobile-rich-output-errors.spec.ts`; reuse existing rich-output fixture helpers.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/task.json`
- `docs/public/automation-and-mcp.md`, Native rich output section.

## Dependencies

Task 01, for the complete corrected-authoring/disclosure outcome.
This ordering does not require shared production state or parallel agents.

## Risks

Do not nest interactive buttons or allow a popup click to toggle row expansion.
Test missing retained payloads instead of dumping generic objects.
Keep Tooltip closed while Popover is open. Preserve accessible focus on dismissal.
Assert narrow fine-pointer phone behavior as well as the default touch device.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/agents/requirements/agent-rich-output.md), criteria 001.21-.23.
- [Failure design](../../specs/agents/system-design/agent-rich-output-failures.md).
- Existing `RemoteCloudTooltip` touch disclosure and rich-output E2E fixtures.
- [Shared MCP result selection](../../specs/ui/system-design/kandev-mcp-tool-results.md).

## Results

Implemented a rich-output-only error disclosure. Failed calls retain the red
cross and expose the retained plain-text diagnostic through a desktop tooltip
and popover or a phone/coarse-pointer drawer. Unsupported results use localized
fallback copy, and arbitrary results or submitted arguments are never printed.
The optional row status control preserves pending and rejected permission icons.

The host labels are translated in all seven supported locales. Traditional
Chinese was generated with `pnpm run i18n:zh-hant`. The Native rich output guide
now tells users how to inspect a failed call.

Review remediation: `useTouchDrawer` now runs unconditionally, so the disclosure
keeps its open state when the same mounted component resizes from 768px to
767px and back to 768px. The Drawer title keeps Radix's generated label ID, and
DrawerContent describes the diagnostic body. A mobile browser assertion
locates the dialog by its accessible name and verifies that its description
points to the diagnostic. Both added regressions reproduced their findings
before the fixes.

Verification passed:

- The full-consumer and helper Vitest command: 56 tests passed.
- Follow-up responsive disclosure and rich-output unit tests: 16 tests passed,
  including the mounted breakpoint transition.
- `pnpm run i18n:zh-hant`, `pnpm run i18n:pseudo`, `pnpm run i18n:check`,
  and `pnpm run i18n:ratchet`.
- `NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck`, targeted ESLint,
  and Prettier checks.
- `pnpm e2e:run --project mobile-chrome tests/chat/mobile-rich-output-errors.spec.ts`:
  two tests passed after a fresh backend and Vite build, including the dialog
  accessible-name and description association assertions.
- `pnpm e2e:run --project chromium tests/chat/rich-output-errors.spec.ts`:
  passed. The final desktop test also passed after an added focus-only tooltip
  assertion.
- `pnpm e2e:run --project mobile-chrome tests/chat/mobile-rich-output-errors.spec.ts`:
  two tests passed. Both projects built fresh Vite assets.
- Public-doc tests (63 passed), public-doc validation (54 pages), specification
  validation and lint, and `git diff --check`.

The default-heap typecheck first hit Node's 4 GB heap limit. The 8 GB retry
passed. The first mock-agent failure fixture did not create a persisted error
status message, so the E2E helper now seeds ordered failed and successful
transcript messages after the agent turn ends.

PR review remediation on 2026-10-26 adds `cursor-pointer` to the trigger and
uses `PopoverTrigger` so activating an open desktop disclosure closes it. The
new component regression first failed because the dialog remained open after a
second activation. Mobile dialog-name and diagnostic-description checks remain
in the browser test. Desktop and phone PR screenshots were recaptured from the
fixed build.

Follow-up verification passed:

- RED: `(cd apps/web && pnpm exec vitest run components/task/chat/messages/kandev/rich-output/rich-output-error-disclosure.test.tsx)` failed because the popup did not close on a second click.
- GREEN: `(cd apps/web && pnpm exec vitest run components/task/chat/messages/kandev/rich-output/rich-output-error-disclosure.test.tsx components/task/chat/messages/kandev-tool-message-rich-output-errors.test.tsx)` passed (7 tests).
- `(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)` passed.
- `(cd apps/web && pnpm exec eslint components/task/chat/messages/kandev/rich-output/rich-output-error-disclosure.tsx components/task/chat/messages/kandev/rich-output/rich-output-error-disclosure.test.tsx e2e/tests/chat/rich-output-errors.spec.ts e2e/tests/chat/mobile-rich-output-errors.spec.ts)` passed.
- `(cd apps/web && pnpm exec prettier --check components/task/chat/messages/kandev/rich-output/rich-output-error-disclosure.tsx components/task/chat/messages/kandev/rich-output/rich-output-error-disclosure.test.tsx e2e/tests/chat/rich-output-errors.spec.ts e2e/tests/chat/mobile-rich-output-errors.spec.ts)` passed.
- `git diff --check` passed.
- `CAPTURE_PR_ASSETS=1 pnpm e2e:run --project chromium tests/chat/rich-output-errors.spec.ts` passed (1 test) and rebuilt the web app.
- `CAPTURE_PR_ASSETS=1 pnpm e2e:run --project mobile-chrome tests/chat/mobile-rich-output-errors.spec.ts` passed (2 tests) and rebuilt the web app.

PR CI follow-up: shard 7 exposed a resize test that dragged the pointer 2,000px
outside its 1,280px viewport. An initial reduction to 800px still reached the
review dialog's far edge in the focused post-merge run, so the final test drags
400px to exceed the 600px maximum while keeping the pointer inside the dialog.
Shards 8 and 9 measured
44px mobile targets at 43.999938px. The remaining direct height assertions now
use the shared `expectTouchControl` helper's 0.01px layout-rounding tolerance.
The mobile Quick Chat activity test was marked flaky by CI: its first attempt
missed the quick action while the navigation drawer was opening, and its retry
passed. The test now waits for finite drawer animations to finish before each
quick action and verifies that the drawer closes. Its focused mobile run passed
three repetitions with retries disabled. The resize test passed with retries
disabled, and the full mobile diagnostic-bundle spec passed all three tests
with retries disabled.

Shard 16 also exposed an agent-profile deletion test that assumed the first
listed agent had a profile. The current `main` version resolves the fixture's
seeded profile ID through `seededAgentProfile`. It supersedes the branch's
first-available-profile workaround. Its focused test passed three repetitions
before this rebase.

Before this rebase, after merging the then-current `main` base, the focused resize E2E showed that an
800px drag could reach the far edge of the review dialog and lose the resize
event. The final resize change passed the clamp case three repetitions with
retries disabled, and all four tests in its file passed with retries disabled.
Post-merge checks also passed the rich-output component tests (7), backend MCP
server package test, web typecheck, full web lint, the rich-output and resize
desktop E2E files, and the five related mobile E2E files (11 tests). The E2E
runner rebuilt the web app successfully.

PR fixup on 2026-10-27: merged the current `main` base at `d549fa694b7` into
the PR branch. The earlier profile-deletion conflict now uses the base's
`getSeededAgentProfile` helper, which targets the fixture profile. The merge
completed without conflicts, and the existing debug-skill edits in the
worktree were left unstaged.

The prior PR-head E2E run reported the mobile shared-sidebar test failing on
all four attempts. Its backend log showed that unarchive raced an archive
cleanup still in the `running` state. The destination is only rendered as a
sidebar row, so its fixture no longer creates an unnecessary completed
session; the anchor retains the seeded conversation needed by the test.
Focused verification on the merged base passed: the mobile shared-sidebar
test passed three repetitions with retries disabled, the desktop counterpart
passed once with retries disabled, and targeted ESLint, Prettier, and
`git diff --check` passed. The PR has not yet received CI results for the
merged head.
