---
id: integrated-control-office-aggregate-task-02
title: Integrated control aggregate dashboard
status: done
wave: 2
depends_on:
  - integrated-control-office-aggregate-task-01
plan: plan.md
requirements:
  - REQ-UI-INTEGRATED-CONTROL-002
  - REQ-UI-INTEGRATED-CONTROL-003
acceptance_criteria:
  - AC-UI-INTEGRATED-CONTROL-002.1
  - AC-UI-INTEGRATED-CONTROL-002.2
  - AC-UI-INTEGRATED-CONTROL-002.3
  - AC-UI-INTEGRATED-CONTROL-002.4
  - AC-UI-INTEGRATED-CONTROL-002.5
  - AC-UI-INTEGRATED-CONTROL-002.6
  - AC-UI-INTEGRATED-CONTROL-003.1
  - AC-UI-INTEGRATED-CONTROL-003.2
  - AC-UI-INTEGRATED-CONTROL-003.3
  - AC-UI-INTEGRATED-CONTROL-003.4
  - AC-UI-INTEGRATED-CONTROL-003.5
system_design:
  - ../../specs/ui/system-design/integrated-control-navigation.md
legacy_specs: []
---

# Task 02: Integrated control aggregate dashboard

## Scope

- `useControlAggregate` read in the integrated control provider, gated by the route, authentication, and `features.office`; the existing Refresh button refreshes it too.
- Pure derivations in `control-aggregate-signals.ts`.
- Overview: stall band, attention list, and workspace comparison (`control-aggregate-overview.tsx`).
- History: folded recent Office activity (`control-grouped-history.tsx`).
- Copy in `sidebar` for en and ko written by hand; ja, pt-pt, and zh-cn translated; zh-hk and zh-tw from `pnpm run i18n:zh-hant`; pseudo from `pnpm run i18n:pseudo`.

The host flag stays `features.integratedControl` (default off). No new flag.

## UI preview

See the Office aggregate dashboard section of the
[integrated control system design](../../specs/ui/system-design/integrated-control-navigation.md#office-aggregate-dashboard).

## Verification

The Native follow-up retains the Office tables and ports only the existing
dashboard GET opt-in, recorded metrics, DTO/parser/client, revision-based reads,
five-stage grouping and two Native comparison tables with raw-step disclosure.
Capacity, provider, task-list/history/ledger and conversation actions remain out.
Only consumed copy is added to all seven real catalogs, using the existing
Traditional Chinese converter. The Native query has permanent repository and
handler coverage. The browser parser, row retention, stage derivation, wait
reason precedence, and actual workspace destinations also have permanent tests.
Control reads have a finite per-read deadline and abort expired requests.
The 15-second deadline starts retry backoff after a stalled read. A late result
from an expired read cannot replace a newer aggregate snapshot.

- `cd apps/web && pnpm test -- components/integrated-control hooks/domains/coordinator/use-control-aggregate.test.tsx`
- `cd apps/web && pnpm exec vitest run hooks/domains/coordinator/control-refresh-scheduler.test.ts hooks/domains/coordinator/use-control-aggregate.test.tsx hooks/domains/coordinator/use-integrated-control-data.test.tsx`
- `cd apps/web && pnpm run i18n:check`
- `cd apps/web && pnpm e2e:run --project chromium tests/coordinator/control-aggregate-dashboard.spec.ts`
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/coordinator/mobile-control-aggregate-dashboard.spec.ts`

The contributor branch previously used one-time Native checks that were not
retained. The maintainer follow-up replaces those checks with collected
regression tests in the files listed by Task 03.

## Number formatting follow-up

Office aggregate facts, comparison rows and totals use the existing shared
formatNumber helper for the active locale, including both numbers in the
working-agent ratio. The ratio keeps its mathematical slash and existing
localized label; no new prose, catalog key or alternate counting is introduced.
Raw values still drive sums, ordering, zero styling and workspace navigation.
The pseudo locale reuses the helper's existing English numeric fallback.

Disposable UI checks reproduced ungrouped numbers on the original source.
The candidate passed both disposable UI checks, covering English, Portuguese,
Korean, live locale switches and the pseudo fallback, plus the four existing
aggregate dashboard cases. Zero styling, raw sums and navigation remain intact.
Normal hooks must pass before publication. Existing Office/Native reads,
feature flags, history and API responses are unchanged.

Upstream generated-catalog corrections in zh-hk, zh-tw and pseudo are retained
alongside this PR's added keys. A key-level three-way comparison found no
competing value changes. Current upstream is merged normally; the generated
catalog conflicts retain both branches' values and all added keys. No ownership,
counting or data-retention behavior is removed to obtain a clean merge.

The full existing locale check also exposed a missing plural contract for the
Native raw-stage disclosure label. Its count interpolation now has \_one/\_other
forms in all catalogs, preserving the current wording and raw-stage data. The
checker and count semantics are unchanged.
