---
id: "03-ui"
title: "App-wide UI and localization"
status: done
wave: 3
depends_on: ['02-automation']
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-NOTIFY-001
  - REQ-AGENTS-RUNTIME-NOTIFY-002
acceptance_criteria:
  - AC-AGENTS-RUNTIME-NOTIFY-001.3
  - AC-AGENTS-RUNTIME-NOTIFY-001.4
  - AC-AGENTS-RUNTIME-NOTIFY-002.7
system_design:
  - ../../specs/agents/system-design/runtime-update-notifications.md
---
# Task 03: App-wide UI and localization

## Summary and scope

Share runtime status outside Settings, render notifications and a durable indicator with direct navigation, and add saved automatic-policy controls/manual guidance. Preserve existing update dialog and phone drawer. Localize all shipped catalogs.

## Out of scope

No model discovery, worker delegation, or global developer CLI/login changes. Native unverified activation remains manual.

## Acceptance

- The linked acceptance criteria hold across multiple registered identities.
- Failures preserve authoritative state and expose truthful recovery.
- Exact verification below passes, with results recorded.

## ASCII UI preview

Use UI-01 and UI-02 in [the plan](plan.md#ascii-ui-preview).

```text
[Agent runtime updates: 2] [View updates]
Claude | Current 0.81.2 / Latest 0.82.0 | [Manage versions]
Automatic updates [off]    [Save changes]
Cursor | Unknown version | [Vendor update guidance]
```

Phone wraps metadata into one column, provides 44px actions, page-owned scrolling, and the existing version drawer. Desktop retains 28px actions.

## Verification

```bash
(cd apps/web && pnpm exec vitest run lib/state/slices/ui/ui-slice.test.ts components/settings/use-runtime-auto-update-policy.test.ts lib/agents/runtime-update-statuses.test.ts lib/ws/handlers/notifications.test.ts lib/state/slices/settings/settings-slice.test.ts components/settings/agent-runtime-update-control.test.tsx hooks/domains/settings/use-agent-runtime-update-statuses.test.tsx hooks/use-update-available-toast.test.ts lib/api/domains/agent-update-api.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/agent-runtime-notifications.spec.ts tests/settings/agent-runtime-update.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-agent-runtime-notifications.spec.ts tests/settings/mobile-agent-runtime-update.spec.ts)
```

## Files likely touched

app/settings/agents/; components/settings/; components/update-available-toast-bridge; hooks/; lib/api/; lib/state/; lib/types/; locales/; focused e2e specs

## Dependencies and inputs

02-automation. Read the linked requirements/design and nearest source/tests.

## Risks

See the plan for native ownership, source failure, consent/selection races, and overlapping PRs.

## Parallelism

sequential

## Results

Passed 107 focused frontend tests, then the added origin/readiness regression brought the settings slice to 20 passing tests (108 total across the same nine files). Typecheck and strict affected-file lint passed. Localization validation passed English, Portuguese, Simplified Chinese, both generated Traditional Chinese catalogs, Japanese, newly shipped Korean, and the generated pseudo catalog.

Passed 17 desktop E2E cases, including the narrow fine-pointer phone layout, and six phone E2E cases. These cover named notifications, exact runtime links, persistent indicators, saved opt-in policy/reload, native manual guidance, unknown version states, retained failures, existing dialogs/drawers, rollback, return-to-default and retries. Captures wait for finite animations and are kept out of the merge branch.

A batch-notification regression demonstrated that one runtime notice could overwrite another before React rendered it. The queued notification store preserves each occurrence and suppresses duplicate queue entries. Shared request tests cover cached second consumers, initiating consumer unmount, queued forced refreshes and offline retention.

PR review remediation covers shared-source caller cancellation, consent-preserving manual admission and durable-outcome release before refresh callbacks. Native-host managed fallback controls preserve package selection/recovery without global native mutation or false host capability publication. Red/green regression tests pass for cancellation, rejected manual requests, terminal admission, verified native fallback update/rollback/default, strict policy JSON, save contributor identity, original outcome identity, and bootstrap readiness. Backend controller/handler/registry/backendapp tests pass with -race -tags fts5; five directly changed frontend suites pass 39 tests, plus two card-destination snapshot tests. TypeScript, all seven shipped locales, 18 desktop and seven phone E2E cases pass. External exact-head CI/review/merge gates remain pending.
