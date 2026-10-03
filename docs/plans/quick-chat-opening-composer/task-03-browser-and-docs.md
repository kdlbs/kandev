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
- Desktop configuration popover: all six passed in the final focused run,
  including the command-palette setup case.
- New desktop and mobile opening-composer E2Es, creation failure/retry, and
  desktop/mobile plugin Quick Chat actions passed. Each opening prompt appeared
  once in persisted session messages.
- `node --test scripts/validate-public-docs.test.mjs`,
  `node scripts/validate-public-docs.mjs`, `python3 scripts/list-docs.py validate`,
  `python3 scripts/lint-spec-files.test.py`, and
  `python3 scripts/lint-spec-files.py --all` passed.
- Documentation-coverage validation and `git diff --check` passed.

Final focused browser checks were run sequentially from `apps/web`:

- `pnpm e2e:run --host --no-build --project chromium e2e/tests/chat/quick-chat-opening-composer.spec.ts`: 2 passed, including staged attachment file-byte verification and desktop 1440x400 overflow.
- `pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/chat/mobile-quick-chat-opening-composer.spec.ts`: 1 passed, including 390x560 overflow and Send containment.
- `pnpm e2e:run --host --no-build --project chromium e2e/tests/chat/agent-profile-recent-use.spec.ts`: 1 passed after closing the setup tab through the supported tab action and reopening from the still-open add menu.
- `pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/chat/mobile-agent-goal.spec.ts --grep 'submits once while the message acknowledgement is delayed' --retries=0`: 1 passed; the settled opening-message baseline plus the delayed user send produces exactly one additional `message.add` request.
- `pnpm e2e:run --host --no-build --project chromium e2e/tests/chat/queue-admission-reliability.spec.ts --grep 'reconciles without duplicating a queued message' --retries=0`: 1 passed. Queue-add diagnostics now reset at fault injection; the targeted lost-response attempt sends once. The initial whole-test count included queue traffic before the fault was armed.
- `pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/chat/mobile-queue-admission-reliability.spec.ts --grep 'reconciles through a touch submit' --retries=0`: 1 passed.
- `pnpm e2e:run --host --no-build --project chromium e2e/tests/plugins/composer-actions.spec.ts --grep 'Quick Chat setup'`: 1 passed.
- `pnpm e2e:run --host --no-build --project chromium e2e/tests/session/session-resume-prompt-queue.spec.ts`: 3 passed.
- `pnpm e2e:run --host --no-build --project chromium e2e/tests/settings/config-chat-popover.spec.ts`: 6 passed. `pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/settings/mobile-config-chat-popover.spec.ts`: 1 passed.
- The first mobile workflow-preview run reproduced a touch-dismissal defect: a long popover covered the task-description editor, so its tap was intercepted by workflow-option content. The touch popover now exposes a localized 44px close control. `(cd apps/web && pnpm e2e:run --host --project mobile-chrome e2e/tests/task/mobile-task-create-workflow-step-previews.spec.ts --grep 'keeps long workflow previews contained and touch-usable on a phone' --retries=0)` rebuilt the app and passed 1 test. The other long-option scrolling case passed in the initial two-test run.

Merged-base PR fixup verification also passed:

- `pnpm e2e:run tests/chat/setup-recovery.spec.ts` passed 4 desktop tests with a
  fresh managed backend and Vite build. This covers setup retry, late-session
  tab retention, persisted post-allocation recovery, and preparation warnings.
- `pnpm e2e:run --no-build --project mobile-chrome tests/chat/mobile-setup-recovery.spec.ts`
  passed 2 phone tests using that fresh production bundle, including inline
  error recovery and the 44px Send target.
- Public-document validation passed all 62 tests and 47 published pages.
  Specification catalog validation and full specification lint passed.
- All 15 changed frontend test files passed (225 tests); focused Quick Chat tests
  passed all 151 tests. `git diff --check` passed.
