---
created: 2026-10-28
status: implemented
requirements:
  - REQ-EXECUTORS-SSH-CONNECTIONS-001
  - REQ-EXECUTORS-SSH-CONNECTIONS-004
system_design:
  - ../../specs/executors/system-design/ssh-connection-management-ui.md
legacy_specs: []
---

# Implementation plan: SSH discovery and verified status

## Overview

Make the SSH entry, collection controls, and successful verification visible in
the normal reading path. Deliver one bounded UI slice from Executors through
the manager. Executors owns reusable SSH access; this is a revision to its
existing requirements and interaction design, not a new UI system or ADR.

Confirmed: the user rejects both far-right control clusters and explicitly
requests green for SSH verified. Verified: `SSHConnectionsEntry` uses the hub's
trailing header action, and `ManagerHeader` puts both tabs and actions in the
shared header's right-aligned rail. Both badge call sites use neutral outline.
Baseline: `106018b68ca687f44d014090b25f4cff829a208b`.

Implemented composition: full-width navigation card before Profiles; a manager
content toolbar below the title with adjacent tabs/create action; green only
for current ready status. Existing ownership, permissions and readiness remain
settled. No material question blocks planning. The user authorized implementation with the subsequent “go for it” request.
The work order records the implementation and its tests.

## Scope

### In scope

- One full-width SSH card before the hub's profile sections, including empty hub.
- Leading-aligned manager tabs and creation action; intentional phone layout.
- Green ready badge in connection rows and saved detail headers.
- Focus, keyboard, route, shared-save, localization and permission parity.
- Focused desktop/phone/state tests and public navigation wording at implementation.

### Out of scope

New settings sections, global component/style changes, larger ordinary desktop
buttons, authentication/readiness changes, remote enrollment, SSH APIs/storage,
executor/task behavior, feature defaults, CI/review work, committing/pushing or
refreshing the running lab. Existing implementation records stay completed.

## Technical approach

Change `SSHConnectionsEntry` in `app/settings/executors/page.tsx` to a full-card
Link using existing title/description translations, icon, chevron and settings
surface tokens. Move it from the shared header actions into normal content.
Use the full-row navigation precedent in `connection-collection.tsx` and the
whole-body destination behavior in `plugins/plugin-row.tsx`. No nested button.

In `ssh-connections/manager-header.tsx`, keep the title component and render
the tabs/local actions as a scoped content toolbar. Desktop collection tabs and
New are adjacent at the leading edge. Phone New gets its own full-width row.
Detail tabs move below the title; existing edit/menu/cancel/save semantics stay.
Do not change global `settings-typography.tsx` or `settings-tabs.tsx`.

Add a small local `status-badge.tsx` with `ConnectionStatusBadge`; reuse it at
both existing badge sites. Consume `connectionState`, not raw historical
`verified_route`, and keep labels localized. Success styling uses the existing
green semantic palette with a readable foreground. Verify contrast in each
theme; no global token changes. Keep ordinary desktop controls 28px (1px
tolerance) and phone/coarse controls at least 44px.

### Compatibility

| Shape | Intended behavior | Evidence | Failure behavior |
| --- | --- | --- | --- |
| Admin, flag enabled | Visible navigation and local creation | Hub unit and desktop/phone E2E | Busy state disables actions |
| Member or disabled | Existing management gates | Hub unit, auth/disabled E2E | No entry or hook mount |
| Existing direct/bastion records | Same URLs and readiness | Existing presentation/state suite | Stale/failed/uncertain never green |
| Empty/loading/read error | Toolbar remains usable | Layout E2E fixtures | No false verified or empty state |
| Identity/adoption/executor return | Existing query and save owners | Existing contextual setup E2E | No automatic save/probe |

## UI previews

Structured previews follow the task's rich-output/no-ASCII requirement.
UI-01 and UI-02 specify leading alignment, content order and one primary action;
spacing and copy shown here are illustrative, using existing localized strings.

| View | Desktop | Phone |
| --- | --- | --- |
| UI-01: Executors populated/empty | Title and description; full-width SSH connections card with key icon, explanation and chevron; then Profiles/Create profile | Same content order; wrapping explanation; entire card is a touch destination |
| UI-02: Connection collection | Title/description; leading toolbar: Connections, Identities, New connection; full-width resource list | Title/description; tabs row; full-width New connection row; resource list |
| UI-02: Identity collection | Same toolbar with New identity | Same explicit action row with New identity |
| UI-02: Loading/error/empty | Same toolbar; content-owned status/retry/empty message | Same bounded rows and status; no trailing action rail |
| UI-03: Saved ready detail | Resource title; green SSH verified text badge; leading section tabs; existing overview/actions | Same hierarchy; wrapping status and touch-sized tabs/actions |
| UI-03: Stale/failed/uncertain | Existing state text, without green success | Same state and warning visibility |

No fixed toolbar, extra scroll owner, sticky footer or new overlay is needed.
The page uses the existing settings scroller and safe-area padding. Phone direct
navigation follows Kandev's one-focused-destination pattern. Existing collection
rows are the nearest shipped phone exemplar; plugin rows supply whole-card taps.
Shared state and hooks remain identical across widths. New sits in page content
rather than a floating action that can cover a connection row.

## Tests and E2E

- AC-004.8/004.13 and AC-001.5: hub mounting/gates plus UI-01/02 geometry.
- AC-004.14: badge component and existing presentation tests cover ready,
  outdated stamp, failed test and uncertain enrollment. Browser light/dark
  checks measure computed success styling and contrast at both badge sites.
- AC-004.11/004.12: existing navigation/context/access helpers prove explicit
  actions, focus, no incidental probes, hit areas and no horizontal overflow.

Extend `ssh-management-layout` and `ssh-navigation` helpers and their desktop/
phone specs. Replace old assertions that require hub entry beside the title;
do not weaken checks to visibility alone. Assert card spans the content width
and precedes Profiles, tabs sit below title, New is adjacent to tabs on desktop
and spans its own row on phone. Cover 390px, 767/768px, 820px coarse tablet and
1280px, long/pseudo labels, light/dark, empty and failed read. Update every
consumer of affected helpers and rerun it. Capture and inspect both widths.

## Work orders

- [x] [Task 01: Visible SSH entry, controls and status](task-01-visible-controls.md)

## Verification results

Design-only validation passed on 2026-10-28: documentation catalog (393
decisions and 1674 specifications), all 36 specification-validator tests, full
specification lint, package relative links, tracked/untracked whitespace checks,
and local delivery-coverage preflight for the single work order. The preflight
included the prospective manager-header change to activate cross-reference
validation; it made no GitHub request. Receipts:
`/tmp/ssh-discovery-{catalog,spec-tests,spec-lint,diff}.log` and
`/tmp/ssh-discovery-delivery-preflight.json`.

At the design checkpoint no product code, permanent tests, public future-flow
docs or runtime changed. The work-order results below supersede that historical
phase status. The existing lab still serves `106018b68ca6`.

### Implementation results

Task 01 is done. Requirements are active and the interaction design is current.
All implementation stayed in the primary session with no delegation/model switch.

| Check | Result |
| --- | --- |
| Focused unit suite | 48 tests in 10 files passed |
| Desktop browser | 21 passed, plus a final palette case after dark-fill polish |
| Phone browser | 13 passed, plus a final palette/wrapping case on fresh assets |
| Administrator/member boundary | 1 browser case passed |
| Verified badge text contrast | Light 6.31:1; dark list 10.14:1; dark detail 10.97:1 |
| Typecheck, formatting, lint, localization | Passed |
| Public docs | 63 tests and 54 pages passed |
| Specs and delivery | 36 tests, catalog, lint, links, whitespace and delivery-reference checks passed |

The 35 distinct browser cases are UI/metadata fixtures plus the auth boundary;
the SSH transport/runtime was unchanged and was not rerun. Actual RED/GREEN
failures, source scope, exact verification commands, screenshots and receipts
are recorded in [Task 01 results](task-01-visible-controls.md#results).
No source of credentials, permission policy, readiness calculation, navigation
URL, shipped default or global UI primitive changed.

The worktree remains uncommitted. The manual lab and main instance were not
refreshed or modified. PR/CI/review and publication remain outside this package.

## Risks

Green could imply a stale or live guarantee; use the current state derivation
and retain the verification/executor distinction. Full-card links must retain
keyboard focus and admin gates. Toolbar placement must not remount secret
panels or lose contextual drafts. Avoid fixing one page with global primitives.

## Related records

[Implemented prior UX package](../ssh-connections-ux/plan.md),
[requirements](../../specs/executors/requirements/ssh-connection-manager.md), and
[interaction design](../../specs/executors/system-design/ssh-connection-management-ui.md).
Public setup navigation in `docs/public/executors.md` needs its header wording
updated during implementation. Existing coverage mapping remains applicable.

## Changed-file manifest

```text
apps/web/app/settings/executors/page.tsx
apps/web/components/settings/ssh-connections/connection-collection.tsx
apps/web/components/settings/ssh-connections/manager-header.tsx
apps/web/components/settings/ssh-connections/status-badge.tsx
apps/web/e2e/helpers/ssh-management-layout.ts
apps/web/e2e/helpers/ssh-navigation.ts
apps/web/e2e/helpers/ssh-verified-status.ts
apps/web/e2e/tests/settings/mobile-ssh-management-layout.spec.ts
apps/web/e2e/tests/settings/ssh-management-layout.spec.ts
docs/plans/ssh-connections-ux/plan.md
docs/plans/ssh-discovery-status/plan.md
docs/plans/ssh-discovery-status/task-01-visible-controls.md
docs/public/executors.md
docs/specs/executors/requirements/ssh-connection-manager.md
docs/specs/executors/system-design/ssh-connection-management-ui.md
```


## Proposed enrollment and diagnostics follow-up (2026-11-15)

The [access experience package](../ssh-access-experience/plan.md) revises enrollment
and diagnostics after new user feedback. Its work orders are pending. This
package's completed status and validation above remain historical evidence.
Discovery placement, green status, ownership and security contracts remain.
