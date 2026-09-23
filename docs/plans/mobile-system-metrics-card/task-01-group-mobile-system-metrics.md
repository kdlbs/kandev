---
id: "01-group-mobile-system-metrics"
title: "Group mobile system metrics"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-APP-STATUS-BAR-001
acceptance_criteria:
  - AC-UI-APP-STATUS-BAR-001.2
  - AC-UI-APP-STATUS-BAR-001.6
  - AC-UI-APP-STATUS-BAR-001.7
system_design:
  - ../../specs/ui/system-design/app-status-bar.md
---

# Task 01: Group Mobile System Metrics

## Scope

Implement the [plan](plan.md) using the [status-bar requirements](../../specs/ui/requirements/app-status-bar.md) and [system design](../../specs/ui/system-design/app-status-bar.md). Own the built-in metrics component, its existing tests, both resource-metrics Playwright specs, and the corresponding operations/layout documentation. Exclude plugin UI, collection, settings persistence, and navigation changes. No dependencies or delegation.

## Acceptance

1. Phone Menu and Status show one coherent group: Host alongside the heading, three equally sized columns, detailed labels and values, and meters below the values. Every enabled reading remains visible.
2. Narrow phones and breakpoint transitions retain containment, one parent scroller, focus return, and the compact desktop presentation.
3. Loading/subscription behavior, simplified mode, thresholds, formatting, and source filtering remain intact. Capture fresh isolated mobile and desktop screenshots for the PR.

## ASCII UI preview

UI-01 from the [full preview](plan.md#ascii-ui-preview), mapped to AC-UI-APP-STATUS-BAR-001.2, .6, .7:

```text
+-------------------------------------+
| SYSTEM METRICS                 Host  |
| CPU          Memory         Disk    |
| 3%           52%            72%     |
| [meter]      [meter]        [meter]  |
+-------------------------------------+
```

Desktop retains UI-02's inline bar. The phone card shares the existing drawer scroller. The drawing defines grouping and reading order, not exact pixels.

## Verification

From `apps/web`:

```sh
pnpm exec vitest run components/system-metrics
pnpm exec eslint components/system-metrics/status-surface-metrics.tsx components/system-metrics/status-surface-metrics.test.tsx e2e/tests/settings/mobile-resource-metrics-display.spec.ts e2e/tests/settings/resource-metrics-display.spec.ts
pnpm run typecheck
pnpm run i18n:ratchet
CAPTURE_PR_ASSETS=true pnpm e2e:run --host --project mobile-chrome e2e/tests/settings/mobile-resource-metrics-display.spec.ts --retries=0
CAPTURE_PR_ASSETS=true pnpm e2e:run --host --no-build e2e/tests/settings/resource-metrics-display.spec.ts --retries=0
```

Preserve mobile assets before the second capture run and merge the manifests afterward. From the repo root:

```sh
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Results

RED reproduced the old five-reading grid as three rows instead of two. All listed checks passed: 11 unit tests, 3 mobile Playwright tests, 2 desktop Playwright tests, ESLint, typecheck, i18n ratchet, specification/catalog validation, and public-doc validation (62 validator tests). The final mobile run used `--no-build` after a fresh successful production build; only the test's scroll reachability assertions changed between runs. Prettier and whitespace checks passed. Phone and desktop screenshots were captured with disposable E2E data and visually inspected against UI-01/UI-02.
