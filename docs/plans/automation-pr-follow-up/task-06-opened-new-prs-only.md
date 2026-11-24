---
id: "06-opened-new-prs-only"
title: "Fire opened only for new pull requests"
status: done
wave: 4
depends_on:
  - "01-new-commits-event"
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-003
acceptance_criteria:
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.10
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-003.6
system_design:
  - ../../specs/office/system-design/automations-pr-follow-up.md
---

# Task 06: Fire `opened` only for new pull requests

## Summary

Live testing showed that saving a trigger on a repository with open pull
requests starts a task for each of them, and that a capacity skip then
consumes a pull request's only `opened` key. The user chose a new default:
`opened` fires only for pull requests created after the trigger, with an
opt-in **Also run on pull requests already open** checkbox. The same change
stops `opened` from resubmitting a held dedup key on every poll, which logged
a unique-key warning per pull request per minute.

## In scope

- `GitHubPRTriggerConfig.IncludeExistingPRs` (`include_existing_prs`).
- `opened` eligibility from the PR's `created_at`, the trigger's
  `created_at`, and the observation's `tracked_events`, which now includes
  `opened`.
- A `HasRunWithDedupKey` check before an `opened` firing.
- The editor checkbox and its copy in every locale, the MCP schema field, and
  public docs.

## Out of scope

- Changing `opened` dedup keys or capacity-skip semantics.

## Acceptance

1. Evaluator tests prove that a PR created before the trigger does not fire,
   that a PR created after it fires once, that an unknown creation time fires,
   that `include_existing_prs` restores backfill, and that a PR observed
   before `opened` was selected does not fire.
2. A test proves that steady-state polling does not call admission for a held
   `opened` key and records no skipped run.
3. The editor shows the checkbox only with **Opened** selected and saves the
   field on desktop and phone.

## Verification

```bash
(cd apps/backend && go test -trimpath ./internal/automation/ -run 'Opened|PRTrigger|NewCommits|ReviewRequested' -count=1)
(cd apps/backend && go test -trimpath ./internal/mcp/server/ -run 'ConfigAutomation' -count=1)
(cd apps && pnpm --filter @kandev/web test -- components/automations/trigger-configs/github-pr-config.test.tsx)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/automations-pr-follow-up.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/mobile-automations-pr-follow-up.spec.ts)
```

## Parallelism

`sequential`

## Results

Done on 2026-11-24.

- `(cd apps/backend && go test -trimpath ./internal/automation/ -count=1)`: ok, including `TestOpened*` (created before or after the trigger, unknown creation time, `include_existing_prs`, a PR observed before `opened` was selected, and no resubmission of a held key).
- `(cd apps/backend && go test -trimpath ./internal/mcp/server/ -run 'ConfigAutomation' -count=1)`: ok.
- `(cd apps && pnpm --filter @kandev/web test -- components/automations/trigger-configs/github-pr-config.test.tsx)`: 13 passed.
- `(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/automations-pr-follow-up.spec.ts)`: 2 passed.
- `(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/mobile-automations-pr-follow-up.spec.ts)`: 1 passed.

Deviation: a row whose `tracked_events` lacks `opened` keeps it untracked until the row is pruned, so the information that a pull request was open before `opened` was selected survives later rewrites.
