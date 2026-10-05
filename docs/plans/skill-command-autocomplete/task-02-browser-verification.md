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
  - AC-AGENTS-COMMAND-AUTOCOMPLETE-002.1
  - AC-AGENTS-COMMAND-AUTOCOMPLETE-002.2
  - AC-AGENTS-COMMAND-AUTOCOMPLETE-002.3
  - AC-AGENTS-COMMAND-AUTOCOMPLETE-002.4
  - AC-AGENTS-COMMAND-AUTOCOMPLETE-002.5
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
- A small how-to section in `docs/public/tasks-and-workflows.md` about selection, category/state chips, and argument hints.
- Final targeted checks and design lifecycle updates after both work orders pass.

## Out of scope

- Provider accounts and live retro execution in E2E.
- Broad QA, review, or unrelated E2E suites.

## Acceptance

1. Task chat and quick chat show clean skill names and confirmed mode state, then submit the raw command only after explicit send.
2. Phone tests prove the same flow with touch, a visible badge, contained geometry, and a measured minimum 44-pixel row target.
3. Public docs explain the shipped behavior and classification fallback; recorded verification matches actual test results.

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
- After review remediation, the complete managed Chromium composer suite passed 7 tests and the mobile Chromium composer suite passed 2 tests. Both new live-state regressions passed against the session's actual execution ID.
- Documented command selection, chips, and argument hints in `docs/public/tasks-and-workflows.md`.
- Public documentation tests passed (62 tests) and the validator checked 47 published pages. Specification catalog validation, all specification files, and the 36 specification-linter tests passed.
