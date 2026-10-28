---
status: current
system: executors
requirements:
  - REQ-EXECUTORS-SSH-CONNECTIONS-001
  - REQ-EXECUTORS-SSH-CONNECTIONS-002
  - REQ-EXECUTORS-SSH-CONNECTIONS-003
  - REQ-EXECUTORS-SSH-CONNECTIONS-004
  - REQ-EXECUTORS-SSH-CONNECTIONS-005
---

# SSH connection management interaction design

## Scope and evidence

Executors owns this interaction because it manages the reusable SSH access
contract. This is a focused companion to the [connection service design](ssh-connection-manager.md),
not a separate UI-owned feature. Credential custody, APIs, host verification,
reference guards, runtime behavior, and shipped feature defaults stay with that
design and its [accepted custody decision](../../../decisions/2026-09-19-managed-ssh-credential-custody.md).

The 2026-10-26 user screenshots show an isolated SSH navigation button on the
Executors page, a narrow name-only list, and an always-editable detail card
containing trust, diagnostics, raw key data, enrollment, and deletion. At baseline
`b3517cdd869f2d6db96ad76c666338473e816cdf`,
`ssh-connections/settings.tsx`, `connection-details.tsx`, and
`enrollment-panel.tsx` establish the same structure. Repeated fixture labels
also exist in separate preserved lab rounds; that evidence does not establish
a duplicate-rendering bug. Do not deduplicate persisted records.

This document carries forward the navigation and continuation contracts from
the completed [navigation package](../../../plans/ssh-connections-executor-navigation/plan.md).
The [UX revision package](../../../plans/ssh-connections-ux/plan.md) records the
new presentation below, its work orders, and local implementation evidence.
The user authorized implementation after reviewing that package.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-EXECUTORS-SSH-CONNECTIONS-001 | Identity collection; Commands and security |
| REQ-EXECUTORS-SSH-CONNECTIONS-002 | Saved overview; Setup and access states |
| REQ-EXECUTORS-SSH-CONNECTIONS-003 | Explicit enrollment; Commands and security |
| REQ-EXECUTORS-SSH-CONNECTIONS-004 | Collections; Saved overview; Setup; Continuity; Mobile |
| REQ-EXECUTORS-SSH-CONNECTIONS-005 | Navigation compatibility; Failure behavior |

## Components and composition

Keep `useSSHConnections` as the data/mutation owner. Presentation consumes its
records and commands. Do not add component fetches, another data cache, new
management APIs, or speculative global form infrastructure.

Compose `SettingsPageHeader` from `settings-typography.tsx`, `SettingsTabs`,
`SettingsGroup`/`SettingsRow`, `SettingsInfo`, and shared settings control helpers.
Use `SettingsPageTemplate` with `contentFrame="none"` for metadata editors so
the existing save coordinator can own drafts without adding a giant outer card.
The nearest visual precedents are the expanded groups in `editors-settings.tsx`,
the header actions in `workspace-canvases-page.tsx`, and executor profile rows.
Reuse their anatomy, not the profile row's non-semantic clickable div.

Split domain presentation into collection, overview, metadata form, access,
diagnostics, and identity detail components. These are local domain components,
not shared APIs. Keep components below 200 lines. A small
pure view model derives displayed states from current records; it must not
invent persisted statuses or trigger commands.

## Navigation compatibility

Canonical location remains `/settings/executors/ssh-connections`. Keep the
static route ahead of dynamic profile matching and the old pathname redirect
with its query/hash and replacement-history behavior. Both locations retain
feature/admin gates before mounting data hooks. Executors remains active in
settings navigation and remains the discovery parent of `ssh-connections`.

Move `SSHConnectionsEntry` into the Executors `SettingsPageHeader.actions` slot.
On desktop it sits beside the title block. On phone it occupies the shared
header's action row. Remove the dedicated navigation-only spacer, not profile
or create-profile sections. It stays visible with zero profiles.

Keep existing `tab=connections|identities`, `id`, `new`, adoption, bastion-return,
and identity-return parameters. Extend the central
`sshConnectionSettingsPath` helper with validated public view parameters:
`section=overview|access|diagnostics` and `mode=edit`. The central route helper
validates these values.
Absent or invalid values resolve to overview; `new=1` opens the metadata form.
Old selected-record bookmarks now open its overview. A newly saved draft may
navigate to Access, but that navigation cannot start a probe or trust operation.
Clear entity-specific mode/section when switching records or collection tabs.

Use real links for collection destinations. Preserve browser Back/Forward,
list scroll and row focus on return. Keep one explicitly labelled Back to
connections/identities action on detail pages. There is no desktop mini-sidebar.

## Collections

Connections and Identities use the shared header tabs. The active collection
owns one New action. Render one full-width grouped list with restrained row
dividers, rather than a row of buttons or a card around each row.

A connection row shows name, `user@host:port`, identity name, optional bastion,
and a text status. The row body opens the resource. An identity row shows name,
kind, fingerprint when available, and dependency count. Do not show raw key
blocks, edit controls, or deletion buttons in either collection.

Rows with identical names remain independent. Endpoint and identity metadata
normally distinguish them; if these also match, show a short public record ID
as a final discriminator. Do not introduce host-based merging or uniqueness.
Keep the complete record ID available in detail. No search, pagination, bulk
actions, or new naming policy is needed for this bounded revision.

Empty collections offer one creation action and one short explanation. Loading
uses shared skeleton/status treatment in the content region. Failed reads show
Retry rather than a misleading empty collection. Disabled/denied views mount
no management hook.

## Saved overview and metadata editing

The detail header shows the resource name, endpoint description, text status,
and a visible actions menu. Header tabs are Overview, Access, and Diagnostics.
Overview is a readable summary, with Connection and Identity/Route groups.
Each group stays expanded; do not convert every section into an accordion.

A ready overview has one emphasized `Test SSH access` command and a secondary
Edit action. Verification means the current route stamp passed authentication,
not current reachability or executor readiness. A new failed test takes visual
priority over historical success. Use the existing domain distinction in copy.

Edit opens the dedicated metadata form in the same focused detail composition.
Keep Name, Host/Port, SSH user, and Identity in logical groups. Direct access is
the default. Optional bastion fields appear after explicitly enabling that route
choice; an existing selected bastion always remains visible. Create identity
and Create bastion are quiet contextual actions at their respective fields.
Explain that creation preserves the connection draft. Avoid implementation
phrases such as 'save draft and create identity' as the dominant button label.

The shared floating Save/Reset surface owns metadata persistence. Edit, Cancel,
collection navigation, and header-tab changes participate in the coordinator's
existing unsaved-change guard. No second local metadata Save button is added.
After a successful metadata save, return to overview for a ready route or Access
for incomplete setup. Never silently run an authentication check.

Delete lives in a visible overflow menu with its existing confirmation. Explain
reference conflicts and route to current dependents. Do not render a red button
inside the routine connection form. An identity's dependents remain visible in
its overview even when deletion is refused.

## Setup and access states

Access uses one status summary and a focused next step. Completed steps become
short readable summaries. Do not expose all edit, trust, authorization, and
diagnostic forms at once. The steps are not a persisted wizard.

| Current evidence | Visible next action | Secondary path |
| --- | --- | --- |
| Unsaved metadata | Shared Save changes | Reset or cancel editing |
| Incomplete target/identity | Edit details | Contextual identity/bastion creation |
| No trusted host key | Inspect host key | Independent verification guidance |
| Fresh observation | Trust this host key with existing confirmation | Cancel observation |
| Trusted, not checked | Test SSH access | Install key using existing access; diagnostics |
| Authentication rejected | Retry after correction | Diagnostics or explicitly open Install key |
| Current stamp verified | Test SSH access | Host details, copy public key, explicit Install key |
| Changed key or stale stamp | Blocking explanation and fresh inspection | Edit/reload, no credential fallback |

All steps use current route-stamp eligibility. Inspecting an already verified
host is available in Access, without giving it equal weight on the overview.
Full fingerprints remain available in labelled details and always appear in
trust/change confirmations. A changed-key warning shows old and new values and
never collapses behind optional help.

Diagnostics owns `PrerequisitePanel` and its hop-specific results. Opening it
does not run preflight. Its explicit Check prerequisites command is separate
from SSH authentication. Keep the executor-readiness distinction visible near
results, with optional longer explanation in `SettingsInfo`.

## Explicit enrollment and identity collection

The Access view offers `Copy public key` without opening a large key block.
`Show public key` reveals the complete public value in a bounded wrapping
region. Key bytes are never truncated in clipboard output. Identities have
their own overview, custody explanation, public-key controls, rename editor,
and linked dependencies using the same page anatomy.

`Install key` opens a focused inline enrollment step within Access. This is a
substantial form, so use direct page content on phone as well as desktop.
Show the required existing-access explanation before its method selector.
Only the chosen password or bootstrap-identity fields mount. Show the existing
destination/user, trusted-host fingerprint and public-key fingerprint in the
explicit confirmation; retain `ManagerConfirmation` and its phone adaptation.

Verified enrollment closes the credential form and shows its sanitized result
and Test SSH access. Opening it again requires an explicit action. For
`installed_unverified`, `outcome_unknown`, or interrupted work, keep a visible
warning that authorization may already have changed. Offer a fresh test;
reinstallation requires a new deliberate action and credential. A failure must
not disappear merely because the form closed.

Changing section, record, method, route stamp, or leaving/cancelling the form
clears its secrets. `SettingsTabsPanel` force-mounts visited panels, so do not
rely on tab invisibility as cleanup: conditionally mount secret-bearing forms
only while active, and guard late responses after departure. Non-secret
metadata drafts stay with their existing save owner.

## Setup continuity

Keep the reviewed identity and bastion return behavior. Contextual creation
first saves non-secret connection metadata through the coordinator, then opens
the focused related form. Failed save stays in place. Success selects the new
identity as an unsaved connection draft; Cancel preserves the old selection.
Preserve outer adoption/bastion context and consume return selections once.

`SSHSetupContinuationProvider` remains in the persistent settings shell outside
the pathname-keyed save provider. Its typed in-memory executor continuation
retains the editor route, draft name, selected connection, new-profile owner,
and baseline binding evidence. Never add credential fields, storage, or URL
serialization. Clear on save, discard, flow exit, logout, and reload.

Return reloads current records and clears executor-check success. A changed
baseline requires reconciliation. Return to executor preserves selection;
Use connection is explicit and offered only for a ready route. Missing
continuation opens the normal editor without fabricated draft state. Final
binding still requires a new executor check and shared Save.

## Mobile, geometry, and visual treatment

Use `useResponsiveBreakpoint` at the canonical 768px phone boundary. Desktop
uses full-width rows and a focused detail page. Phone uses one row list, direct
detail navigation, and a vertically grouped form. Its primary action follows
the compact summary; the settings Save surface keeps its established safe-area
behavior. Coarse tablets retain suitable wider composition with touch sizing.

The curated `kanban-with-preview.tsx` supplies the direct-navigation principle;
`mobile-menu-sheet.tsx` supplies inset temporary-choice geometry. Reuse existing
`ConnectionPicker` drawers for short identity/bastion choices. Do not import
task-owned mobile components or stack a creation form inside a picker drawer.
Existing responsive confirmation/menu primitives own their phone presentation.

The settings shell is the only vertical page scroller. Inline steps do not add
nested scroll panes. Any overlay uses its primitive's bounded `100dvh` layout,
safe-area padding and `overscroll-contain`. Header tabs may scroll within their
own bounded region; they cannot expand document width. Restore focus to the
trigger/row and announce commands through localized `role=status` feedback.

Use existing semantic tokens, `SETTINGS_TYPOGRAPHY`, and 28px fine-pointer
controls. Phone and coarse-pointer targets are at least 44px. Use normal shared
section spacing (`space-y-8`), group spacing (`space-y-3`) and row padding, not
larger controls or a new font scale to compensate for poor grouping. Keep one
bordered surface per group. Match existing muted status/badge treatments and
Tabler outline icons. No new shadows, staggered entrances, animated stepper,
or global component/CSS changes. Existing transitions honor reduced motion.

## Commands, failures, and security

Metadata Save never replays generate/import/test/trust/enroll. Opening a view,
returning from another view, resizing, or restoring a bookmark never starts a
remote operation. Existing duplicate-submission and stale-response guards stay
at their domain boundaries. Preserve pinned verification of both hops and
server authorization. This package changes no endpoint, SQL, key storage,
agent environment, audit data, or remote authorization policy.

Missing entities show an unavailable state with a route back to the collection.
Version conflicts preserve drafts and require reload/reconciliation. Failed
commands display their sanitized localized error in the active region. A
stale success cannot advance setup or suppress a changed-key warning. Gate
return/selection on current data, not presentation state.

Localize new copy in all seven languages and pseudo. Keep raw public
fingerprints and confirmation targets outside translation. Update the public
executor setup instructions with implementation. No new telemetry is needed.

## Verification and delivery

The [plan](../../../plans/ssh-connections-ux/plan.md) maps the new ACs and the
existing safety criteria to sequential work orders. Unit tests cover view-state
derivation, route allowlists, coordinated edits and secret cleanup even for
visited tabs. Desktop/phone browser checks prove the actual hierarchy, focus,
touch geometry, long labels, pseudo locale, and explicit commands. Existing
real OpenSSH enrollment/adoption scenarios remain required compatibility proof.

Use disposable backend/SSH fixtures. Preserve the running main instance and
the user's current lab during implementation and verification. Updating that
lab or publishing this revision requires a later request. No new ADR is needed:
this revision preserves the accepted ownership, custody, and API boundaries.
