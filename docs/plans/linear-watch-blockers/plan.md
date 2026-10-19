---
created: 2026-10-19
status: implemented
requirements:
  - REQ-INTEGRATIONS-LINEAR-BLOCKERS-001
  - REQ-INTEGRATIONS-LINEAR-BLOCKERS-002
system_design:
  - ../../specs/integrations/system-design/linear-watch-blockers.md
legacy_specs: []
---

# Implementation Plan: Linear Watch Blockers

## Overview

Add an opt-in watch setting that holds an issue until its Linear blockers are
completed ([issue #4583](https://github.com/kdlbs/kandev/issues/4583)). Two
work orders deliver the backend hold and the dialog setting, following the
[owning design](../../specs/integrations/system-design/linear-watch-blockers.md).

## Evidence

On `main` at `0e30c3cf0`, `issueFragment` requests no relations,
`CheckIssueWatch` publishes every unseen matching issue, and a default-order
watch reads one page. The watcher dispatch coordinator orders and caps
published issues but has no notion of a Linear dependency. Linear's public
GraphQL schema exposes blockers as `Issue.inverseRelations` nodes of type
`blocks` (`IssueRelationType`), each with the blocking `issue` and its
`state.type`.

## Scope

### In scope

- `REQ-INTEGRATIONS-LINEAR-BLOCKERS-001` and `-002` with all their criteria.
- Linear models, GraphQL client, mock client, watch service, watch dialog,
  watch table, locales, and public docs.

### Out of scope

- Other relation types, sub-issues, and the `/linear` browser.
- Changes to the orchestrator, the storage schema, or other providers.

## Work orders

- [x] [Task 01: Hold Linear watch issues until blockers complete](task-01-hold-blocked-issues.md)
- [x] [Task 02: Blocker setting in the Linear watch dialog](task-02-dialog-setting.md)

Task 02 depends on Task 01 for the stored setting.

## ASCII UI preview

`UI-01: Linear watch dialog, filter section` — entry point: Settings →
Integrations → Linear → New watcher (or a watcher row). State: setting on.
Structural requirement: a switch row directly under Query; details are
illustrative.

```text
Query
Free-text match across title and description (optional).
[ auth bug                         ]

Only start issues whose blockers are done            [on ]
Holds each matched issue until every Linear issue
blocking it is completed. A canceled blocker still holds it.
```

The phone layout is the same single column. Maps to
`AC-INTEGRATIONS-LINEAR-BLOCKERS-002.1`, verified by the desktop and mobile
Playwright specs.

## Verification

```bash
(cd apps/backend && go test -race -tags=fts5 -count=1 ./internal/linear/ ./internal/orchestrator/)
bash scripts/lint-go-changed <changed Go files>
(cd apps/web && pnpm exec vitest run components/linear/ && pnpm run typecheck && pnpm run i18n:check)
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/integrations/linear-watcher-blockers.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/integrations/mobile-linear-watcher-blockers.spec.ts
python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all
```

## Verification results

Author-reported results are recorded in each work order.

## Risks

- Watches with the setting on add a nested relation block to each search and
  read up to `issueWatchMaxPages` pages even in default order. Watches with
  the setting off are unchanged.
- An issue with more than 25 incoming relations is held until relations are
  removed, because its blocker list is incomplete.
- Pending (held or released but not yet started) issues are re-checked by ID
  with up to five extra requests per poll. Beyond 250 pending issues, the
  extra ones start only while they are in the search window.
- A canceled blocker holds its issue until the relation is removed or the
  setting is turned off.
