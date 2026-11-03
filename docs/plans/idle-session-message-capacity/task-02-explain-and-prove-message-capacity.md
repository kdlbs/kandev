---
id: "02-explain-and-prove-message-capacity"
title: "Explain and prove message capacity"
status: complete
wave: 2
depends_on:
  - "01-bound-and-queue-message-turns"
plan: "plan.md"
requirements:
  - REQ-AGENTS-SESSION-CEILING-002
  - REQ-AGENTS-SESSION-CEILING-003
acceptance_criteria:
  - AC-AGENTS-SESSION-CEILING-002.7
  - AC-AGENTS-SESSION-CEILING-003.1
  - AC-AGENTS-SESSION-CEILING-003.3
  - AC-AGENTS-SESSION-CEILING-003.4
  - AC-AGENTS-SESSION-CEILING-003.5
  - AC-AGENTS-SESSION-CEILING-003.8
system_design:
  - ../../specs/agents/system-design/session-concurrency-ceiling.md
---

# Task 02: Explain and prove message capacity

## Summary

Explain the changed enabled-capacity behavior in existing help and public docs.
Prove the outcome through the real desktop and phone composer.
Accepted input waits at full capacity and starts after a slot becomes available.

## In scope

- Update Session capacity help and Settings discovery descriptions.
- Reuse the current queue panel and page-level Settings save behavior.
- Update affected existing translations and generate the Traditional Chinese pair.
- Add focused desktop and mobile E2E scenarios for the backend outcome.
- Update public capacity and message-delivery documentation in the implementation PR.
- Record final package validation and work-order results.

## Out of scope

- A new capacity switch, queue component, banner, overlay, status field, or task state.
- Renaming unrelated task controls or redesigning chat and Settings.
- Additional live-provider or container executor coverage.
- Changing the backend ownership implemented in Task 01.

## Acceptance

1. Help describes message-turn waiting, install-wide scope, explicit execution
   overrides, and separate WIP limits in every supported locale.
2. Desktop and phone submit real input at capacity, show one pending item,
   retain it after reload, and show one delivered turn after release.
   Auto-run OFF prevents automatic delivery after release.
3. Public docs match the implemented compatibility change. Both work orders
   contain their actual targeted command results before the plan is complete.

## ASCII UI preview

### UI-01: Message accepted while capacity is full

See the [combined preview](plan.md#ui-01-message-accepted-while-capacity-is-full).

```text
Desktop chat                       Phone chat
Conversation                       Task / conversation
Queued (1)  Auto-run [ON]           Queued (1) Auto-run [ON]
Follow-up message                  Follow-up message
[existing queue actions]           [existing queue actions]
[composer]                [Send]   [composer]          [Send]
```

The existing queue owns its internal scrolling. The task layout owns the phone
viewport and safe-area inset. Keep current keyboard and touch actions.
This preview maps to criteria 003.3, .4, and .8.

### UI-02: Existing Session capacity help, both viewports

See the [Settings preview](plan.md#ui-02-existing-session-capacity-help-both-viewports).

```text
Session capacity
Limit automatic sessions             [ON]
Maximum sessions                     [ 1 ]
Automatic starts and new message turns wait at the limit.
Explicit Start, Resume, and Send Now can exceed it.
```

Keep the current stacked phone fields and the existing page-level Save/Reset bar.
Text wraps inside the current card. Labels are illustrative and remain localized.
This preview maps to criterion 002.7.

## Mobile contract and existing examples

Use `mobile-session-capacity-settings.spec.ts` for the existing Settings surface.
Use `mobile-message-queue-management.spec.ts` for the inline queue and touch actions.
The entry point is the normal phone task chat, not a new drawer.
The hierarchy stays conversation, pending queue, composer, and Send.
No layout or breakpoint changes are planned.

Measure the existing Send target and document overflow in the new mobile flow.
Use the configured `mobile-chrome` project without a device override.
Desktop and phone share server capacity and queue state.

## E2E scenarios

Create shared setup in `session/message-turn-capacity.helpers.ts`.
Use the worker-scoped mock backend and current session-capacity API helper.
Prepare an idle warm target and a separate long-running filler.
Enable capacity one after preparation. Use existing mock slow-duration support
and a causal state observation, not a sleep to guess that the filler started.

In both projects:

1. Open the target chat and submit a unique follow-up through the composer.
2. Assert one queue entry and one running filler, with no target provider turn.
3. Reload and assert the same accepted input remains visible.
4. End the filler through its normal control or cleanup API.
5. Observe capacity release, then assert one new response for the unique input.
6. Repeat the hold with Auto-run OFF and assert the message remains pending.

Use the response count before submission to distinguish the new turn from
earlier transcript text. Assert retained message/session identities through API
evidence alongside the visible outcome. Use causal waits from the existing helper.

For sibling coverage, submit to two eligible conversations of one task.
Observe independent pending entries and one-at-a-time progression.
Backend tests own detailed priority, failed persistence, and stale-identity races.

Capture and restore capacity settings in `afterEach` or `finally`.
Dispose of the tasks and restore any modified canonical profile or workflow.
Acquire `testPage` before applying non-default Settings state when fixture
initialization can rewrite that state.

## Copy and documentation

Update existing `system.json` capacity help keys and the Settings discovery
description. Keep the current switch label and control identity.
Translate English, Portuguese, Simplified Chinese, Japanese, and Korean.
Generate `zh-tw` and `zh-hk` with `i18n:zh-hant`, then regenerate pseudo as required.
Do not add hardcoded copy or Unicode em dashes.

Update the reference/explanation sections in:

- `docs/public/agents-and-profiles.md`: remove ordinary message delivery from the manual override examples.
- `docs/public/tasks-and-workflows.md`: explain the shared message/automatic capacity boundary and retained queue policy.
- `docs/public/websocket-api.md`: describe durable pending acceptance and existing response compatibility for `message.add`.

Search `README.md`, `docs/screenshots.md`, specifications, and decisions for stale
manual-message wording. Historical delivery results remain historical.
Public docs were updated in this implementation.

## Verification

Run this bootstrap once in a fresh worktree before any package command:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

Run the targeted checks from the repository root:

```bash
(cd apps/web && pnpm exec vitest run components/settings/system/session-capacity-settings.test.tsx lib/settings-discovery/catalog.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/session/message-turn-capacity.spec.ts tests/system/session-capacity-settings.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-message-turn-capacity.spec.ts tests/system/mobile-session-capacity-settings.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/idle-session-message-capacity
```

The managed E2E commands rebuild changed web/backend assets.
Run them sequentially with the repository's worker budget.
Record actual discovered test counts and results. Do not claim rendered
verification from a build or translation check alone.
Run the exact local PR-documentation coverage preflight in
[the plan](plan.md#local-documentation-coverage-preflight) before delivery.

## Files likely touched

- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-tw,zh-hk,ja,ko,pseudo}/system.json`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-tw,zh-hk,ja,ko,pseudo}/settings.json`
- `apps/web/components/settings/system/session-capacity-settings.test.tsx`
- `apps/web/lib/settings-discovery/catalog.test.ts`
- `apps/web/e2e/tests/session/message-turn-capacity.helpers.ts` (new)
- `apps/web/e2e/tests/session/message-turn-capacity.spec.ts` (new)
- `apps/web/e2e/tests/session/mobile-message-turn-capacity.spec.ts` (new)
- `apps/web/e2e/tests/system/session-capacity-settings.spec.ts`
- `apps/web/e2e/tests/system/mobile-session-capacity-settings.spec.ts`
- `docs/public/agents-and-profiles.md`
- `docs/public/tasks-and-workflows.md`
- `docs/public/websocket-api.md`
- This package's plan and results sections.

## Dependencies

Task `01-bound-and-queue-message-turns` must pass its backend checks first.

## Risks

- Existing assertions can preserve old manual-message help unless updated together.
- A stale build can hide a backend regression. Use the managed E2E runner.
- Worker-scoped settings can contaminate later tests without explicit restoration.
- A queue count alone does not prove that the provider received exactly one later turn.

## Parallelism

`sequential`

## Results

Implemented in the primary session.

- Updated Session capacity help and Settings discovery copy in `en`, `pt-pt`, `zh-cn`, `zh-tw`, `zh-hk`, `ja`, `ko`, and pseudo.
- Added desktop and phone message-turn E2E specs plus shared fixture setup. Both submit through the real composer, reload with one pending entry, release a separate filler, and assert one response; both also cover Auto-run OFF.
- Updated existing desktop and mobile Settings specs with rendered help assertions.
- Updated `agents-and-profiles.md`, `tasks-and-workflows.md`, and `websocket-api.md` for the message-turn compatibility change and durable pending acceptance.

Verification actually run:

```text
pnpm exec vitest run components/settings/system/session-capacity-settings.test.tsx lib/settings-discovery/catalog.test.ts
28 passed

NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck
passed

pnpm run i18n:check
passed (ja, ko, pt-pt, zh-cn, zh-hk, zh-tw complete; pseudo in sync)

pnpm run i18n:ratchet
passed

node --test scripts/validate-public-docs.test.mjs
63 passed

node scripts/validate-public-docs.mjs
Validated 54 published docs pages

python3 scripts/list-docs.py validate
Validated 425 decisions and 1885 specifications

python3 scripts/lint-spec-files.test.py
36 tests passed

python3 scripts/lint-spec-files.py --all
All specification files passed
```

Local PR-documentation coverage preflight passed with `status: covered` and both work orders linked. `git diff --check` was clean. Before the commit, `git status --short -- docs/plans/idle-session-message-capacity` reported the plan directory as untracked; the package was then committed.

Desktop and mobile Playwright scenarios passed:

```text
pnpm e2e:run --project chromium tests/session/message-turn-capacity.spec.ts
2 passed
pnpm e2e:run --project mobile-chrome tests/session/mobile-message-turn-capacity.spec.ts
2 passed
pnpm e2e:run --project chromium tests/system/session-capacity-settings.spec.ts
2 passed
pnpm e2e:run --project mobile-chrome tests/system/mobile-session-capacity-settings.spec.ts
2 passed
```

