---
id: TASK-SENTRY-WATCH-DIALOG-01
title: Preserve current watch
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-INTEGRATIONS-SENTRY-WATCH-DIALOG-001
acceptance_criteria:
  - AC-INTEGRATIONS-SENTRY-WATCH-DIALOG-001.1
  - AC-INTEGRATIONS-SENTRY-WATCH-DIALOG-001.2
  - AC-INTEGRATIONS-SENTRY-WATCH-DIALOG-001.3
  - AC-INTEGRATIONS-SENTRY-WATCH-DIALOG-001.4
  - AC-INTEGRATIONS-SENTRY-WATCH-DIALOG-001.5
system_design:
  - ../../specs/integrations/system-design/sentry-watch-dialog-lifetime.md
---

# Task 01: Preserve Current Watch

## Outcome and references

Implement REQ-INTEGRATIONS-SENTRY-WATCH-DIALOG-001, AC-INTEGRATIONS-SENTRY-WATCH-DIALOG-001.1 through .5.
[Design](../../specs/integrations/system-design/sentry-watch-dialog-lifetime.md).
[Plan](plan.md). Dependencies: frozen install and preserved pre-TDD package.

## Owned scope

`apps/web/components/sentry/sentry-issue-watch-dialog.tsx` and sibling lifetime
integration test/helpers; `apps/web/e2e/tests/integrations/` focused sentry-watch
desktop/mobile specs and helpers. Only the Sentry watch paragraph of
`docs/public/integrations.md` if needed. No other production modules.

## Acceptance

1. Causal immutable-source RED and ordinary controls precede minimal GREEN;
   real parent list publication, original payload and truthful feedback survive.
2. Later same/different/create openings, their busy state, unmount/scope and raw
   passive hydration survive old success/error. Current success/error/retry and
   subsequent Save/Cancel remain usable.
3. Desktop and native-tap phone production-browser proof and owned fixture failure
   cleanup control pass; checks and normal hooks pass. External full-diff packet
   is ready for independent parent rerun before any delivery.

## ASCII UI preview

UI-01 ([combined preview](plan.md#ascii-ui-preview)), shared desktop/phone:

```text
[Edit watcher | raw newer query | ...] [Cancel] [Update]
```

An old ACK keeps this opening visible, focused and unchanged; a newer pending
save keeps Saving busy until its own response. Existing dialog scroller/footer.

## Verification commands

Run under Node 24.18.0, with frozen install from `apps/` first:

```bash
cd apps/web
pnpm exec vitest run components/sentry/sentry-watch-dialog-lifetime.test.tsx components/sentry/sentry-issue-watch-dialog.test.tsx components/sentry/sentry-issue-watch-form.test.ts components/sentry/sentry-issue-watchers-section.test.tsx
pnpm exec eslint components/sentry/sentry-issue-watch-dialog.tsx components/sentry/sentry-watch-dialog-lifetime.test.tsx components/sentry/sentry-watch-dialog-lifetime.test-helpers.tsx e2e/tests/integrations/*sentry-watch-dialog*
NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck
pnpm run i18n:check
pnpm run i18n:ratchet
GOMAXPROCS=2 GOFLAGS=-p=2 pnpm e2e:run --project chromium e2e/tests/integrations/sentry-watch-dialog-ownership.spec.ts -- --retries=0
GOMAXPROCS=2 GOFLAGS=-p=2 pnpm e2e:run --no-build --project mobile-chrome e2e/tests/integrations/mobile-sentry-watch-dialog-ownership.spec.ts -- --retries=0
cd ../..
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
python3 scripts/lint-spec-files.test.py
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

Run repository documentation coverage helper with the exact base. Commit with
active normal pre-commit/commit-msg hooks and retain receipt. No push/PR.

## Results

Completed. The [plan results](plan.md#verification-results) record causal RED,
43 focused tests, desktop 3/phone 2 production-browser passes, cleanup control,
static/docs checks and bounded duplicate assessment. All receipts, full source/docs
and the normal-hook checkpoint are retained outside the repository. Production
ownership is confined to the dialog. Browser assets were freshly built once and
reused only while its source hash stayed identical. Earlier setup/fixture failures
remain distinct from causal RED. Stop before delivery for independent parent review.

Parent proof base: `688fb4772b43d07d5c4eb6b78b219ca7289b3ee6`.
