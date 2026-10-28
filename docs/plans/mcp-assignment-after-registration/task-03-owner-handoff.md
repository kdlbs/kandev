---
id: "03-owner-handoff"
title: "Connect optional MCP assignment to existing owner editors"
status: completed
wave: 3
depends_on:
  - "02-registration-completion"
plan: "plan.md"
requirements:
  - REQ-AGENTS-MCP-ASSIGNMENT-001
  - REQ-AGENTS-MCP-ASSIGNMENT-002
  - REQ-AGENTS-MCP-ASSIGNMENT-003
  - REQ-AGENTS-MCP-ASSIGNMENT-004
acceptance_criteria:
  - AC-AGENTS-MCP-ASSIGNMENT-001.1
  - AC-AGENTS-MCP-ASSIGNMENT-001.2
  - AC-AGENTS-MCP-ASSIGNMENT-001.3
  - AC-AGENTS-MCP-ASSIGNMENT-001.4
  - AC-AGENTS-MCP-ASSIGNMENT-002.1
  - AC-AGENTS-MCP-ASSIGNMENT-002.2
  - AC-AGENTS-MCP-ASSIGNMENT-002.3
  - AC-AGENTS-MCP-ASSIGNMENT-002.4
  - AC-AGENTS-MCP-ASSIGNMENT-002.5
  - AC-AGENTS-MCP-ASSIGNMENT-002.6
  - AC-AGENTS-MCP-ASSIGNMENT-003.1
  - AC-AGENTS-MCP-ASSIGNMENT-003.2
  - AC-AGENTS-MCP-ASSIGNMENT-003.3
  - AC-AGENTS-MCP-ASSIGNMENT-003.4
  - AC-AGENTS-MCP-ASSIGNMENT-004.1
  - AC-AGENTS-MCP-ASSIGNMENT-004.2
  - AC-AGENTS-MCP-ASSIGNMENT-004.3
  - AC-AGENTS-MCP-ASSIGNMENT-004.4
system_design:
  - ../../specs/agents/system-design/workspace-mcp-assignment.md
---

# Task 03: Optional owner handoff

## Summary

Complete Use in on the registration receipt and existing catalog cards. Discover
authorized owners and hand off to existing editors through the management
sibling's committed navigation contract. Prove recovery and publish usage docs.

## In scope

- Read-only owner choice, scope consequences, existing-definition highlighting.
- Repository, workspace/global profile with explicit workspace, and exact
  existing task agent session; no default or implicit selection.
- Failure/eligibility/uncertain-save recovery and context generation guards.
- Desktop and phone composition, locales, focused E2E, and public docs.

## Out of scope

Management hub/Used in/global-profile implementation, session inherited
exclusions, built-in management, task-level or bulk assignment, owner creation,
endpoint probes, and runtime reconnection changes.

## Acceptance

1. All three owner paths use existing editors and their save contracts. Mere
   discovery/navigation changes no selection and starts no agent. Global
   profiles retain the explicit registering workspace; session links target
   the named existing sibling, not a default or newly ensured session.
2. Failed/uncertain assignment, conflict, disabled/removed targets, and access
   loss retain the saved definition and offer actionable recovery. Retry never
   calls create. Existing direct/inherited choices remain intact and no stale
   result crosses contexts. Provider timing and prior accepted setup remain intact.
3. Phone and desktop pass the plan's scenario matrix with measured geometry,
   keyboard/focus/resize and localized wrapping. Public docs describe actual
   shipped behavior, saved-only completion and partial-success recovery.

## UI preview

UI-01 adds Use in beside Done on the saved receipt and to catalog cards.
UI-02 and UI-03 from [full previews](plan.md#ui-02-owner-choice-and-consequence-review):

| Region | Desktop dialog | Phone Drawer |
| --- | --- | --- |
| Header | Use Atlas docs in / Acme / Close | Fixed, contained close |
| Body | Owner type, then search/results, then consequences | One focused view per step; one scroll owner |
| Footer | Back / Open MCP settings | Same controls with safe-area and 44px targets |
| Failure | Saved remains visible; retry read or choose another owner | Same recovery without nested overlays |

Settings reveal the MCP section and can highlight the available definition
without changing its existing checkbox state or search. Session navigation opens
only the conversation; its hint waits until the user opens plug/Customize.
Existing settings Save changes or explicit session selection autosave owns commit. Maps to every criterion listed in this work order.

## Verification

Run TDD before implementation. From root, after work orders 01/02 and the
committed sibling prerequisite are integrated:

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/workspace/use-mcp-registration-completion.test.ts hooks/domains/workspace/use-mcp-assignment-targets.test.ts lib/mcp/assignment-hint.test.ts hooks/domains/workspace/use-mcp-selection-editor.test.ts hooks/domains/workspace/use-mcp-selection-editor-context.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/mcp-assignment.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-mcp-assignment.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run targeted ESLint with all actual changed TS/TSX paths and record the expanded
command. Do not add a broad lint/test audit. Run E2E sequentially, with managed
fresh builds; inspect intended project discovery and one phone render. Tests
must assert persisted membership, no duplicate definition, and forbidden-request
absence, not only button visibility. Use disposable fixtures and mock providers.
No real-provider certification is claimed by these tests.

## Files likely touched

- `apps/web/components/settings/workspaces/mcp-settings.tsx` and completion component.
- New `mcp-assignment-chooser.tsx` and domain hook
  `hooks/domains/workspace/use-mcp-assignment-targets.ts`, with focused tests.
- `apps/web/lib/mcp/assignment-hint.test.ts` against the
  sibling's committed destination builder. Modify the builder only by the
  reviewed integration contract, not by copying a competing implementation.
- Existing repository/profile destination consumers and composer Customize
  focusing hook as needed; exact files follow the committed sibling contract.
- `apps/web/e2e/helpers/mcp-assignment.ts`, desktop/phone assignment specs and
  affected locale catalogs.
- `docs/public/agents-and-profiles.md`, `docs/public/automation-and-mcp.md`;
  the owning requirements/design and this work package for actual results.

## Dependencies

Work orders 01 and 02. Also management task
`000d477e-f792-4c9d-8503-50ed73bb9646`, planned
`docs/plans/mcp-management-clarity/task-02-management-navigation.md`.
Before coding, read its reviewed committed requirements/design/work order,
record the integrated SHA and actual route/owner-link symbols here, and reconcile
this package. If it is unavailable, stop this work order with the concrete
dependency. Do not implement the sibling feature or silently omit global profiles.

The management draft records `MCPOwnerDestination` and
`apps/web/lib/mcp/management-links.ts`. Consume its exact routes and required
identities from the [shared integration contract](../../specs/agents/system-design/workspace-mcp-assignment.md#management-sibling-integration).
Management owns base routes, validation, MCP section reveal and global-profile
context. This work order extends the builder with optional `definitionId` and
reserved `mcpServerId` through `URLSearchParams`, preserving query and hash.
Implement the focus-only parser and destination lifecycle here. It must validate
same-workspace availability in existing inventory, reject duplicate/malformed
hints, leave search and drafts unchanged, consume each hint once, and wait for
explicit user opening of session Customize. It must never open an overlay or
broaden inventory. Add parser/lifecycle tests plus desktop/phone coverage for
unknown, removed, disabled, cross-workspace and filtered-out definitions, delayed
loading, context changes, existing selected state, reopen, rerender and reload.
The catalog's `serverId`/`view=usage` route is optional recovery, not an assignment
destination. No definition hint is a selection command.

When exclusions is integrated, test that addition-only writes omit exclusions,
preserve an existing excluded ID, and use the same session revision. Do not
claim an excluded server is effectively included or restore it automatically.

## Risks

Two independent owner-link builders, cross-workspace global-profile defaults,
opening a removed session and triggering ensure, owner-list overfetch,
save-contributor draft replacement, locale conflicts, and catalog receipt
being mistaken for runtime success.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/workspace-mcp-assignment.md)
- [Owner handoff and sibling boundary](../../specs/agents/system-design/workspace-mcp-assignment.md#owner-handoff)
- [Parent testing guide](../workspace-mcp-configuration/testing-guide.md),
  interpreted as historical scope/evidence only.
- Existing picker, settings-save and composer E2E patterns; `/tdd`, `/e2e`,
  `/mobile-parity`, `better-ui`, and `/docs-maintainer`.

## Results

Incomplete. On 2026-10-23, management's plan remained at
`40b577c3-0d95-43b5-9a57-ae8af8ee149b` (2026-10-22 11:09), with design only
and no committed navigation prerequisite. Work orders 01 and 02 are complete.
No owner-handoff UI has been added. Resume integration after the committed
management task 02 result is published and reconcile its exact typed contract.

On 2026-10-25, the coordinator authorized independent preparation while management
task 02 is in progress. The standalone `lib/mcp/assignment-hint.ts` helper now
parses, appends, removes, and resolves the focus-only hint without constructing
owner routes or changing UI state. Its 15 tests passed after an initial RED for
the absent module; focused ESLint passed. Cases cover duplicate/malformed hints,
query/hash preservation, wrong-workspace/disabled/removed/filtered-out rows,
inventory readiness, and explicit session surface opening. Destination lifecycle
and rendered behavior remain to be integrated and tested; no completion is claimed.

Prepared bounded task discovery in `use-mcp-assignment-tasks.ts`: explicit pages
of 25, no read before task choice is active, workspace and eligibility filtering,
generic failure and read-only retry, and request-generation cancellation on
query/context changes, close, and unmount. All six focused tests passed after RED;
focused ESLint passed. The hook is not yet mounted in the chooser.

Profile discovery investigation found that `/agents` intentionally omits
workspace-owned profiles. Despite its frontend `listAgentProfiles` name, the
Office endpoint returns AgentInstance records, so it is not an authoritative
settings-profile inventory. This correction was relayed to management for its
destination admission contract. Await the committed authorized profile read
contract; do not infer profile identity or enabled state from Office instances.
No broader inventory or new endpoint was introduced. Requested the exclusions
sibling's minimal committed saved-ID comparison correction for independent
delivery, without adopting its full branch.

Before the next dependency rebase, preserve the current published assignment tip
`6faaa61b817dc9dddc60d2c12026cc1273748a13`, all local preparation, and old management
base `f7bbe6076477eebace8740a42adb206e7229e3e4`. Consume only an exact verified
published management prerequisite. Keep PR #4760 draft; no CI waiting or review
handling is authorized.


### Implementation checkpoint, 2026-10-25

The historical dependency blockers above are resolved by published management
`45e90466378f982e13cc762c52432f14ab43c32c`. The chooser consumes
`mcpOwnerDestinationHref` and `listMCPProfileOwners`; it does not build another
management surface. Global and exact-workspace profiles are filtered by enabled,
concrete and MCP-supported metadata. Task discovery stays at 25 rows per page;
sessions are read only after one task is selected. Every open revalidates the
owner and enabled definition. Failed reads preserve the saved catalog identity.

Desktop uses a bounded dialog. Phone uses an inset bottom drawer with a fixed
header/footer, one scrolling body, safe-area padding, and 44px or larger touch
actions. Domain state is above responsive wrappers and survives resize. Both
registration receipts and cards offer Use in; Done is still complete registration.
Scope review precedes navigation. Settings retain shared Save changes; session
navigation does not open Customize and explicit checkbox changes keep autosave.

Focus-only mcpServerId processing is scoped to the validated owner and existing
visible picker inventory. It waits for readiness and explicit session opening,
consumes duplicate/invalid/disabled/foreign/filtered hints without a fallback,
and leaves search, checkbox and save state unchanged. StrictMode replay is
covered and cannot consume the same hint twice. Backend addition admission also
rejects terminal/passthrough sessions and missing, disabled, foreign or
non-MCP profiles; existing removal and same-set application retry paths remain.

RED/GREEN evidence includes missing Use in, authorized profile metadata and
cursor-loop rejection, StrictMode consumption, and real session admission.
Focused frontend validation passes 71 tests across 11 files. Final browser
and integration results follow.


### Final validation, 2026-10-25

Completed against management `45e90466378f982e13cc762c52432f14ab43c32c`.
No missing owner path or unimplemented management prerequisite remains.

- Focused frontend: 11 files / 71 tests passed, including discovery cancellation,
  metadata pagination, hint parsing/StrictMode, manual save and conditional writes.
  Log: `/tmp/assignment-wo03-final-unit.log`. Final focus refactor: eight tests
  passed in `/tmp/assignment-focus-final.log`.
- Session domain plus actual `components/task/chat/chat-input-container.test.tsx`:
  108 files / 1059 tests passed (`/tmp/assignment-wo03-session-consumers.log`).
- Full `pnpm run typecheck`, targeted changed-file ESLint and Go lint passed.
  Logs: `/tmp/assignment-wo03-final-typecheck.log`,
  `/tmp/assignment-wo03-refactor-eslint.log`,
  `/tmp/assignment-wo03-final-go-lint.log`.
- `go test -race` with `MCP|Selection|Effective|Usage|ImportStatus` passed in
  `internal/agent/mcpconfig`, `internal/agent/settings/handlers`,
  `internal/backendapp`, and `internal/agent/settings/store`. Store verification
  included actual disposable PostgreSQL 17 as well as SQLite. Logs:
  `/tmp/assignment-wo03-final-go.log`, `/tmp/assignment-wo03-final-store.log`.
- Managed E2E, one host shard, zero retries: eight assignment and four management
  integration journeys pass on Chromium; the same 12 journeys pass on
  mobile-chrome. The fresh-build desktop run passed 11 cases
  (`/tmp/kandev-run.e2e.zjibAhGh.log`); the corrected repository assertion passed
  separately (`/tmp/kandev-run.e2e.4A2ZMTZ7.log`). The unchanged build then passed
  all 12 phone cases (`/tmp/kandev-run.e2e.x1kaIXJ2.log`).
  The repository test now waits for the shared Save bar to settle and asserts
  exactly one MCP PUT while allowing the existing optional repository PATCH.
  Registration tests distinguish the footer Close from the dialog Close icon.
  The management rejection test explicitly Reloads choices before retrying,
  matching the existing conditional-write draft interlock.
- `pnpm run i18n:check`, spec catalog validation and spec lint passed. Public docs
  validator tests passed (63), public page validation passed (54 pages), and
  actual-diff documentation coverage returned covered. `git diff --check` passed.
- Fresh desktop/phone assets are in ignored `apps/web/.pr-assets`; inspected
  receipt, review, repository selection draft and session Customize captures.
  The receipt keeps Done, both scopes explain effects before navigation, and
  session Customize highlights an unchecked row without opening itself.

The browser suite uses disposable workspaces and mock agents. No live provider
certification, new agent creation by assignment, endpoint connection during
registration/discovery, or sibling-owned exclusions behavior is claimed.


Additional breakpoint regression passed on Chromium and mobile-chrome, one test
per project with zero retries (`/tmp/kandev-run.e2e.DRpGU2fU.log`,
`/tmp/kandev-run.e2e.t7p7H6Rk.log`). The global-profile review preserves its owner
through 320, 390, 767, 768 and 900px widths and back to the original viewport.
Measured primary hit areas are at least 44px on phone layouts and coarse
pointers, stay inside the viewport, and do not introduce horizontal overflow.
This includes a narrow fine-pointer desktop window and a coarse-pointer tablet.
The production build is unchanged from the final fresh-build desktop suite.


The first normal commit attempt rejected the session hook's complexity of 16
(limit 15). Reusing the same committed selection state and saved IDs removes the
repeated expressions without changing UI or retry behavior. Strict changed-file
ESLint then passed; the session selection hook and real chat consumer passed all
17 focused tests (`/tmp/assignment-wo03-session-hook-final.log`). Existing captures
remain valid because this refactor changes no rendered behavior. No hook bypass
was used. The historical verification command now names the actual hint test.


### Follow-up: inline profile composition, 2026-10-25

Integrated verified management `7aca966a6ec262eb1ce839fa883f66e23441d477` by
rebasing only assignment commits from old base `45e904663`. Backup
`backup/clarify-mcp-assignment-before-inline-profile-20261025` preserves old
published assignment `fe08307e9f4701715e9fc906b1d65916c18971e1` for exclusions.
Management owns the inline searchable profile list and removal of its redundant
opener/description/catalog link. Assignment passes the focus hint directly to
MCPSelectionContent; the old automatic opener prop is removed. Phone tests no
longer dismiss a picker sheet before using the shared Save changes action.

The first integrated browser run exposed a real readiness regression: both
assignment profile paths lost the highlight because loading starts in an effect
and the inline content could consume the hint before an accepted selection
existed (`/tmp/kandev-run.e2e.VOLemtu9.log`: two failures, two management passes).
The focus readiness guard now requires `editor.selection` as well as settled
loading and no load error. This leaves the hint pending until the owner snapshot
is ready. Existing E2E tests are the primary integration regression; no new
production test seam is needed.

After the guard: 36 focused tests across assignment focus/discovery/hint and
settings-draft suites pass; strict changed-file ESLint and full web typecheck
pass. Four affected Chromium profile journeys pass with zero retries
(`/tmp/kandev-run.e2e.rrFe5buX.log`), including global/workspace-owned assignment,
manual save/workspace switching, and authorized cold profile admission. Browser
captures now cover the unchecked highlighted row directly in the inline editor.
Backend and non-profile runtime behavior are unchanged from the prior verified
head; no duplicate broad backend or browser suite is required for this UI-only
integration. Spec catalog/lint and public docs checks pass.


Final phone validation passed the same four profile journeys, zero retries
(`/tmp/kandev-run.e2e.qFEMrqEz.log`), using the unchanged freshly built desktop
source. Global/workspace profile screenshots were captured on both viewports.
The remaining assignment behavior and prior screenshots are unchanged. Full web
typecheck and strict lint passed after the readiness correction; no temporary
instrumentation or hook bypass is present. Current rebased WO01 is
`778c9ebabeb039c60530c5935704e2dddf6892ab`.


### Management squash integration, 2026-10-26

Verified management PR #4759 merged at `e5c38184cf9c191075dc88c1ffad28c636858bf5`
and fetched that current main. Replayed only nine assignment commits after
integrated management `7aca966a6`; backup
`backup/clarify-mcp-assignment-before-management-squash-20261026` preserves old
assignment `f3be6a5186f43dcce4487371c7a493be6c9ec280` for exclusions. PR #4760
now targets main and remains draft. Current WO01 is
`84f880ae45c143b3bda7349d47a9b46907937443`.

Preserved the merged layout-effect update of draft/context refs, profile retry
admission, deferred global legacy editor, usage retry handling and exact-session
route guards. The only rebase conflict moved management's new transport test
into `mcp-usage-api.test.ts`, alongside its existing mocked-fetch tests, while
retaining assignment's real-fetch conditional serialization tests separately.
Range-diff shows the other eight assignment patches unchanged.

The first focused test run passed 82 of 83 cases. Its one failure was the newly
merged suspended-render test expecting the old four-argument write call. The
assertion now also requires `expected_definition_ids: ["saved"]`; this preserves
the test's committed-versus-suspended-draft contract and proves the conditional
baseline is retained. All four draft-suite tests then passed. Combined focused
coverage is 83 tests across 11 files, including owner hints, metadata, usage,
conditional writes, registration recovery and actual task-route admission.
Logs: `/tmp/assignment-squash-unit.log`, `/tmp/assignment-squash-draft-final.log`.

Full web typecheck and strict affected ESLint pass. Race-enabled
`MCP|Selection|Effective|Usage|ImportStatus` checks pass in MCP config, settings
handlers and backend composition (`/tmp/assignment-squash-go.log`); backendapp
completed in 137.142s. Storage/conditional-operation source is byte-identical to
the prior verified tip, so existing SQLite/PostgreSQL evidence remains applicable.
Spec catalog validation and spec lint pass. Fresh-build desktop E2E passes all
13 assignment/management journeys with zero retries
(`/tmp/kandev-run.e2e.IJfS4gTV.log`), including the new exact-session admission case.


The unchanged build also passed all 13 phone assignment/management journeys with
zero retries (`/tmp/kandev-run.e2e.vr4VlPJ4.log`). Fresh desktop and phone captures
replace the prior PR evidence. Public docs validation passed (54 pages), catalog
validation passed (403 decisions / 1742 specifications), and diff whitespace is
clean. Only the test assertion and integration records changed after the rebase;
no additional production changes were needed. Normal commit hooks and exact-OID
publication are required before reporting this head to exclusions.

### Local verification for the remediation

- Frontend: 76 tests across eight dialog/assignment suites, plus 30 tests across
  four shared editor/draft/session suites passed. The suspended-render regression
  failed with a null accepted result before the layout-effect fix and passed
  afterward. Full typecheck and affected ESLint passed.
- Backend: the real settings-store/session-service regression rejected the
  internal launch write before the fix. Afterward the MCP/Selection/Effective/
  Usage/ImportStatus race command passed in mcpconfig, settings handlers and
  backendapp. The same regression preserves HTTP rejection, workspace isolation,
  disabled-definition rejection and desired-state advancement. Changed-package
  Go lint passed with zero issues.
- Browser: 14 desktop journeys and 21 phone journeys passed, retries disabled.
  These include the new task-edit recovery, delayed destination check, exact
  session review and all three reported failing E2E scopes. The final binary's
  phone assignment/recovery subset is rerun before publication. Fresh synthetic
  images replace affected evidence. No real endpoint connection was tested.
- Catalog/spec/public-doc checks passed. Incoming main's harness change also
  passed harness and spec validator tests/lint. Normal commit hooks and exact-head
  remote CI/review recheck remain delivery steps, not completed local evidence.

Commands and logs: focused Vitest results are recorded in
`/tmp/pr4760-fixup-unit-final.log` and `/tmp/pr4760-fixup-guards-final.log`;
backend race in `/tmp/pr4760-review-backend-green.log`; browser runs used managed
host mode, one shard and sequential projects (`/tmp/kandev-run.e2e.YETxqUNT.log`,
`/tmp/kandev-run.e2e.RhOn7ji6.log`). Authoritative base for this remediation is
main `b4e9cf5ede3545ca6a8e58165e580ad7c4988afa`.
