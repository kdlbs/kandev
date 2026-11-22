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

Executors owns reusable SSH access. This interaction design complements the
[service design](ssh-connection-manager.md) and preserves its
[credential custody decision](../../../decisions/2026-09-19-managed-ssh-credential-custody.md).
The implemented [navigation](../../../plans/ssh-connections-executor-navigation/plan.md),
[UX](../../../plans/ssh-connections-ux/plan.md), and
[discovery/status](../../../plans/ssh-discovery-status/plan.md) packages record
prior decisions and validation. The November 15 lab includes the discovery
card, leading controls, and green ready badge. The proposed experience revision
below is unimplemented and changes enrollment/diagnostics presentation only.

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

Render `SSHConnectionsEntry` as one full-width navigation card in the Executors
content, immediately below the title/description and before Profiles. Remove
it from `SettingsPageHeader.actions`. Reuse the connection-collection and plugin
row anatomy: leading key icon, existing localized title and manager description,
trailing chevron, one semantic Link for the entire card, visible keyboard focus.
There are no nested buttons or competing destinations. Keep existing feature
and administrator gates before rendering it. Its absence leaves no empty group
or navigation-only spacer. It stays visible with zero profiles.

The card uses content geometry and theme tokens, with at least a 44px body
hit area. It is not an enlarged ordinary desktop button. On phone the description
wraps under its title and the whole card opens the existing manager route.
Executors remains its settings parent. The user now requests a direct child
entry in its sidebar branch. Prepend one static SSH connections leaf in
`buildExecutorsBranch`, gated by feature/admin eligibility from
`useSettingsMenuBranches`. Reuse the canonical route and translated title.
It is first, with a key icon, no expansion chevron, and no profile-count impact.
Use existing active-path expansion in both tree modes and phone SettingsTree.
Flat mode retains its existing flat navigation and the Executors content card.
No SSH records are fetched just to render navigation. See
[Task 04](../../../plans/ssh-access-experience/task-04-sidebar-entry.md).

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

Compose the manager title/description with `SettingsPageHeader`, but render its
tabs outside that component's trailing slot. Place `SettingsTabsList` in a
leading-aligned toolbar below the title. On desktop the collection's one New
action follows the tabs with a bounded gap, not `justify-between`, auto margins,
or a far-right rail. On phone put the tabs first and the create action on a
separate full-width row. Keep the same order at the 767/768px boundary; a coarse
tablet uses touch-sized controls while retaining an appropriate wrapped toolbar.
On saved connection details the section tabs also sit below the title, aligned
to the content. Existing Edit/menu and Cancel actions keep their established
role; creation/edit states still suppress irrelevant tabs.

Retain shared `SettingsTabs` state, ARIA/manual keyboard activation, validated
route helpers and save guards. Keep header controls outside the conditional
loaded content so empty/loading/error views retain the permitted create action.
Manager busy state still disables mutations. Do not change the shared global
`SettingsPageHeader` or `SettingsTabs` geometry to fix this one feature.

Render one full-width grouped list with restrained row
dividers, rather than a row of buttons or a card around each row.

A connection row shows name, `user@host:port`, identity name, optional bastion,
and a text status. Use one local `ConnectionStatusBadge` for collection and
detail status presentation, consuming the existing `connectionState` result.
Only `ready` gets a green success border/fill and a contrasting green foreground.
Reuse theme success colors and verify at least 4.5:1 text contrast in both themes;
do not change global color tokens. All other states retain their existing
non-success treatment. Keep the translated text label, rather than conveying
verification through color alone. A fresh failed probe or uncertain enrollment
must remove green even when historical verification remains in the record.
No second readiness computation, persisted status or probe is added.
The row body opens the resource. An identity row shows name,
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
| Fresh observation | Trust this host key in the inline review | Cancel observation |
| Trusted, not checked | Test SSH access | Install public key using existing access; diagnostics |
| Authentication rejected | Retry after correction | Diagnostics or explicitly open Install public key |
| Current stamp verified | Test SSH access | Host details, copy public key, explicit Install public key |
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

The manual installation choice offers `Copy public key` without a large key block.
`Show public key` reveals the complete public value in a bounded wrapping
region. Key bytes are never truncated in clipboard output. Identities have
their own overview, custody explanation, public-key controls, rename editor,
and linked dependencies using the same page anatomy.

`Install public key` opens a focused inline enrollment step within Access. This is a
substantial form, so use direct page content on phone as well as desktop.
Show the required existing-access explanation before its method selector.
Only the chosen password or bootstrap-identity fields mount. Show the existing
destination/user, trusted-host fingerprint and public-key fingerprint in the
inline review. `ManagerConfirmation` remains for unrelated destructive actions.

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
Read-only identity remedies carry allowlisted `return_section` context to return
to Access or Diagnostics without opening the connection editor.

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

Reuse `ConnectionPicker` drawers for short identity/bastion choices. Do not
import task-owned mobile components or nest creation forms in picker drawers.
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


## Access experience revision (2026-11-15)

This implemented section defines the current Access, enrollment and diagnostic
presentation. See the [experience package](../../../plans/ssh-access-experience/plan.md)
for verification evidence.
AC-004.15 through AC-004.19 belong to REQ-EXECUTORS-SSH-CONNECTIONS-004.
AC-002.2/002.3 and AC-003.1 through 003.6 remain security constraints.
Collections, route ownership, metadata saving, executor return, and identity
custody keep their accepted contracts. No new ADR or backend API is needed.

### Evidence and visual direction

Live desktop and Pixel 5 inspection on 2026-11-15 confirmed repeated enrollment
headings, competing Access and authorization cards, a narrow fingerprint-heavy
confirmation popover, and diagnostics reduced to a button and issue bullets.
`connection-access.tsx`, `connection-trust.tsx`, `enrollment-panel.tsx`, and
`prerequisite-panel.tsx` establish those compositions. The existing package
improved discovery but did not give these operation states sufficient hierarchy.

Use the existing `SettingsGroup`/`SettingsRow` anatomy from
`editors-settings.tsx`: one heading, a quiet surface, and row separators.
`kubernetes-diagnostics-card.tsx` supplies the icon/title/detail vocabulary for
results, but its individual bordered rows and reported step timings are not
copied. SSH does not return that evidence. `kanban-with-preview.tsx` supplies
phone direct navigation; `mobile-menu-sheet.tsx` supplies bounded temporary
picker geometry.
The access body and diagnostic report use a leading-aligned `w-full max-w-3xl`
column under the unchanged manager header. Keep one bordered main surface with
16px content padding, 24px between semantic sections, and subtle row dividers.
Do not nest result cards inside status cards. Use shared heading/body/meta type
styles, Tabler outline icons, and existing radius/color tokens. Use normal
foreground for titles, muted text for explanation, purple only for the current
primary action, and success/warning/destructive tokens only for their semantic
states. Fingerprints use a labelled, wrapping monospace value with exact copy.
Keep existing tokens and control sizes. Do not change global primitives.

### Access composition and trust review

Retain Overview, Access, and Diagnostics navigation. Access has three compact
milestones: Connection details, Server trust, SSH access. They describe evidence,
not required enrollment steps. Installation is optional under SSH access.
Desktop shows a compact horizontal summary above one active surface. Phone shows
only the current milestone title and a short completed-prerequisite summary;
it does not squeeze three step labels into a horizontal strip. Completed facts
remain available through Host details/Identity links, without expanded forms.

Incomplete metadata offers Edit connection. Missing identity offers identity
selection through the connection editor. Missing or unready bastion links to
that bastion's Access view, preserving existing return context. A trusted but
unchecked route offers Test SSH access, with Install public key as a quieter
alternative. A ready route shows one green result and Test SSH access, with
Host details and Install public key as secondary actions. Do not demand
installation from a user who already has working access.

Inspect host key shows a local pending indicator, then replaces its task body
with an inline review: destination account, hop, observed server fingerprint,
and independent comparison guidance. Changed keys additionally show the saved
fingerprint, observed fingerprint, and an always-visible warning. Trust this
server is the explicit commit action; Back discards the observation. Do not add
another popover after this already complete review. No credentials reach the
untrusted hop. Preserve the receipt, actor, expiry, and route-stamp checks in
`manager.trust`. An expired receipt offers Inspect again, not blind retry.

Review is a local view, not a persisted step or new public query parameter.
Changing record, route stamp, selected identity, method, or leaving the view
invalidates the prepared decision. Back and Escape leave review and restore
focus to its initiating control. Resizing an active decision across the phone
boundary invalidates it using the same safety policy as the existing responsive
confirmation adapter. Required public values never depend on hover disclosure.
`ManagerConfirmation` remains for unrelated deletion; access and enrollment
use the new inline review instead of its anchored popover.

### Optional installation flow

Install public key replaces the ordinary access body, retaining the connection
header and tabs. The focused flow has three views: Choose access, Review, Result.
It shows its own concise progress label only while active, replacing the Access
milestones. Back to access is visible. Do not mount an ordinary Access test
button and a competing enrollment form together.

Choose access presents three full-width radio rows with a title and one-line
explanation: Password, Existing identity, Install manually. The method is not
preselected. Selection expands only that method's fields in the same surface.
The two assisted choices explain that existing access to the destination account
is required. Password means the destination account's SSH password used once,
not a generated one-time-password or an OTP protocol. It is never forwarded to
the bastion. Existing identity uses `ConnectionPicker`; no eligible identity
shows a useful empty state and an alternate method, without generating keys.
Preserve current eligibility; do not silently exclude an identity the API accepts.

Manual installation reveals the public key, full-value Copy public key, and
instructions for the named remote account. It does not execute a command or
claim installation occurred. Test SSH access is the explicit next action. No
new copyable shell script, remote console, or configuration overwrite is added.

Review installation is a full inline review, never a small anchored popup:
labelled Account (`user@host:port`), Via bastion when present, Server fingerprint,
Key to install (identity label and public fingerprint), and Access method.
Explain that Kandev appends this key, preserves existing entries/restrictions,
and then tests with the new identity. State that no administrator privileges
are requested. Do not render the password, even as a summary or masked echo.
Only Install public key dispatches `manager.enroll`. Enter in a credential field
may move to review but cannot submit enrollment. Back clears the password,
preserves the chosen non-secret method, and requires fresh input before review.

Keep request admission and request IDs at the existing domain boundary. Bind
the review to the exact route stamp, key fingerprint, and method. The form owns
its password until dispatch, clears component state at dispatch, and never
puts it in shared navigation state, storage, URLs, logs, or screenshots.
Submitted request data necessarily lives until the request settles. A section,
record, method, target, cancellation, or responsive decision change clears the
secret and invalidates review. A late result updates domain records but cannot
reopen a form or focus a control on another view.

While pending, replace credentials with one status region: Installing key and
checking access. Do not invent separate live installation/verification phases:
`Enroll` returns a terminal result, not a progress stream. Disable duplicate
submission. Back to access is navigation, not a claim to cancel remote work;
show that an already submitted operation may still finish. Retain started or
unknown evidence when returning and offer explicit refresh/test as permitted.

Render terminal results as a single outcome, with icon, title, concise detail,
and a relevant primary action. `verified` shows green and removes credentials.
`failed` names the safe error and allows a new attempt with fresh credentials.
`installed_unverified` says the key was installed but access was not verified.
`outcome_unknown` says the key may have been installed. The latter two use a
warning and Test SSH access as primary, never automatic reinstall or rollback.
A subsequent test does not erase uncertain history or mark the connection ready
unless existing domain evidence allows it. Opening another installation attempt
is always explicit. Results of a previous route are not current success.

### Diagnostics with honest scope and remedies

`Service.Preflight` in `internal/sshconnections/preflight.go` returns a route
stamp, a list of issues, and readiness partly derived from saved verification.
It examines metadata, trust presence, local key/agent availability and bastion
readiness. It does not perform a fresh network login or test executor tools.
`SSHPreflight.checks` contains failures only. No per-check durations or successful
steps exist. Preserve the API and do not manufacture a checklist of green rows.

The Diagnostics view has a heading, a one-sentence scope description, Check
setup, and one result region. Before running, state Not checked in this view.
Pending replaces the previous current report with Checking setup; optionally
retain it only under an explicitly Previous result disclosure. A request error
shows Could not complete setup checks with Retry. A successful response with
zero issues says No setup issues found and offers the separate Test SSH access
action. It does not label authentication successful because `ready` is true.
Show existing saved SSH status separately as previously verified evidence.

For issues, show a summary count using i18n plurals and group rows by Destination
and Bastion, each labelled with its endpoint. A row contains one semantic icon,
a plain-language title, one short explanation, and a relevant link/action.
Keep full code and public context in an optional Technical details disclosure.
Unknown codes use a localized safe generic error and code text, never raw remote
stderr or arbitrary response-body values. Do not guess a failed hop when absent.

| Reported code | Remedy |
| --- | --- |
| `ssh_target_required`, `ssh_identity_required` | Edit the affected connection; focus its required field |
| `ssh_trust_required`, `ssh_host_key_changed` | Open the affected hop's Access view; fresh inspection and explicit trust |
| `ssh_bastion_not_ready` | Open the bastion's Access view, with the existing return path |
| `ssh_agent_unavailable`, `ssh_agent_empty` | Open identity details; explain backend-side socket or loaded-key problem, and offer Edit connection to select another identity |
| `ssh_key_file_unavailable` | Open identity details; explain backend-side path/access problem without offering a nonexistent path editor |
| `ssh_managed_key_unavailable` | Explain matching protected-storage recovery; no replacement-key shortcut |
| `ssh_authentication_rejected` from a live test | Explain rejection without claiming key absence; offer Access/installation and Retry explicitly |
| Unknown or general connection error | Safe explanation, Retry, and visible technical code if available |

Test SSH access remains a separately invoked command with its own pending/result
region, labelled as a network authentication test. It never appears successful
because setup checks passed. Use sanitized `ApiError.body.error` and validated
`hop` when available. Any action link uses canonical settings path helpers,
retains continuation context, and only navigates. It never fixes trust, installs
a key, modifies an agent, or starts a command merely because it was opened.

### Operation feedback ownership

`useSSHConnections.perform` keeps same-tick admission and reload/error semantics.
Add bounded active-operation metadata there only if necessary to route feedback;
never move duplicate protection into a disabled button. Preserve the existing
method API. Operation feedback is scoped to kind, connection ID, and route stamp,
with sanitized code/hop. No second request owner or cross-view result cache.

`settings.tsx` currently emits global Working and a global error with Reload.
Access and diagnostics must own feedback for their active operation, so the
same request does not also create a detached global banner. Initial load,
collection and unrelated mutation errors retain their existing global owner.
An unhandled error must still have a visible fallback. Returning to another
record must not display an old operation error as that record's current result.
One localized live status region announces each operation. Keep command names
stable while pending and add an aria-hidden indicator. Error text explains the
cause before the remedy. Do not depend on a toast or color to convey completion.

### Mobile, keyboard, localization, and proof

Desktop uses the bounded reading column; phone below 768px uses full-width
content, one focused step and vertically stacked labelled values. The settings
shell remains the sole page scroller. Main actions follow the current task,
full-width on phone, with safe-area clearance and no fixed bar that covers the
keyboard. Short identity choices reuse `ConnectionPicker`'s inset drawer.
Dense review stays on the page. No stacked overlays or phone sidebars.

Use `useResponsiveBreakpoint`, shared control helpers, 28px desktop controls and
at least 44px phone/coarse hit targets. Preserve 16px touch input behavior.
Focus the new step heading after an explicit Next/Back action, announce its
status, and return focus on close. Avoid a changing accessible command name.
Respect reduced motion. Keep tab switches immediate; any optional status fade
is opacity-only, at most 150ms, with a static icon/text cue. No global motion work.

All new copy uses settings locale keys in en, pt-pt, zh-cn, zh-tw, zh-hk, ja,
ko and pseudo. Generate Traditional Chinese with the repository command.
Public executor instructions describe this implemented flow.
Tests must cover all method/result states, stale evidence, one-submit semantics,
secret cleanup, direct/bastion routes, unknown errors, focus, dark/light/pseudo,
767/768px transitions, coarse tablet and real disposable OpenSSH. Visual review
must compare every stage against the package's structured previews, including
pending and failure. Existing safety tests must survive selector migrations.
