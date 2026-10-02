---
id: "03-browser-and-docs"
title: "Prove browser flows and update user documentation"
status: done
wave: 3
depends_on:
  - "02-shared-composer"
plan: "plan.md"
requirements:
  - REQ-TASKS-QUICK-CHAT-COMPOSER-001
acceptance_criteria:
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.1
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.2
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.3
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.4
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.5
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.6
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.7
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.8
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.9
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.10
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.11
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.12
system_design:
  - ../../specs/tasks/system-design/quick-chat-opening-composer.md
---

# Task 03: Prove browser flows and update user documentation

## Summary

Reconcile existing browser flows with first-message creation and document the
new user path. Complete the traceability evidence without broad unrelated testing.

## In scope

- Update shared Quick Chat helpers and direct callers that previously started an
  empty conversation. Preserve saved-prompt and subsequent-message assertions.
- Extend desktop/mobile composer coverage for failure, retry, attachment transfer,
  mode changes, profile invalidation, workspace changes, and plugin capabilities.
- Verify existing configuration Settings entry, conversation viewport, tab order,
  and terminal-backed launch behavior with targeted tests.
- Update the public developer guidance with the opening flow and recovery.
- Reconcile obsolete setup-copy/footer expectations in the repository-context
  requirement. Keep isolation and rollback guarantees unchanged.
- Record actual results. Promote this pair to active/current and the plan to
  implemented only after all work orders pass and the code matches the design.

## Out of scope

Full-suite audits, unrelated cleanup, releases, commits, pushes, and PR creation.

## Acceptance

1. New and affected existing desktop/mobile suites pass with one initial message
   and all intended recovery and geometry outcomes, including no duplicate dispatch.
2. Public instructions describe the implemented entry flow and configuration
   limitation; specifications contain no conflicting setup presentation contract.
3. Every AC has recorded test evidence, each work order has exact command results,
   and package references and lifecycle statuses are consistent.

## ASCII UI preview

UI-04 excerpt; [full previews](plan.md#ascii-ui-preview). Covers AC 6, 8-12.

```text
Desktop                              Phone
[trace.zip: failed] [Retry] [Remove]  +------------------------------+
Prompt remains editable              | Prompt and files remain      |
[Attach]          [Send: disabled]    | [trace.zip: failed]          |
                                     | [Retry] [Remove]             |
Creation error: draft remains here.   | [Attach] [Send: disabled]    |
Delivery error: retry in same chat.   +------------------------------+
```

Compare rendered desktop UI-01/02 and phone UI-03 as well. Preserve safe-area and
keyboard reachability in UI-04; error text must not obscure Send or file recovery.

## Verification

Commands run from the repository root. The managed runners rebuild before each
project and enforce resource limits. Run projects sequentially.

```bash
(cd apps/web && pnpm e2e:run --project chromium 'tests/chat/quick-chat.*[.]spec[.]ts' tests/settings/config-chat-popover.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome 'tests/chat/mobile-quick-chat.*[.]spec[.]ts' tests/settings/mobile-config-chat-popover.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Before these runs, inventory helper imports and direct `quick-chat-start` uses
with `rg`. Add any affected suite outside these path patterns to the Results
command list, and run it. Confirm Playwright test discovery and record counts.
Use the repository documentation-coverage validator with the changed files and
complete referenced documents; record its result alongside the spec gates.

## Files likely touched

- `apps/web/e2e/tests/chat/quick-chat-helpers.ts` and affected Quick Chat specs.
- `apps/web/e2e/tests/chat/quick-chat-opening-composer.spec.ts` and its mobile pair.
- `apps/web/e2e/tests/settings/config-chat-popover.spec.ts` and its mobile pair.
- `docs/public/developer-tools.md`.
- `docs/specs/tasks/requirements/quick-chat-repository-context.md`.
- This requirement/design pair and all work-order Results sections.

## Dependencies

Tasks 01 and 02. Read `/docs-maintainer` before public edits.

## Risks

Existing helper callers can mistake the new first turn for a later turn. Use
backend-confirmed settle conditions and assert message identity/content, not only
an enabled editor. Do not replace existing scenarios with weaker visibility checks.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/tasks/requirements/quick-chat-opening-composer.md).
- [Design](../../specs/tasks/system-design/quick-chat-opening-composer.md).
- Root and scoped `AGENTS.md`; `/tdd`, `/mobile-parity`, and `/e2e` as applicable.

## Results

Migrated direct Quick Chat callers and affected first-turn expectations while
preserving saved-prompt, entity-reference, slash-command, tab, queue, and settings
coverage. Added desktop and mobile Quick Chat opening-composer flows and fixture
plugin insert-and-submit checks. Updated the public developer guide and reconciled
the repository-context requirement with the opening-composer behavior.

- Desktop existing Quick Chat/composer regression set: 57 tests accounted for;
  53 passed in the broad run, with the four migrated setup and follow-up queue
  cases passing in focused reruns.
- Mobile Quick Chat and configuration set: 15 tests accounted for; 14 passed
  in the combined run and the migrated configuration-mode picker case passed
  in a focused rerun.
- Desktop configuration popover: five passed in the combined run and the
  migrated command-palette setup case passed in a focused rerun.
- New desktop and mobile opening-composer E2Es, creation failure/retry, and
  desktop/mobile plugin Quick Chat actions passed. Each opening prompt appeared
  once in persisted session messages.
- `node --test scripts/validate-public-docs.test.mjs`,
  `node scripts/validate-public-docs.mjs`, `python3 scripts/list-docs.py validate`,
  `python3 scripts/lint-spec-files.test.py`, and
  `python3 scripts/lint-spec-files.py --all` passed.
- Documentation-coverage validation and `git diff --check` passed.
