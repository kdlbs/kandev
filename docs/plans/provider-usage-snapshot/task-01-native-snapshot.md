---
id: 01-native-snapshot
title: Native provider usage snapshot in agent settings
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-COSTS-PROVIDER-USAGE-SNAPSHOT-001
acceptance_criteria:
  - AC-COSTS-PROVIDER-USAGE-SNAPSHOT-001.1
  - AC-COSTS-PROVIDER-USAGE-SNAPSHOT-001.2
  - AC-COSTS-PROVIDER-USAGE-SNAPSHOT-001.3
  - AC-COSTS-PROVIDER-USAGE-SNAPSHOT-001.4
  - AC-COSTS-PROVIDER-USAGE-SNAPSHOT-001.5
system_design:
  - ../../specs/costs/system-design/provider-usage-snapshot.md
---

# Native snapshot

Implement registry/config/profile/frontend flag wiring, snapshot DTO/controller/
handler, backend composition and account binding, Go client and ordinary agent
profile display. Preserve Office/hold/order/history. Exclude admission, learning,
statistics, cache-backoff changes, recurring polling and model probes.

## Acceptance

1. Enabled API/settings show shared-account provider readings and honest unknowns.
2. Disabled API/UI perform no new provider fetch; existing Office remains usable.
3. Only bounded source metadata leaves the backend and desktop/phone share UI-01.

## ASCII UI preview

UI-01 from [the plan](plan.md#ascii-ui-preview): existing name/model/fallback and
actions, followed by wrapping provider/account/class, window/state and a 44px
Refresh control. Loading/unavailable/no-API replace the percentage explicitly.

## Targeted verification

From `apps/backend`:

```text
go test -p 2 ./internal/runtimeflags ./internal/common/config ./internal/profiles
go test -p 2 ./internal/agent/usage ./internal/agent/settings/controller ./internal/agent/settings/handlers ./internal/backendapp -run 'TestUsageService|TestTeamClaude|TestCodexFetchUsage'
```

From `apps`, after `pnpm install --frozen-lockfile`:

```text
pnpm --filter @kandev/web test -- lib/state/slices/features/features-contract.test.ts
```

From `apps/web`: typecheck, changed-file lint, i18n checks and disposable rendered
desktop/phone/fake-transport proof. Run `git diff --check` before root handoff.
No production provider, task, credential or operating DB mutation.

## Source and verification boundary

The source lineage is 56b1aecc5, 2a6090e05 and bc4bd0b7b. The native
port retains the current shared cache and strict observed-value metadata.
Verification uses fake HTTP responses and isolated credentials created by the
fixture, never an installed credential or live provider. Exact port results are
recorded with the implementation handoff; historical tests are not presented as
new executions on the current branch.


## Stale shared-cache refresh follow-up

The snapshot now carries the existing cache stale marker through the DTO and
client type. A localized failed-refresh notice is shown with the unchanged real
windows and original fetched-at time. Cache policy, polling and Office behavior
are unchanged. Disposable real cache failure/DTO and English/Korean DOM checks
passed, including a prior measured zero, retained timestamp, fresh-reading notice
removal, refresh action, and feature-off/dynamic rows. No permanent cases added.
