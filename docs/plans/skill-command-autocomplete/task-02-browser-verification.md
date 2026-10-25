---
id: "02-browser-verification"
title: "Prove desktop and phone skill autocomplete"
status: done
wave: 2
depends_on:
  - "01-skill-presentation"
plan: "plan.md"
requirements:
  - REQ-AGENTS-COMMAND-AUTOCOMPLETE-001
  - REQ-AGENTS-COMMAND-AUTOCOMPLETE-002
acceptance_criteria:
  - AC-AGENTS-COMMAND-AUTOCOMPLETE-001.1
  - AC-AGENTS-COMMAND-AUTOCOMPLETE-001.2
  - AC-AGENTS-COMMAND-AUTOCOMPLETE-001.3
  - AC-AGENTS-COMMAND-AUTOCOMPLETE-001.4
  - AC-AGENTS-COMMAND-AUTOCOMPLETE-001.5
  - AC-AGENTS-COMMAND-AUTOCOMPLETE-001.6
  - AC-AGENTS-COMMAND-AUTOCOMPLETE-001.7
  - AC-AGENTS-COMMAND-AUTOCOMPLETE-001.8
  - AC-AGENTS-COMMAND-AUTOCOMPLETE-002.1
  - AC-AGENTS-COMMAND-AUTOCOMPLETE-002.2
  - AC-AGENTS-COMMAND-AUTOCOMPLETE-002.3
  - AC-AGENTS-COMMAND-AUTOCOMPLETE-002.4
  - AC-AGENTS-COMMAND-AUTOCOMPLETE-002.5
  - AC-AGENTS-COMMAND-AUTOCOMPLETE-002.6
system_design:
  - ../../specs/agents/system-design/command-autocomplete.md
---

# Task 02: Prove desktop and phone skill autocomplete

## Summary

Extend existing desktop and phone composer E2E suites with representative skill entries.
Prove clean display and exact submitted command text, then document the user-facing behavior.

## In scope

- Desktop task chat and quick-chat selection, raw explicit send, and message recall.
- Mixed command/skill entries with equal display names and absent classification.
- Phone touch selection, visible badges, long text, viewport bounds, and measured row targets.
- Confirmed plan-mode state updates, unknown-state fallback, unchanged configuration on selection, and goal hints without category badges.
- Forged slash-command clipboard markup remains plain visible text; partial provider updates preserve confirmed mode state in an open menu.
- A small how-to section in `docs/public/tasks-and-workflows.md` about selection, category/state chips, and argument hints.
- Final targeted checks and design lifecycle updates after both work orders pass.

## Out of scope

- Provider accounts and live retro execution in E2E.
- Broad QA, review, or unrelated E2E suites.

## Acceptance

1. Task chat and quick chat show clean skill names and confirmed mode state, then submit the raw command only after explicit send.
2. Phone tests prove the same flow with touch, a visible badge, contained geometry, and a measured minimum 44-pixel row target.
3. Public docs explain the shipped behavior and classification fallback; recorded verification matches actual test results.
4. Pasted command-shaped rich HTML cannot create a chip or change the visible text sent by the composer.

## ASCII UI preview

UI-01 and UI-02 excerpts from the [full preview](plan.md#ascii-ui-preview):

```text
Commands
  [icon] /retro [Skill]         Description...
  [icon] /retro                 Custom command...
  [icon] /plan  [Mode] [Active] Turn plan mode off
  [icon] /goal                 Set a goal to keep pursuing
         Arguments: [<objective>|clear|pause|resume]

Composer: [ /retro ] context |
                       [Send]
```

The first row retains raw `$retro`; the second retains raw `retro`.
Desktop and phone share composition, with description truncation and one listbox scroll owner.
Verify structure with screenshots and geometry checks. Applies to all ACs in REQ-001 and REQ-002.
Mode and Active chips require validated metadata and confirmed configuration respectively.

## Verification

Run from the repository root. Run browser suites sequentially through the managed runner.

```bash
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/chat/slash-command-composer.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/chat/mobile-slash-command-composer.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

The managed runner rebuilds required backend and web artifacts. Retain screenshots of desktop and canonical phone skill rows.
Capture raw submitted text with existing WS helpers. Use causal waits rather than fixed sleeps.
Run Task 01 checks after any production correction made during this work order.
The desktop suite also sends a partial provider update while the menu is open and verifies that an omitted confirmed mode remains Active. It pastes forged command-shaped HTML and verifies the plain-text send payload.

## Files likely touched

- `apps/web/e2e/tests/chat/slash-command-composer.spec.ts`
- `apps/web/e2e/tests/chat/mobile-slash-command-composer.spec.ts`
- `apps/web/e2e/helpers/session-store.ts` and `ws-capture.ts` only if their typed helpers need extension
- `docs/public/tasks-and-workflows.md`
- This plan package and paired specification statuses after all checks pass

## Dependencies

Task 01. Use `/e2e`, `/mobile-parity`, and `/docs-maintainer` during implementation.

## Risks

- A DOM text assertion alone does not prove submitted provider text. Capture the outbound payload.
- Same-name rows need scoped option selectors rather than ambiguous global text selectors.
- Type-only fixtures can bypass adapter classification; Task 01 separately proves the real provider conversion path.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/command-autocomplete.md)
- [System design](../../specs/agents/system-design/command-autocomplete.md)
- Existing desktop and mobile composer E2E fixtures and startup-aware page helpers.

## Results

- Added desktop and phone coverage for clean skill display, raw command submission after explicit send, duplicate display names, confirmed plan state, goal argument hints, and draft-only selection.
- The managed Chromium suite passed 5 tests and the mobile Chromium suite passed 2 tests against the final build. Screenshots are retained at `/tmp/kandev-skill-command-autocomplete-evidence/desktop-skill-command-menu.png` and `/tmp/kandev-skill-command-autocomplete-evidence/mobile-skill-command-menu.png`.
- Added event-driven desktop regressions that submit `/plan`, apply post-startup provider updates without `config_options_settled` while the menu is open, and verify the default → plan → default state changes. A second flow verifies immediate invalidation at `STARTING` and restoration only after the new execution settles.
- After review remediation, the latest complete managed Chromium composer suite passed 8 tests and the mobile composer suite passed 2 tests. The open-menu test sends a partial provider update without `config_options_settled` and confirms plan mode remains Active; startup tests verify immediate invalidation and fresh-snapshot restoration; forged command-shaped clipboard HTML remains `/plan` plain text through explicit send.
- Documented command selection, chips, and argument hints in `docs/public/tasks-and-workflows.md`.
- Public documentation tests passed (62 tests) and the validator checked 47 published pages. Specification catalog validation, all specification files, and the 36 specification-linter tests passed.


### CI startup fixture follow-up (2026-10-07)

The startup mode-state case now observes the real session's initial available
commands before installing its controlled plan command. Chat idle does not
prove that the asynchronous provider command frame arrived; that frame can
otherwise replace the seeded menu. The hosted case failed on its first attempt
at the missing `/plan` option. All startup invalidation, new-execution identity
and fresh mode-snapshot assertions remain.

Three first attempts passed with
`E2E_PORT_OFFSET=0 pnpm --dir apps/web e2e:run --host --no-build --shards 1
--project chromium tests/chat/slash-command-composer.spec.ts -- --grep
'invalidates a previous execution' --retries=0 --repeat-each=3` from the root.
Fresh hosted verification remains pending.

### PR #3598 CI remediation (2026-10-07)

Hosted shard 2 exposed the live-provider test seeding commands before the
provider's initial command list arrived. The test now captures the matching
session's initial command notification before seeding its plan command, as the
adjacent startup test already does. Provider updates and all open-menu state
assertions remain. The repaired scenario passed three repetitions without
retries. No production or mobile behavior changed.

Validation from `apps/web`:

```sh
E2E_PORT_OFFSET=0 pnpm e2e:run --project chromium tests/task/create-task-branch-policy.spec.ts tests/chat/slash-command-composer.spec.ts -- --grep 'selects a policy, enables fresh branch mode|keeps an open plan menu current' --retries=0 --repeat-each=3
```

Result: six passed after rebuilding the merged-main artifacts. Focused ESLint
and `pnpm run typecheck` also passed.

The complete desktop spec command also passed without retries:

```sh
E2E_PORT_OFFSET=0 pnpm e2e:run --no-build --project chromium tests/task/create-task-branch-policy.spec.ts tests/chat/slash-command-composer.spec.ts -- --retries=0
```

Result: ten passed. Repository catalog validation, full specification lint, and
PR documentation coverage passed (71 work orders). Hosted CI for the next head
remains pending; these are local receipts.
