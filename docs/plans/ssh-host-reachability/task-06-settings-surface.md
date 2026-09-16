---
id: "06-settings-surface"
title: "Reachability data layer and settings surface"
status: pending
wave: 4
depends_on: ["04-reachability-api-and-events"]
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-REACHABILITY-002
acceptance_criteria:
  - AC-EXECUTORS-SSH-REACHABILITY-001.25
  - AC-EXECUTORS-SSH-REACHABILITY-002.1
  - AC-EXECUTORS-SSH-REACHABILITY-002.2
  - AC-EXECUTORS-SSH-REACHABILITY-002.3
  - AC-EXECUTORS-SSH-REACHABILITY-002.4
  - AC-EXECUTORS-SSH-REACHABILITY-002.5
  - AC-EXECUTORS-SSH-REACHABILITY-002.10
  - AC-EXECUTORS-SSH-REACHABILITY-002.11
  - AC-EXECUTORS-SSH-REACHABILITY-002.12
system_design:
  - ../../specs/executors/system-design/ssh-reachability-surfaces.md
---

# Task 06: Reachability Data Layer and Settings Surface

## Summary

Add the client types, API calls, and store slice for reachability records, and
render one on the SSH executor settings page: state, probed host, failure
reason and message, consecutive-failure count, and the age of the last probe,
with a probe-now action. Localized in all five supported locales.

## In scope

- `SSHReachabilityRecord` in `lib/types/http-ssh.ts` mirroring the backend
  shape, including `probing_enabled`, `probe_interval_seconds` (the
  **effective**, clamped interval), and `updated_at` alongside `checked_at`
  and `last_success_at`.
- `getSSHReachability`, `getSSHExecutorReachability`, and
  `probeSSHExecutorReachability` in `lib/api/domains/ssh-api.ts`, following the
  existing options-spread-then-required-fields ordering in that file.
- Settings slice state keyed by executor id, plus the
  `executor.reachability.changed` WS handler that applies a pushed record. A
  refetch racing a pushed event is reconciled by keeping whichever payload
  carries the **later `updated_at`**, never `checked_at` — a
  connection-configuration reset clears `checked_at` (it is `null`) while
  still advancing `updated_at`, so reconciling on `checked_at` would make a
  reset compare as older than the record it just invalidated and get
  discarded, leaving a stale `reachable`/`unreachable` state showing for a
  host the user just re-pointed. A `null` `updated_at` (the synthesized
  never-probed shape) always loses to a real record.
- A `SSHReachabilityCard` rendering state, the host that was actually probed,
  reason and message when `unreachable`, the consecutive-failure count, and
  the age of the last completed probe; marking the result stale past three
  intervals; stating plainly that periodic probing is off when
  `probing_enabled` is `false`, rather than presenting a retained state as
  current; and offering the probe-now action.
- Refetch on mount and on the configured interval (`probe_interval_seconds`,
  not a constant) while the card is open, because a steady host publishes
  nothing and `checked_at` would otherwise age in place. **No timer runs at
  all when `probing_enabled` is `false`** — the card fetches once on open and
  again after an immediate probe; reading a `0` effective interval literally
  as a cadence would be a zero-delay refetch loop against a record known not
  to be changing.
- A failed load reports that reachability is not known. It must never render
  as `unreachable`.
- Mobile-viewport layout, and state exposed to assistive technology as text
  rather than by color alone.
- Copy through `t()` in `executors.json` across `en`, `pt-pt`, `zh-cn`,
  `zh-hk`, `zh-tw`, with the failure reason rendered as translated copy rather
  than the raw token. Use `pnpm run i18n:zh-hant` for the Traditional Chinese
  pair.

## Out of scope

- The pre-launch warning. Task 07 owns it. The task-card indicator is
  deferred; see the requirement document's `## Out of scope`.
- Any backend file.
- Changing the existing connection, sessions, agent-readiness, or
  task-dir-reclamation cards beyond mounting the new one.
- Any history, trend, or sparkline. One record per executor is all that exists.

## Acceptance

- The card renders every field the settings surface owes for each of the three
  states, and a record older than three intervals renders as stale rather than
  current.
- A pushed `executor.reachability.changed` updates the card without a reload
  and without the card polling for it.
- A pushed record with a `null` `checked_at` but a newer `updated_at` (a
  reset) replaces a stored `reachable`/`unreachable` record rather than being
  discarded as stale.
- With `probing_enabled` false the card says periodic probing is off, runs no
  refresh timer, and a load failure says reachability is not known — neither
  renders as `unreachable`.

## Verification

```bash
# From apps/ (once per fresh worktree):
pnpm install --frozen-lockfile

# From apps/web:
pnpm vitest run components/settings/ssh-reachability-card.test.tsx lib/state/slices/settings/settings-slice.test.ts lib/api/domains/ssh-api.test.ts
pnpm run typecheck
pnpm run lint
pnpm run i18n:check
pnpm run i18n:ratchet
```

## Files likely touched

- `apps/web/lib/types/http-ssh.ts`
- `apps/web/lib/api/domains/ssh-api.ts`
- `apps/web/lib/state/slices/settings/types.ts`
- `apps/web/lib/state/slices/settings/settings-slice.ts`
- `apps/web/lib/state/slices/settings/settings-slice.test.ts`
- `apps/web/components/settings/ssh-reachability-card.tsx`
- `apps/web/components/settings/ssh-reachability-card.test.tsx`
- `apps/web/components/settings/ssh-settings.tsx`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/executors.json`

## Dependencies

Task 04. The record shape, the `probing_enabled` flag, and the change action
are that task's contract.

## Risks

- Translations gate the build: `check-i18n-keys.mjs` fails on a missing key, an
  extra key, a dropped placeholder, or a value left identical to English. The
  six reason tokens plus the state labels are twelve-plus keys across five
  locales, so this is the largest single source of build failure in the task.
- The staleness threshold and the refresh cadence both depend on the effective
  interval. The record carries it as `probe_interval_seconds`; use that value
  rather than a constant, or both rules silently drift from the operator's
  configuration.
- Reconciling on `checked_at` instead of `updated_at` is the one regression
  that passes every test written against a normal probe sequence and only
  fails on a reset, because a reset is the single write that advances
  `updated_at` without advancing `checked_at`. Write the reconciliation test
  around a reset payload specifically, not just a newer/older probe pair.
- `i18next/no-literal-string` is a lint error repo-wide and
  `check-nonjsx-copy.mjs` scans non-JSX positions by exclusion. A reason-to-copy
  lookup table written as a plain `.ts` map is exactly the shape that rule
  catches.

## Parallelism

`sequential`

## Inputs

- System design, sections *API and event contracts* and *Frontend components*.
- `apps/web/AGENTS.md` for the store-slice and WS-hook conventions and the
  "never fetch data directly in components" rule.
- `components/settings/ssh-agent-readiness-card.tsx` as the nearest existing
  card shape.
- `docs/i18n.md` for the five-locale workflow.

## Results

Pending.
