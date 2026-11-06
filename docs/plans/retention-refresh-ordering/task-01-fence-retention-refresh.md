---
id: "01-fence-retention-refresh"
title: "Fence retention refresh publication"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-RUN-HISTORY-RETENTION-004
acceptance_criteria:
  - AC-OFFICE-RUN-HISTORY-RETENTION-004.10
system_design:
  - ../../specs/office/system-design/run-history-retention-operations.md
---

# Task 01: Fence retention refresh publication

## Summary

Prevent stale Office retention GETs from rewinding accepted Save baselines.
Preserve full status and the generic settings coordinator's revision semantics.

## In scope

Hook ownership, real-boundary regression, targeted hook controls and managed
browser proof. Owning requirement/design updates precede permanent code/tests.

## Out of scope

Backend, schemas, permissions, sweeps/deletion, generic resource migration and
publication. No subagents or extra tasks/sessions.

## Acceptance

- Old read success/failure/finalization cannot supersede a newer read or PUT.
- Accepted normalized writes survive unmount; fresh reads update complete status;
  independent stores stay isolated and failed writes retain retry/draft behavior.
- Route leave/reenter on desktop and phone retains the acknowledged baseline,
  clean state and correct Reset, with real isolated backend readback.

## ASCII UI preview

UI-01: Storage > Office retention, Advanced expanded, desktop and phone.
Composition and copy remain shipped; only reconciliation changes.

```text
Office retention
  Policy and sweep status
  Advanced settings [expanded]
    Batch limit [7000]
  [Reset] [Save changes]   (only for a dirty draft)
```

Before: late GET 6000 makes saved 7000 falsely dirty; Reset displays 6000.
After: saved 7000 stays clean; edit 8000 then Reset displays 7000.
Phone reuses the shipped one-column card, touch controls and safe-area-aware
floating Save surface; desktop retains its field grid. No layout, navigation,
copy, or control-size changes. AC-OFFICE-RUN-HISTORY-RETENTION-004.10 is checked
through the real card/coordinator and guarded desktop/phone navigation tests.

See [combined preview](plan.md#ascii-ui-preview).

## Verification

Run from repository root with installed Node 24 on PATH:

```bash
(cd apps/web && pnpm exec vitest run components/settings/system/retention-refresh-ordering.test.tsx components/settings/system/retention-settings-card.test.tsx components/settings/settings-save-provider.test.tsx hooks/domains/system/use-retention-settings.test.tsx lib/api/domains/system-api.test.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm exec eslint hooks/domains/system/use-retention-settings.ts hooks/domains/system/use-retention-settings.test.tsx components/settings/system/retention-refresh-ordering.test.tsx components/settings/system/retention-settings-card.tsx e2e/helpers/retention-refresh-ordering.ts e2e/tests/system/retention-settings.spec.ts e2e/tests/system/mobile-retention-settings.spec.ts)
(cd apps/web && GOMAXPROCS=2 GOFLAGS=-p=2 pnpm e2e:run --host --shards 1 --project chromium tests/system/retention-settings.spec.ts --grep 'acknowledged policy' --retries=0)
(cd apps/web && GOMAXPROCS=2 GOFLAGS=-p=2 pnpm e2e:run --host --shards 1 --no-build --project mobile-chrome tests/system/mobile-retention-settings.spec.ts --grep 'acknowledged policy' --retries=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff 2530afe4e53c481c937bf39eb2b10c5e6382988e HEAD --check
```

Run exact-head PR documentation coverage with the repository validator before
handoff. Retain every process handle through terminal completion. Run browsers
sequentially with one managed worker, no retries, and disposable backend data.

## Files likely touched

- `apps/web/hooks/domains/system/use-retention-settings.ts` and `.test.tsx`
- `apps/web/components/settings/system/retention-refresh-ordering.test.tsx`
- `apps/web/e2e/helpers/retention-refresh-ordering.ts`
- Existing desktop/mobile retention-settings specs
- Owning Office requirement/design and this plan package

## Dependencies

None.

## Risks

Preserve sweep/skip/count metadata and submitted revision behavior; do not
mistake a tab switch for route unmount.

## Parallelism

`sequential`

## Inputs

AC-OFFICE-RUN-HISTORY-RETENTION-004.5, .9, .10 and owning HTTP/frontend design;
REQ-UI-SETTINGS-MANUAL-SAVE-001 compatibility; parent's causal diagnostic.

## Results

Causal component/hook RED and normalized-response RED are recorded in the plan.
Final guarded Vitest: five files, 74 tests passed. Node 24 / 8 GiB typecheck and
targeted ESLint passed. Managed desktop and phone flows each passed one test,
sequentially with one worker and retries disabled. Full policy restored in both
isolated backends. No sweep, deletion or real-user policy mutation occurred.
Catalog, full spec lint, working-tree docs coverage and whitespace checks passed.
Normal commit hooks and exact committed-head docs/diff gates are delivery checks
recorded in the parent handoff. Public docs remain accurate; only internal docs
needed updates. No subagents, push, PR or broad audit.

## Delivery CI follow-up

The parent authorized bounded CI remediation after independent diff validation.
Two existing E2E assertions need causal backend readiness instead of early UI
sampling: the silent-prompt recovery phase and Monitor script completion. Repair
only those waits and retain the existing expected values. See the plan's delivery
remediation section. Run full `tests/chat/monitor.spec.ts`, full phone
`tests/session/mobile-stall-notice-recovery.spec.ts`, and the shared desktop
uncertain cancellation case, all with one worker and `--retries=0`. No product,
wire, persistence or authority behavior changes.

```bash
(cd apps/web && CI=true GOMAXPROCS=2 GOFLAGS=-p=2 pnpm e2e:run --host --shards 1 --no-build --project chromium tests/chat/monitor.spec.ts --retries=0)
(cd apps/web && CI=true GOMAXPROCS=2 GOFLAGS=-p=2 pnpm e2e:run --host --shards 1 --no-build --project mobile-chrome tests/session/mobile-stall-notice-recovery.spec.ts --retries=0)
(cd apps/web && CI=true GOMAXPROCS=2 GOFLAGS=-p=2 pnpm e2e:run --host --shards 1 --no-build --project chromium tests/session/stall-notice-recovery.spec.ts --grep 'watchdog preserves the turn and cancellation stays blocked when uncertain' --retries=0)
```

Post-change results: Monitor 3 passed, phone recovery 4 passed, desktop uncertain
cancellation 1 passed. The failed shard's original membership also passed locally:
146 passed and 1 skipped, one worker, retries disabled, 11.0 minutes. Catalog
fingerprints were regenerated for the edited tests without changing membership.
Fresh exact-head remote CI/reviews and final combined-merge verification remain
delivery gates; these local results do not waive the failed original CI run.

Final local remediation checks: the original five suites passed all 74 tests
again (13.02s); Node 24 / 8 GiB typecheck, targeted ESLint, Prettier, catalog,
full specification lint and whitespace checks passed. No production source
changed in this follow-up. Normal commit hooks and fresh remote evidence remain
delivery steps recorded in the task handoff.
