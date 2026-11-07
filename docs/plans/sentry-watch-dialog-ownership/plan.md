---
created: 2026-10-22
status: done
requirements:
  - REQ-INTEGRATIONS-SENTRY-WATCH-DIALOG-001
system_design:
  - ../../specs/integrations/system-design/sentry-watch-dialog-lifetime.md
legacy_specs: []
---

# Implementation Plan: Sentry Watch Dialog Ownership

## Overview

The source-proven old PATCH completion publishes its original accepted query
and success toast, then unconditionally closes a reopened form. Bound completion
and busy cleanup to the mounted opening while preserving accepted writes.
The user authorizes this package and TDD in one session. Stop at a clean normal-hook
commit and external parent-validation packet; push/PR require later authorization.

## Scope and approach

Only `components/sentry/sentry-issue-watch-dialog.tsx`, necessary local tests/helpers,
focused browser fixtures, this vertical package and the Sentry watch public paragraph.
Follow the [design](../../specs/integrations/system-design/sentry-watch-dialog-lifetime.md).
Exclude instance forms, health, other integrations, Query/store/backend redesign.

## ASCII UI preview

UI-01: Settings > Sentry > watcher row, Cancel pending Update, reopen.
Shared desktop/phone composition; existing scrollable dialog owns content and
footer. Phone entry is the same tappable row on the dedicated settings page.

```text
Before old ACK: [Edit watcher | raw newer query | ...] [Cancel] [Update]
Defect:        [Watcher list]                        (dialog disappears)
After repair: [Edit watcher | raw newer query | ...] [Cancel] [Update]
Newer busy:   [Edit watcher | newer submitted query] [Cancel] [Saving...]
```

Structure and continued actions are required; spacing is illustrative. No new
layout is proposed. Maps to AC .1, .3 and .5; browser checks include native taps.

## Work orders

- [x] [Task 01: Preserve current watch](task-01-preserve-current-watch.md)

## Verification strategy

Preserve external pre-TDD package timestamp/hash and immutable production source.
Run real-section/provider/domain/API/fetch tests with only HTTP controlled and an
unrelated Monaco renderer adapter. Prove causal RED plus ordinary control, then
minimal GREEN. Cover replacement, busy, success/failure, original payload/list/toast,
unmount/scope, passive hydration, defaults, next Save/Cancel and current retry.
Browser checks use one worker/shard, retries zero, strict guards, fresh production
build and owned unique workspace/repository/offline origin plus explicit existing
Worktree executor. Preserve seed/source/task HEAD and full raw porcelain, aggregate
primary/cleanup errors and exercise a failure cleanup control. No operator data.
Run focused lint/type/i18n/spec/public/docs-coverage and active commit hooks.
Refresh open PR inventory and actual relevant patches before checkpoint.

## Risks and results

Scope is established by real parent keyed workspace lifetime, not list object
identity. Component transport proof does not establish browser or physical proof.
External packet: `../sentry-watch-ownership-packet/` (outside the repository).

## Verification results

- Frozen install passed under Node 24.18.0/pnpm 9.15.9 before tests/hooks.
- Immutable-original causal RED: one intended missing-dialog failure after
  original query publication and truthful success toast, one passing owned control.
  The source SHA256 matches the parent proof. Pre-TDD package/timestamp is external.
- Final focused consumer/controls: 4 files, 43 tests passed (20 lifetime cases).
  Real section/providers/domain/API/fetch; only HTTP and unrelated Monaco adapted.
  Includes same-ID newer pending save, stale success/error, unmount success/error,
  actual route/active workspace replacement, passive hydration and suspended render.
- Fresh managed production build; final desktop 3 passed and emulated Pixel 5
  native taps 2 passed, each with one shard/worker, retries zero, strict guards.
  Subsequent runs reused the same fresh assets: production source hash unchanged.
  Browser checks retain accepted original persistence, real parent publication,
  exact newer raw query/focus/caret, next Save/Cancel and reload persistence.
- Deliberate failure cleanup control passed. All 16 attempt/control fixture receipts
  have zero cleanup errors and absent owned source/origin roots. Final receipts
  record full task/seed/source HEAD and raw porcelain before, during and after flow.
  Owned Worktree profiles were deleted; managed backends/browser fixtures stopped.
- Changed-file ESLint (zero warnings), documented 8 GiB typecheck, rich-output
  catalog, i18n check and ratchet passed. Spec catalog/fullspec and 36 spec-linter
  tests passed; public docs validated 54 pages with 63 validator tests passing.
  Documentation coverage is covered by this work order and vertical pair.
- Public docs assessment: updated only the Sentry watch paragraph in the existing
  integration reference/how-to. Existing instance/GitLab/Jira/Linear text is intact;
  there is no paragraph overlap with the sibling Linear work.
- Refreshed 69 open PRs and all paginated files for #1512/#4517/#3143/#4444.
  Heads unchanged. #1512's actual dialog replaces data sources while retaining
  unconditional awaited close/finally; the other inspected patches have no Sentry
  watcher target. This is a bounded inspected-target finding, not backlog review.
- Setup/type/selector/focus-fixture failures remain separately recorded externally;
  they are not counted as causal RED. Phone proof uses browser emulation, not a
  physical device, OS keyboard, real credentials/provider or operator runtime.
- Normal-hook commit and exact full-diff/HEAD/resource receipts are collected in
  the external checkpoint packet. Parent must review and independently rerun before
  separate same-session delivery authorization. No push or PR is authorized.
