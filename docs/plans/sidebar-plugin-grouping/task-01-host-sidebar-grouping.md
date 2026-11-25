---
id: 01-host-sidebar-grouping
title: Generic complete sidebar grouping
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-UI-SIDEBAR-PLUGIN-GROUPING-001
acceptance_criteria:
  - AC-UI-SIDEBAR-PLUGIN-GROUPING-001.1
  - AC-UI-SIDEBAR-PLUGIN-GROUPING-001.2
  - AC-UI-SIDEBAR-PLUGIN-GROUPING-001.3
  - AC-UI-SIDEBAR-PLUGIN-GROUPING-001.4
  - AC-UI-SIDEBAR-PLUGIN-GROUPING-001.5
  - AC-UI-SIDEBAR-PLUGIN-GROUPING-001.6
system_design:
  - ../../specs/ui/system-design/sidebar-plugin-grouping.md
---

# Task 01: Generic Complete Sidebar Grouping

## Summary

Deliver the generic contract, bounded authorized query and saved-view UI as one
vertical host slice, verified with an installed fixture. Do not add a Tag-specific
host branch or treat the existing page-local facet as complete.

## In scope

- Manifest declarations, SDK DTOs, boot metadata, action resolver and complete
  response validation with authorization before plugin execution.
- Registry-independent saved group identity through backend and TS settings,
  boot, HTTP, WS, workspace journals and normal view selection/editing.
- Parameterized repository grouping before pagination, source/lifecycle query
  identity, unique task versus occurrence counts and colored group entries.
- Both desktop and phone entry points, unavailable-provider/error/retry states,
  draft/name/limit protections, duplicate-safe selector creation and row keys.
- Generic fixture and regression/browser coverage; public API/sidebar docs.

## Out of scope

Real Tags implementation, plugin publication, layout-panel changes, direct
plugin-state reads, whole-task browser fetches and /tasks facet redesign.

## Acceptance

1. An installed generic complete provider is directly selectable and becomes a
   normal saved workspace view on desktop and phone, surviving reload and view
   editing without changing other workspaces or dirty drafts.
2. Real backend queries group 101+ tasks and repeated trees before page slicing;
   counts, collapse, pins, parent context and authorization remain correct.
3. Unsupported/incomplete/failed providers retain saved identity and show explicit
   recovery; stale replies cannot cross user/workspace/provider generations.
   Existing built-in sidebar and /tasks facet controls remain green.

## ASCII UI preview

UI-01/UI-02 from [the plan](plan.md#ascii-ui-preview), AC-001.1/.2/.3/.6:

```text
Desktop TASKS [Tag v] [Filters]   Phone Task views drawer
  Plugin views > Tag             [Tag v] [Filters] [Close]
  Layout testing (4)               Layout testing (4)
    task rows                       task rows (scroll)
  Untagged (2)                    Untagged (2)
  [Previous] [Next]              [Previous] [Next] + safe area
```

Existing saved entries stay in the selector. Plugin views are a separate section;
once created, the view is ordinary editable saved state. Reuse mobile picker
and task-switcher composition, not a compressed desktop workbench.

## Verification

Run from the cache-drive build mirror with TMPDIR, GOTMPDIR, GOCACHE,
GOMODCACHE, Vite caches and build outputs on findmnt-verified cache paths.
Fresh worktrees need workspace dependencies first; check all UI package links
before reuse. The focused desktop/phone files and fixture are implemented.

```bash
make fmt
make -C apps/backend build-dev build-e2e-delivery-fixture build-e2e-send-now-fixture e2e-plugin-package
pnpm --dir apps/web run build:e2e
(cd apps/backend && go test -trimpath -tags fts5 ./internal/plugins/... ./internal/user/... ./internal/task/models/... ./internal/task/service/... ./internal/task/repository/sqlite/... -run 'Sidebar|PluginGrouping|Manifest|Action')
(cd pluginsdk && go test -trimpath ./... && go vet ./...)
pnpm --dir apps/web exec vitest run components/task/sidebar-filter lib/sidebar/sidebar-plugin-grouping.test.ts lib/state/slices/ui/sidebar-workspace-views.test.ts lib/state/slices/ui/sidebar-view-wire.test.ts
pnpm --dir apps/web run typecheck
pnpm --dir apps/web run lint
pnpm --dir apps/web e2e:run --host --no-build --project chromium tests/plugins/sidebar-plugin-grouping.spec.ts tests/plugins/task-list-facet.spec.ts tests/task/workspace-sidebar-views.spec.ts --retries=0
pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome tests/plugins/mobile-sidebar-plugin-grouping.spec.ts tests/plugins/mobile-task-list-facet.spec.ts tests/task/mobile-workspace-sidebar-views.spec.ts --retries=0
make -k typecheck test lint GOFLAGS=-p=2
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Changed-package coverage and Oxlint must include any additional actual touched
paths. Managed E2E must build final source before runtime proof; don't reuse
old artifacts. Record any requested full `make typecheck test lint` environment
limits separately; never substitute a narrow pass for a full-gate claim.

## Files likely touched

- `apps/packages/plugin-sdk/src/index.ts`
- `apps/backend/internal/plugins/manifest/` and `store/` metadata projection
- `apps/backend/internal/plugins/action_handlers.go` and new internal resolver
- `pluginsdk/` typed response helpers
- `apps/backend/internal/task/models/sidebar_task_view.go`
- `apps/backend/internal/task/service/sidebar_task.go`
- `apps/backend/internal/task/repository/sqlite/sidebar_task_query*.go`
- `apps/backend/internal/user/models/` and `service/sidebar_workspace_views.go`
- `apps/backend/internal/backendapp/` resolver wiring and boot projection
- `apps/web/lib/plugins/registry.ts`, `types.ts`, `lib/types/http.ts`
- `apps/web/lib/state/slices/ui/sidebar-view-{types,wire,actions}.ts`
- `apps/web/components/app-sidebar/sections/tasks-view-picker.tsx`
- `apps/web/components/task/sidebar-filter/group-picker.tsx`
- `apps/web/components/task/task-session-sidebar-grouped-view.ts`
- `apps/web/components/task/mobile/mobile-task-list.tsx`
- New `apps/web/hooks/use-sidebar-groupings.ts` and grouping projection tests
- `apps/backend/cmd/plugin-fixture/` and the new desktop/mobile E2E files
- `docs/public/plugins-authoring.md`, `plugins-manifest.md`, and sidebar user guide

## Dependencies

None. Query existing symbol references/LSP before exported contract changes.

## Risks

Complete data must not be inferred from a partial facet. No database transaction
may enclose plugin RPC. A repeated task occurrence is not another domain task;
action, navigation and ownership must remain canonical task-ID based.

## Parallelism

sequential

## Inputs

- [Requirement](../../specs/ui/requirements/sidebar-plugin-grouping.md)
- [Design](../../specs/ui/system-design/sidebar-plugin-grouping.md)
- Existing workspace views, pagination and page-local facet consumer suites.

## Results

Implemented in the independent transferred task after the user's explicit
implementation request. The initial main baseline was
`b4c6a197129b5b4a5e99c3e1dab132ada9fb433f`; no origin-task feature commits were
cherry-picked. The current Go SDK owner is standalone `pluginsdk/`, not the
removed backend package. No language servers are configured, so exported
contracts were reconciled through targeted reference searches.

The generic contract resolves authenticated workspace actions before opening
the repository read transaction. Complete snapshots classify authorized,
filtered roots before the 100-row window. Canonical task hydration, repeated
tree occurrences, group counts, source identity, pin ordering and continuation
context remain separate. Saved references and generated-view markers use the
existing workspace journals. Declarations require minimum host `0.102.0`;
no release or SDK publication occurred.

Observed failing-before/passing-after regressions include invalid UTF-8,
disable/enable ABA, per-occurrence continuation context, repeated descendant
counts, stale metadata/page hydration, unavailable summaries, wide-touch paging,
long labels, reduced-root-font targets, and nested phone drawer geometry after
selection or provider removal. Drawer closure goes through the existing close
primitive so the parent restores its geometry.

Final exercised checks:

- Full affected SQLite repository package passed: `945.907s`, with
  `go test -trimpath -tags fts5 -timeout 30m`. The earlier default 10-minute
  package deadline was insufficient; it is not recorded as a passing run.
- All four complete-grouping conformance cases passed on both SQLite and a
  real isolated PostgreSQL 18.6 server. The verified-snapshot guard also passed;
  the PostgreSQL cases were not skipped.
- Plugin/manifest/action and gateway selections passed after the lint-driven
  helper extractions. Standalone SDK tests and vet passed.
- 559 focused Vitest tests in 49 files, native web typecheck and final classic
  typecheck across all five TypeScript workspaces passed.
- Current-artifact Playwright: 7 desktop and 6 phone tests passed with one
  worker and retries disabled. These include 3 desktop grouping cases,
  4 phone grouping cases, and existing facet/workspace controls.
- Rendered coverage includes 103 unique tasks, repeated trees and two
  occurrence pages, live changes, saved workspace journals, unavailable/recovery,
  320px containment, full long labels, keyboard focus and 44px targets with a
  12px root font.
- Required `make fmt`, complete `make lint`, SQL guard and final backend/UI
  artifact builds passed. CLI and complete script tests also passed.
  The plan records the non-green broad backend/web outcomes separately.

Pure picker markup is exercised through real browser controls. The old
pagination CSS-class/mock-forwarding test and plugin manifest-copy assertions
were removed rather than re-pinned; boundary and native geometry coverage remain.
Managed logs and PNGs are retained under the cache-drive build mirror's
`verification-logs` / `evidence`, and the owned smoke root's `final-logs`.
A first normal-hook attempt surfaced a duplicated cleanup failure token in
existing task-service code. Archive and delete paths now share the unchanged
private string constant; literal persisted-token fixtures remain independent.
The existing inspection/retained cleanup regression selection passed (`2.398s`).
Host implementation commit:
`c42775f671cef7532065c054669c62dbd5970275`.
Normal signed commit passed every applicable pre-commit and commit-message hook.
The plan and task-plan appendices record the continuing adversarial review loop.

### Round 2 disclosure correction

Save as and duplicate discarded collapsed plugin groups in an inherited shared
saved-view path, contrary to AC-001.2. Both state/journal regressions failed
before correction; all 27 action/workspace tests passed afterward. A draft
keeping its grouping clones the base disclosure, while a changed grouping starts
expanded. Duplicate clones source disclosure, and later expansion is independent.
Native Save as also reproduced the defect against the published pre-fix build:
the copied Alpha heading was expanded rather than collapsed. This is a shared
state correction with no desktop/phone composition or touch geometry change.

Final focused verification passed: 36 action/workspace/wire/sync tests, native
typecheck and document validation. Native browser suites passed with one worker
and no retries: six desktop and five phone scenarios. The new Save as regression
keeps the copied heading collapsed after save/reload and proves source independence.
Fresh real Tags package proof is recorded in Task 02.

### Round 4 configuration lifecycle correction

The service regression reproduced missing catalog invalidation after a successful
configuration restart, a save with a stopped runtime and a failed config commit.
Restart failure was the positive control: its existing deliverer path notified.
Every admitted policy rotation now defers sidebar catalog invalidation until the
operation completes. Existing MCP timing, dispatch leases and locks are unchanged.

AC-001.4 owns the lifecycle invariant: a connected saved view must stop using its
old provider generation. The real service/FSStore/event-bus regression covers
successful restart, stopped runtime, failed write and failed restart without
depending on runtime health callbacks. The native scenario requires a completed
fresh provider-generation response before one Next reaches page two, retaining
the ordinary saved view. An unrelated or ungrouped response cannot satisfy it.
Cross-generation reply and workspace fences retain their existing regressions;
this change adds no task/session scheduling, ownership or replay mechanism.

Final verification passed: `go test -race ./internal/plugins -count=1`
(`23.762s`), required `make fmt`, changed-package Go lint and catalog/spec lint.
Rebuilt the complete managed runtime/helper/fixture artifact set without
bypassing timestamp guards. Native suites passed: seven desktop and five phone
scenarios, one worker and no retries. The final config-transition scenario
also passed separately after its observer explicitly waited for completed
responses and rejected canceled stale reads. Its page-two PNG and the current
phone pager/chooser PNGs were visually inspected. Evidence is retained under
the cache-drive `sidebar-grouping-review4-4v7ftoo0` root.

### Round 6 committed lifecycle cleanup correction

Disable and unhealthy-runtime regressions reproduced committed Disabled/Error
status with no catalog event when automation cleanup failed. Both real FSStore
rows and the registry retained the inactive state. Their failed-status-write
controls preserved Active, the existing provider generation and no event.
Catalog invalidation is now deferred only after successful status persistence,
so subsequent cleanup errors cannot hide a committed provider transition.
Existing cleanup errors, fail-closed state, dispatch locks and MCP behavior remain.
The complete owning plugin package passed `-race` (`29.562s`); the final four-case
regression was rerun after removing redundant payload-shape coverage.
The existing config regression owns catalog payload privacy; this regression
owns committed-state notification and unavailable-provider behavior.
Final changed-package lint, required `make fmt` and catalog/spec validation passed.
Rebuilt all guarded runtime/helper/fixture artifacts. Native acceptance passed
seven desktop and five phone scenarios with one worker and no retries, including
the existing unavailable/recovery and generation/pagination consumers.
Evidence is under the cache-drive `sidebar-grouping-review6-5cq96b32` root.

### Round 9 committed catalog snapshot correction

Concurrent catalog reads previously exposed provisional registry status while its
status Save was blocked, including values later rolled back. Six channel-barrier
cases failed before correction: active to Disabled/Error and inactive to Active,
each with successful and failed persistence. Catalog capture now holds the existing
status-persistence mutex only while cloning registry records; projection and event
callbacks remain outside that guard. Failed commits still publish no notification.
The one-second bounded pending-write observation window rejects early authoritative
membership; channels own the write/admission/release ordering, without sleeps.
The complete owning plugin package passed `-race` (`28.877s`), including all six
snapshot cases and previous completion-fence regressions. Changed Go lint passed.
Required `make fmt`, catalog/spec validation and complete guarded artifact rebuild
passed. Native acceptance passed seven desktop and five phone scenarios, one
worker and no retries. Exact persistence-failure scheduling is owned by the
service regression, not a new runtime-only testing flag.
Evidence is under the cache-drive `sidebar-grouping-review9-p3gv5v37` root.

### Round 11 durable policy publication correction

The status capture guard did not cover provisional config-policy publication.
Pending policy Save exposed a generation that could later roll back without
notification. Both commit/rollback cases reproduced the incorrect pending catalog.
Policy rotation now saves its existing private record copy before publishing it
to the registry, retaining the policy and record-persistence write serialization.
The obsolete registry mutator was removed; no new lock or rollback event is added.
The same channel barriers overlap resolver capture with the real dispatch lease:
rollback keeps the unchanged healthy provider available; successful config change
rejects the old in-flight generation. Failed policy persistence preserves the
original catalog, including its invalidation epoch. Full plugin `-race` passed
(`26.695s`), including policy and status completion/rollback cases.
Required `make fmt`, changed Go lint and catalog/spec validation passed. All
guarded runtime/helper/fixture artifacts rebuilt; native acceptance passed seven
desktop and five phone scenarios with one worker and no retries.
Evidence is under the cache-drive `sidebar-grouping-review11-3lkcw9vo` root.

Review closure: rounds 12/13/14 each reported CLEAN, zero findings, on host
`763babaf87d29b4e767f7285b409a80f792b713d` and the unchanged Tags snapshot.
The three-consecutive-clean gate is satisfied. The plan retains broad test
limitations; completion here does not mean all repository tests are green.

### Post-rebase round 2 corrections

The second post-rebase review identified lifecycle lock inversion, unscoped
uninstall fencing, authorization-denial cache ABA, oversized disclosure keys and
anonymous boot catalog exposure. Status epoch advancement now shares the durable
commit boundary; both guards release before cleanup/publication. Removal emits
the plugin identity. Access denial retires cached/in-flight plugin identities.
Value disclosure uses fixed-size framed digests; Untagged remains distinct.
Anonymous enforced-auth boot no longer advertises backend-only providers.

Regression coverage includes synchronous subscriber lock admission and concurrent
unrelated metadata progress, real service uninstall publication, cached/deferred
pages through denial and identical rediscovery, 260 maximum-identity groups below
the public query budget, and anonymous/signed-in/auth-off boot. A native 320-value
saved-disclosure scenario and updated desktop/phone consumers own hash cutover
proof. Verification receipts and the next review snapshot are recorded in the
platform task plan; earlier sections remain historical evidence.
The status-admission barrier also reproduced same-plugin overtaking after global
guard release. A per-plugin completion guard preserves the existing cleanup
order and releases before synchronous publication. The native disclosure fixture
supplies explicit workflow identity and permits a 1 MiB test-writer input; the
production action response and public query caps remain unchanged.

