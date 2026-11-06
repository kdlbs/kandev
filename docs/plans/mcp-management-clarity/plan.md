---
created: 2026-10-22
status: done
requirements:
  - REQ-AGENTS-MCP-MANAGEMENT-001
  - REQ-AGENTS-MCP-MANAGEMENT-002
  - REQ-AGENTS-MCP-MANAGEMENT-003
  - REQ-AGENTS-MCP-MANAGEMENT-004
  - REQ-PLATFORM-MCP-SESSION-OBSERVABILITY-001
system_design:
  - ../../specs/agents/system-design/workspace-mcp-management.md
  - ../../specs/platform/system-design/mcp-session-observability.md
legacy_specs: []
---

# Implementation plan: MCP management clarity

## Overview

Deliver deferred points 3 and 4 from PR #3293. Build authorized usage reads
first, then their management UI and global-profile context, then neutral
session evidence. Execute the three work orders sequentially.

This is a rebuilt design package. The reported prior six-file package and
Platform patch were not recovered from this task or the accessible parent plan.
No historical validation result is adopted. All three work orders are now
implemented; their results record checks run on 2026-10-25.

## Baseline and authorization

- Task: `000d477e-f792-4c9d-8503-50ed73bb9646`.
- Own branch: `feature/improve-mcp-manageme-mpe`.
- Fetched parent: `feature/plan-workspace-mcp-c-dcc` at
  `193de56ba2597b335a97b1e5ab01d09340f22647`, verified 2026-10-22.
- Workspace: `/home/zeval/.kandev/tasks/improve-mcp-manageme_int5yuud/pr3293-followup-source-20261022`.
- User authorized beginning follow-ups before merge. No reviewed package was
  found. Stop after this design handoff until a later implementation request.

Publication checkpoint, 2026-10-24: PR #3293 merged as
`0b81ae48d4e2c358f80ac489e39388dfd8d7e6ab`. This branch was reconciled onto
current main `cc687bdce96bc22fb8a9ca87abe571b8309d3125`, which contains that
squash. The old branch had no task-owned commits; none of the old parent
commits were replayed. The original six design files were backed up first.

The user authorized committing and publishing this design-only checkpoint as a
**draft PR targeting main**. This supersedes the original pre-merge stacked-base
instruction. Implementation and its tests remain pending. The publication graph
is `main -> feature/improve-mcp-manageme-mpe ->
feature/clarify-mcp-assignme-mhh -> feature/allow-session-only-i-2gn`.
The independent built-in branch targets main: `feature/builtin-mcp-packages-6d1b`.
A published design does not satisfy assignment work order 03: it still requires
this package's implemented task 02 navigation/global-profile contract.

Keep parent and sibling branches/workspaces unchanged. No review handling, CI
waiting, merge, task/session creation, delegation, or autopilot is authorized.
Do not touch main runtime port 9998 or historical preview ports 48593/48595.

## Scope

### In scope

- Named, authorized direct usage and navigation from a catalog definition.
- Shared workspace-aware picker for saved global profiles, guarded context
  switching, legacy fallback clarity, and existing settings save ownership.
- Neutral delivered/unobserved evidence independent of saved selection status.
- Desktop, phone, accessibility, localization, and focused documentation.

### Out of scope

Session exclusions, assignment at registration, built-in management, new probes,
runtime reconnection, resolver changes, and automatic package/agent activity.

## Confirmed requirements and decisions

[Requirements](../../specs/agents/requirements/workspace-mcp-management.md)
own REQ-AGENTS-MCP-MANAGEMENT-001 through -004. Agents remains the vertical
owner because catalog selections are its durable contract. Platform retains
connection evidence ownership. Existing additive-selection and observability
ADRs remain in force. No new ADR is needed for these presentation extensions.

The registration sibling's saved scope was read through task
`5a8cb5cd-22ee-42e5-bd6d-e2179e899c8c` on 2026-10-22.
That scope requests reuse of a management destination when available, not a
second hub. A later same-day read confirmed its draft requirements/design and
explicit dependency on this package's navigation work. That sibling plans a
conditional-selection write extension and owns that change. The shared
destination proposed here is the existing catalog with `serverId` and
`view=usage`. The coordinator received the proposal. This package does not
depend on the sibling's implementation or assume its assignment flow exists.

The exact profile, repository, task, session, and catalog routes are recorded in
the design's [shared owner-navigation contract](../../specs/agents/system-design/workspace-mcp-management.md#shared-owner-navigation-contract).
Task 02 owns route validation and `#mcp` section entry. Assignment owns the
saved-definition receipt, repeatable Use in action, owner choice, and recovery.
Route entry is read-only and never selects or saves a definition. Session entry
opens the exact conversation; the user activates its existing plug/Customize.
No automatic assignment or second management hub is introduced.
The same contract defines the typed `MCPOwnerDestination` input. Definition
highlighting remains assignment-owned through an optional focus-only
`mcpServerId` query. Management task 02 does not implement that highlighting.

## Technical approach

1. Extend `mcpconfig.SelectionService` with a separate safe usage read and typed
   owner projection. Add settings-store enumeration and backendapp owner wiring.
   Add the catalog `/usage` handler and typed frontend client. Read existing
   association tables only. Preserve delete impact and resolver semantics.
2. Extend `MCPSettings` with Used in detail and validated query entry. Centralize
   destination building. Route global saved profiles into
   `ProfileMCPSelectionCard`. Use `requestNavigation` and
   `SettingsSaveProvider` for workspace switching. Expose only safe import
   readiness while retaining explicit authorized legacy recovery.
3. Correct configured-only fallback in `buildMcpExplorerServers` and unify
   neutral delivery presentation across all evidence surfaces. Amend existing
   Platform wording and public how-to sections in the same implementation.

| Provider/path | Evidence identity | Intended behavior | Evidence/fallback |
| --- | --- | --- | --- |
| ACP accepted setup | Current session/execution/attachment | Delivered stays neutral until stronger evidence | Existing history fixtures; absent report remains unknown |
| Passthrough process start | Existing attachment history | Delivery does not imply tools or connection | Existing passthrough fixtures; no new producer |
| Kandev tools observed | Current attachment tools evidence | Preserve Active and known tool counts | Existing status tests; old attempts never promote current |
| Third-party configured name only | No server-specific delivery evidence | Unknown/unobserved, including beside observed servers | View-model regression; no inferred delivery |
| Unsupported/filter/failure | Existing typed reason | Preserve explicit reason and failure semantics | Mixed-state UI fixtures; no new probes |
| Saved session selection | Desired/applied selection state | Existing pending/next-start/failure markers remain independent | Existing selection tests and simultaneous neutral row |

## UI preview

The following structural previews use control-order descriptions instead of
pixel mockups. The task's rich-output instruction takes precedence over the
skill's ASCII format. Copy is illustrative and must be localized. The hierarchy,
scope, save ownership, and one-scroller rules are requirements.

### UI-01: Configured server, Used in

Desktop entry: Settings, workspace, MCP servers, configured server card, Used in.
The card retains metadata and Edit/Disable/Delete. Used in opens a bounded
dialog: fixed title and close, scope-grouped owner list, then load-more/retry.
Each authorized row shows owner name, scope, and a navigation chevron. An
unavailable destination has no chevron. No assignment checkboxes appear.

Phone: Used in opens one full-height detail surface. Back and server name stay
fixed. One vertical body contains scope headings, full-width named owner rows,
and load-more/retry. Activating an owner navigates to its existing page and
closes usage. Back returns to the same catalog context.

States: loading skeleton with stable heading; successful empty says no visible
saved usage; error says usage could not load with Retry; partial page retains
rows and Load more without a false total; failed refresh marks retained data
stale. Deleted definition shows unavailable recovery and Back to configured.

Criteria: AC-001.1 through .4, AC-004.1 through .3 under this package's prefix.

### UI-02: Global profile workspace picker

Desktop: profile settings, MCP servers heading, visible Workspace selector,
scope/future-session sentence, catalog choices, Manage workspace MCP servers
link. Existing shared Save changes/Discard stays in its current location.

Phone: same settings page and shared draft owner. Workspace opens the existing
touch picker. Catalog choices remain in the shared responsive selection surface.
Keep the primary save control above the safe area. Do not stack workspace and
catalog drawers. A context change with dirty settings opens the existing guard:
Save changes and continue, Discard and continue, or Continue editing.

No workspace: show Choose workspace and no choices. A denied/removed workspace
clears its data. A failed save keeps the workspace and draft. Pending/unknown
legacy import shows a fallback explanation with authorized recovery, separate
from the workspace picker. A disabled profile retains its existing read/edit
permission rules and is never enabled by navigation.

Criteria: AC-002.1 through .5 and AC-004.1 through .3.

### UI-03: Connection evidence and saved changes

Desktop hover/focus summary: Saved changes and its existing marker first when
applicable, then Current agent with each server's independent evidence.
Connections list/detail retain server navigation and tool inspection.

Example: Saved changes says Waiting to apply with its clock. A server row says
Delivered, connection unverified in neutral styling. Another row can say Active
with its known tool count. A configured-only row says connection unobserved.

Phone: existing composer plug drawer, fixed centered title and view selector,
one body scroller. The same labels and reasons appear without hover. No extra
conversation bar or Apply button. Application success removes only its marker.

Criteria: AC-003.1 through .3 and AC-004.2/.3. Platform AC-001.14 through .17
remain regression requirements, with neutral delivery wording reconciled.

## Tests

New test names below are planned, not existing or executed evidence.

| Criteria | Planned evidence |
| --- | --- |
| AC-001.1/.3/.4 | `catalog_usage_test.go`: `TestCatalogUsageAuthorizedOwners`, `TestCatalogUsagePaginationAndErrors`; store and handler counterparts |
| AC-001.2/.3 | `management-links.test.ts`: `routes only authorized typed owners`; backendapp owner-projection tests |
| AC-002.1/.5 | `profile-mcp-config-card.test.tsx`: saved global/workspace/draft capability routing; safe import-status handler tests; legacy resolver regressions |
| AC-002.2/.3/.4 | `profile-mcp-selection-card.test.tsx`: `guards workspace switch`, `preserves draft after failed save`, `ignores obsolete context`, `retains edits during save` |
| AC-003.1/.2/.3 | `mcp-explorer-view-model.test.ts`: configured-only beside observed; `mcp-status-presentation.test.tsx`: neutral delivery and independent pending marker |
| AC-004.1/.2/.3 | Browser scenarios below plus existing settings navigation/focus tests and locale gates |

## E2E tests

Task 02 adds `e2e/tests/settings/mcp-management.spec.ts` and
`mobile-mcp-management.spec.ts`. Cover names and direct usage, global profile
workspace A/B isolation, dirty save/discard/stay, partial failure, denied/stale
targets, deep links, reload, long lists, and keyboard/focus return. Assert the
phone surface, scroll owner, rendered hitboxes, and no horizontal overflow.
Use 320px, 393px, 767px fine pointer, 768px desktop, and 900px coarse pointer.

Task 03 extends `e2e/tests/chat/mcp-status.spec.ts` and
`mobile-mcp-status.spec.ts` for neutral delivery beside pending/next-start/failed
application, configured-only mixed evidence, and stronger observed states.
Run the existing connection geometry and status clarity scenarios in those
files. Fixtures prove presentation, not live provider connectivity.

Use the existing `test-base` isolated backend and managed `e2e:run` builds.
Do not overlap suites or use the developer runtime. Exact commands belong to
each work order. Install dependencies once from `apps/` before package commands.

## Work orders

- [x] [Task 01: Authorized usage projection](task-01-usage-projection.md)
- [x] [Task 02: Management navigation](task-02-management-navigation.md), after 01
- [x] [Task 03: Neutral evidence](task-03-neutral-evidence.md), after 02

## Verification results

Design validation on 2026-10-22:

- `python3 scripts/list-docs.py validate`: passed, 397 decisions and 1693 specs.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Catalog query lists both new Agents specification files.
- `.github/scripts/pr-docs.cjs::validateCoverage`: the actual docs-only diff is
  exempt. A second preflight with a synthetic `mcp-settings.tsx` change returned
  `covered` for all three work orders with zero errors. No source file changed.
- All six new files passed local Markdown-target and whitespace checks.
  `git diff --check -- docs/specs docs/plans/mcp-management-clarity` passed.

Production, permanent tests, browser runs, and runtime validation: not started.
At the original handoff, all six package files were unstaged and uncommitted.
The 2026-10-24 publication checkpoint authorizes their documentation commit and
draft PR. Public docs updates remain assigned to implementation because the
proposed behavior has not shipped.

Draft-publication validation on 2026-10-24, after reconciliation onto main:

- `python3 scripts/list-docs.py validate`: passed, 403 decisions and 1734 specs.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- Local Markdown targets and whitespace: all six files passed.
- `.github/scripts/pr-docs.cjs::validateCoverage`: actual six-file diff is
  documentation-only and exempt, with zero errors.
- No production, runtime, browser, or live-provider tests ran. No implemented UI
  changed, so UI screenshot evidence does not apply to this draft checkpoint.

## Risks

- Owner data spans settings and task services. Keep access checks explicit and
  do not expose raw impact totals as authorized usage totals.
- A global profile has two different scopes: workspace selections and legacy
  global configuration. Keep contributor, permission, and fallback boundaries.
- New deep links require route-level validation. Never substitute another owner
  after an invalid session or workspace hint.
- The sibling may edit shared MCP files later. Re-read its latest package before
  implementation and reconcile interfaces without changing its workspace.
- Platform's current amber delivery rule must be amended with the UI. Leaving
  both rules authoritative would create contradictory guidance.

No material product decision remains open in this draft. The proposed query
entry is a local interface choice communicated to the coordinator, not an
assertion that the destination already exists.

## Implementation progress, 2026-10-25

The user requested implementation after the published design handoff. Task 01
and task 02 are complete; their work orders record RED/GREEN and final affected
checks. Task 03 is also complete with 45 focused unit regressions, 3 desktop/
tablet and 4 phone explorer scenarios, and light/dark/pseudo capture inspection.
Public docs and Platform evidence requirements/design are reconciled. The
published PR remains draft;
no review handling or merge is authorized. The design-only statements above
record the earlier publication checkpoint, not the current local worktree.


Final implementation validation, 2026-10-25: all three work orders passed their
recorded affected checks. The shared route/global-profile prerequisite was
published at `fea04b3413acf020d63e218af724889d5aa82aaa` and communicated to the
assignment sibling. Neutral evidence is the final additive commit. No selection
assignment, session exclusion, built-in management, runtime reconnect, or new
probe was introduced. PR #4759 targets main and remains draft. Screenshots use
synthetic data. No CI waiting, review handling, or merge was performed.


Integration correction, 2026-10-25: task 02 reopened for workspace-profile
admission after the assignment sibling identified that the global profile
inventory excludes those rows. The earlier completion report did not cover
this case. The focused correction preserves the agreed route and ownership,
adds authorized metadata reads, and retests both workspace and global profiles.
Tasks 01 and 03 remain complete. Publication remains draft-only.

Correction completed, 2026-10-25: authorized canonical profile metadata now
admits workspace-owned profiles through the existing owner route without
changing the global inventory. Task 02 is complete again. Its correction
receipt records the additional SQLite/Postgres, frontend, desktop and phone
checks. Assignment can consume the published metadata and navigation contract.
