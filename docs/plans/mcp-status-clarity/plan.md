---
created: 2026-10-06
status: done
requirements:
  - REQ-PLATFORM-MCP-SESSION-OBSERVABILITY-001
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-006
system_design:
  - ../../specs/platform/system-design/mcp-session-observability.md
  - ../../specs/agents/system-design/workspace-mcp-configuration.md
legacy_specs: []
---

# Implementation Plan: MCP status clarity

## Overview

Explain what the active agent received separately from changes saved for later.
First implement one shared status presenter and the desktop/phone views; then
prove the mixed states in browser journeys and deploy the reviewed assets to
the owned sandbox. The existing instance already has five additional API-created
scenarios, described in [the testing guide](testing-guide.md).

## Evidence and assumptions

The live original session records `pending_idle` (desired revision 9, applied 0),
Active built-in Kandev, and three third-party servers Filtered with
`http_unsupported`. The mock agent advertises SSE, not HTTP. The hover displays
a generic application label followed by server status labels; it ignores the
recorded reason unless a free-text summary exists. The server detail omits the
reason code too. This makes a deliberate transport exclusion look like a tool
selection or connection failure.

The live Kandev row can retain `http_unsupported` while becoming Active through
SSE. A reason presenter must respect the current evidence state. Additional
sandbox evidence also shows an absent stdio command as Delivered by the mock;
delivery is not proof the executable was launched. No mock capability or
connection evidence will be fabricated to make a case appear successful.

Confirmed: retain the composer plug, save real test data, preserve the main
instance and original scenarios. Agent design choice within that scope: refine
copy/grouping in the existing dialog/drawer rather than add another interaction.
Platform owns this refinement because it owns session attachment evidence;
Agents retains the independent desired/applied selection contract. No new ADR
is needed: the accepted session-owned observability decision already establishes
this distinction. No material user question blocks planning.

## Scope

### In scope

- Readable exclusion reasons and delivery-versus-verification explanations.
- Saved-change grouping in the hover, dialog, and native phone drawer.
- Existing pending, next-start, failed, applied, and unknown fallback states.
- Localized copy, mixed-state unit/browser coverage, and the sandbox guide.
- Stronger Customize section hierarchy and restrained application-state colors.

### Out of scope

- Transport support changes, connection probes, provider protocol extensions,
  fabricated attachment evidence, or changing the missing-executable resolver.
- New application-state transitions or changes to reconnect/retry behavior.
- Main instance restart, marketplace registry mutation, tool pruning, or PR push.

## Technical approach

Extend `mcp-explorer-view-model.ts` with a presentation result containing label
and explanation keys, derived from status and stable reason codes. Inventory
actual backend producers before mapping reason codes. Reuse it in
`mcp-status-presentation.tsx`, `mcp-server-list.tsx`, and
`mcp-server-detail.tsx`; do not create a second state store.

Refine `mcp-selection-presentation.ts` and the existing explorer application
notice so that saved changes and current evidence are visually separate. Keep
the controller, single poller, context guards, failure ownership, trigger marker,
  and automatic Customize entry unchanged. A waiting notice must not imply the
current rows already show the new configuration. No per-server change count is
available from an authoritative applied-selection snapshot.

The session body in `apps/web/components/task/mcp-session-selector.tsx` gains
an application summary with a semantic icon and restrained accent, plus distinct
`From task setup` and `Additional servers` groups. The former has a neutral
surface, section-level identity and compact readable origins; remove repetitive
Inherited badges and dashed per-row cards in this surface. The latter has its
own heading, supporting sentence and search. Group spacing and heading weight
carry the hierarchy; color is reserved for application state. Use composition
or a narrow shared-content presentation option to preserve other picker callers.
Do not change row order after a click or introduce collapse that hides required
choices. Applied means the configuration was accepted, not that all tools work.

Add all copy through translations in en, pt-pt, zh-cn, ja and ko. Generate the
Traditional Chinese pair and pseudo catalog with the repository tools. Update
`docs/public/agents-and-profiles.md` only after the visible behavior is implemented.

| Provider/transport shape | Honest behavior | Evidence/fallback |
| --- | --- | --- |
| Mock ACP SSE | Configuration delivered; calls can reach the local fixture | Does not make third-party attachment Active |
| Mock ACP HTTP | Not available: agent does not support HTTP | Recorded `http_unsupported` |
| Absent stdio executable | Explain recorded state only | Current mock reports Delivered; do not invent a command failure |
| Built-in dual SSE/HTTP | Active when current tools/list is observed | Ignore obsolete filter reason on Active row |
| Direct third-party endpoint outage | Saved/delivered is distinct from reachability | A scripted call can fail without an attachment-failure report |
| Pending / deferred / failed selection | Saved change state separate from connection evidence | Preserve authoritative application state |

## ASCII UI preview

UI-01: Existing desktop hover (confirmed in source and user screenshot).

```text
Pending until idle
MCP servers
  kandev             Active
  issue-tracker      Filtered
```

UI-02: Proposed desktop hover/focus, pending mixed state.

```text
Saved changes
  Saved, not applied yet.
  Waiting to update the agent's setup.

Current agent
  kandev             Active
    Tools loaded for this agent.
  issue-tracker      Not available
    This agent does not support HTTP MCP servers.
  atlas-docs         Delivered
    Configuration sent. Connection not verified.

Click the plug to review or customize.
```

UI-03: Proposed phone plug drawer, Connections view.

```text
+---------------------------------------+
| MCP servers                       [X] |
| [Connections]  [Customize]             |
| Saved changes                         |
|   Saved, not applied yet.              |
| Current agent                         |
|---------------------------------------|
| kandev                  Active     >  |
| atlas-docs              Delivered  >  |
|   Connection not verified.             |
| issue-tracker           Not available |
|   This agent does not support HTTP.    |
|                                       |
+---------------------------------------+
```

UI-04: Alternate application notices in the same region.

```text
Next start: Saved. Applies on the next agent start.
Failed:    Saved, but the agent did not accept the update.
Applied:   Compact "Setup up to date" in Customize; no pending hover notice.
Unknown exclusion: Not available. Not included in this agent's setup.
```

UI-05: Current Customize hierarchy from the user's screenshot.

```text
[Applied]
The current session is using this MCP selection.
Inherited MCP servers
  [dashed server card] [Inherited]
  [dashed server card] [Inherited]
Add to this session
  [search]
  [bordered server card]
```

UI-06: Proposed Customize hierarchy, desktop.

```text
MCP servers                           [X]
[Connections]  [Customize]

(check) Setup up to date
        Your saved setup has been applied to this agent.

+-----------------------------------------+
| FROM TASK SETUP                       3 |
| Selected by your profile, repos or task. |
|                                         |
| Atlas shared documentation              |
|   Profile: Atlas tools / Repo: Atlas API |
|-----------------------------------------|
| Atlas issue tracker                     |
|   Repository: Atlas operations          |
+-----------------------------------------+

ADDITIONAL SERVERS
Choose extra servers for this agent session.
[Search servers__________________________]
[x] Atlas review notes
[ ] Session extras
```

UI-07: Proposed phone Customize drawer.

```text
MCP servers                       [X]
[Connections] [Customize]
(clock) Changes saved, waiting to apply
        Your current setup stays in use.
-------------------------------------
FROM TASK SETUP                     3
  Atlas shared documentation
  Profile: Atlas tools
  Repository: Atlas API
-------------------------------------
ADDITIONAL SERVERS
[Search servers_____________________]
[x] Atlas review notes
[ ] Session extras
```

The check uses a muted green accent, waiting a clock/amber accent, next-start
blue, and failed an alert/red accent. Avoid saturated full-panel fills; use the
existing theme tokens, small icon/accent and readable headline/description.
Status meaning must be visible without color. `Setup up to date` is compact;
pending/deferred/failure notices carry the next-action consequence. Waiting copy
does not say a turn is running without current activity evidence. The phone
header/view controls stay fixed; the summary and both groups share the existing
single scrolling body. Source labels wrap rather than form a long dense line.

Grouping, state meanings, wrapping, and reason visibility are required;
illustrative English wording and spacing are not pixel specifications. Desktop
hover is read-only. The phone drawer is the shipped MCP explorer exemplar, with
fixed header/view controls, one scroll owner, safe-area clearance, and 44px
phone/coarse-pointer targets. Desktop click retains the wide dialog and tool
drill-down. No nested drawers or new conversation bar.

These views cover AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.13 through .16 and
AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.9 through .13.
UI-06 and UI-07 additionally cover platform .17 and agents .14.

## Proposed follow-up: Task-local inherited exclusions

The user asked why inherited servers cannot be temporarily disabled for a task.
There is no protocol requirement for that restriction: the accepted additive
ADR deliberately omitted subtraction and conflict rules in the first design.
Local exclusion is a reasonable extension, not an edit to the shared profile
or repository defaults.

Recommendation for a later contract decision: task-owned excluded definition
IDs are applied after the union. `Off for this task` is visibly different from
a workspace-disabled definition or provider-filtered server. A repeated
definition selected by several origins is excluded once; its origins remain
visible. Restoring the task default removes the local exclusion. Exclusions
persist across reload/resume until restored, affect this task's agent sessions,
and use the existing safe idle/next-start application rules. New sessions in
this task resolve the latest local exclusions; other tasks remain unchanged.
A task exclusion should not be undone silently by a session reselecting the
same definition.

This is a recommendation, not an accepted contract or implemented toggle. It
requires an ADR amendment/new decision, agent requirements/design updates,
selection API and persistence work for SQLite/Postgres, lifecycle propagation
to every task agent, and precedence/restore tests. The host-owned built-in
Kandev endpoint is separately injected; pruning its tools belongs to issue
3808 and must not be accidentally disabled by a catalog-level exclusion.
Do not add an enabled-looking inherited toggle before this contract exists.

## Tests

- `mcp-explorer-view-model.test.ts`: supported reason mapping, unknown fallback,
  mixed states, delivered explanation, and Active with obsolete HTTP reason.
- `mcp-server-explorer.test.tsx`: saved/current grouping, application-state
  notices, reason in detail and accessible list description.
- `mcp-selection-presentation.test.ts`: pending/deferred/failed/applied marker
  behavior remains independent from server health.

## E2E tests

- `apps/web/e2e/tests/chat/mcp-status.spec.ts` (chromium): hover/focus reasons,
  pending notice plus mixed current evidence, deferred/failed/applied notices,
  customization entry, and save/reload with stable focus and search.
- `apps/web/e2e/tests/chat/mobile-mcp-status.spec.ts` (mobile-chrome): tap-only
  access to the same reasons, one drawer, wrapping/containment, scroll owner,
  Back/focus behavior, and minimum target geometry.
- Controlled attachment fixtures prove presentation. The five API-created live
  sandbox tasks prove actual supported flows, with provider limitations recorded
  rather than overwritten. See the guide for the distinction.

## Work orders

- [x] [Task 01: Explain saved changes and current evidence](task-01-explain-status.md)
- [x] [Task 02: Prove and deliver the desktop and phone journeys](task-02-prove-and-deliver.md)

## Verification results

Task 01 is done. Shared explanations, saved/current grouping, application-state
icons/colors and session section hierarchy are implemented. Task 02 is done, and the owned preview serves the updated UI.
Final browser evidence covers 16 distinct cases: 2 desktop explorer/status,
6 adjacent desktop task-selection, and 8 phone cases. The corrected desktop
status spec passed 2/2; the adjacent cases passed in the combined run. Phone
combined run passed 8/8. Exact commands and initial test corrections are recorded
in the work orders.

Focused unit tests: 3 files, 40 passed. Typecheck, scoped ESLint, seven-language
completeness/ratchet and whitespace passed. Public-document validation: 62 tests
and 47 published pages passed. Catalog validation: 349 decisions and 1336
specifications; all-spec lint passed. Native test artifacts were rebuilt after
locale generators refreshed embedded backend timestamps; no cross-platform
artifacts or preview backend restart were needed for the UX.

Live checks cover named multi-repository origins, actual HTTP exclusion,
honest Delivered/Active explanations, save 200 and reload persistence, pending
changes during a 12-second turn followed by Applied in the same session, one
390px phone drawer with a 44px close target and zero horizontal overflow, and
light/dark screenshot inspection. The disabled/outage case retains its disabled
task selection and local endpoint 503; user start remains manual. All four
selection scopes match the before-deployment snapshot. Main 9998 and owned
preview process identities remain unchanged. Runtime evidence is recorded in
`/tmp/kandev-pr3293-test-runtime/ux-rehearsal.json`.

Controlled application reports cover pending/deferred/failed/applied copy;
they do not claim actual provider failures. After mock reconnect, built-in
Kandev can return to Delivered because the mock reuses its MCP client without
fresh tools/list evidence. The browser test checks that recorded state rather
than manufacturing Active. No task exclusions, delegation, commits or pushes.

Design checkpoint checks passed before implementation. The package remains
uncommitted along with the previously authorized repairs.

## Post-delivery tooltip alignment repair

Task 2 exposed a wrapped status beginning in the dot column rather than the
server text column. The tooltip now uses a separate dot column; server name,
status, explanation and optional summary share the same wrapping text column.
No status meanings, selection behavior or user-facing copy changed.

The initial regression failed against the previous build. Final geometry checks
target the visible tooltip content rather than Radix's accessibility copy.
Desktop explorer/status checks passed 2/2; phone checks passed 2/2. The final
visible-content regression rerun passed 1/1. Scoped ESLint, formatting, the web
build and whitespace checks passed. Live task 2 at a constrained 240px content
width showed identical left edges for name, wrapped status and explanation;
a temporary reconstruction of the old visible row reproduced the 16px error.

The frontend was redeployed to owned preview 48595, retaining existing hashed
assets for open tabs. Task 2's selection and main/preview process identities
were preserved. Evidence: `alignment-deployment.json`, `alignment-after-visible.png`,
`alignment-visible-desktop-e2e.log` and `alignment-phone-e2e.log` under
`/tmp/kandev-pr3293-test-runtime`.

## Dialog hierarchy revision

The post-delivery screenshot still made `From task setup` resemble a server
entry. This is a visual repair within AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.14
and AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.17, preserving the accepted behavior.
The existing implementation and delivery work orders cover this revision.

Section titles sit outside list surfaces. Both use the same uppercase,
letter-spaced heading treatment, descriptive secondary copy and consistent
spacing. The count sits beside its title. Rows are inset within bounded lists;
names use 13px body typography and origins remain secondary, wrapping metadata.
Applied is a compact neutral headline with a green check. Pending, deferred and
failure retain the existing colored consequence notice and retry. The dialog
title is larger than body text, tabs have one full-width separator, and the
Customize column has a readable maximum width. Connections use the same small
section-heading treatment and icons align with the first line of each row.

```text
MCP servers                                      [X]
Review MCP connections, tools, and session additions.
[Connections] [Customize]
----------------------------------------------------
    (check) Setup up to date
            Saved setup applied. Connections shows health.
    ------------------------------------------------
    FROM TASK SETUP  [3]
    Selected by profile, repositories or task.
    +----------------------------------------------+
    | Atlas shared documentation                   |
    | Profile: Atlas tools   Repository: Atlas API  |
    |----------------------------------------------|
    | Atlas review notes                           |
    | Task: Inheritance                            |
    +----------------------------------------------+

    ADDITIONAL SERVERS
    Choose extra servers for this session.
    [Search MCP servers____________________________]
    +----------------------------------------------+
    | [x] Session extras                           |
    |     session-extras                           |
    +----------------------------------------------+
```

Phone uses the existing full-height MCP drawer exemplar: fixed title/tabs,
one body scroller, safe-area clearance, wrapping names/origins and 44px touch
controls. It uses the same heading/list distinction and status meanings, with
the column filling its available width. No nested overlay or extra conversation
bar. Check the real three-origin case in light/dark desktop/phone screenshots.
The shared desktop/phone regression checks matching external heading treatment,
row text indentation and the absence of a heading-enclosing card surface.

Revision delivered on 2026-10-07. The desktop hierarchy regression failed on the
previous build before the presentation changes. Final desktop explorer/status:
2/2 passed; phone: 2/2 passed. Focused unit checks: 40/40 passed after the row-list
extraction. Typecheck, warning-free scoped ESLint, Prettier, web build and
whitespace checks passed. No new copy or locale changes were needed.

Owned preview 48595 serves the new assets. Real case 01 screenshots in both
themes show external section titles, inset lists, wrapped three-origin metadata
and the quieter Applied summary. At 390x844 the phone has one drawer, a 44px
close target, one scrolling body and no horizontal document overflow. Additional
choices remain reachable by scrolling. All four persisted selection scopes and
main/preview process start identities match before deployment. Old hashed assets
were retained for open tabs. No selection write, backend restart, commit, push
or inherited disabling was performed. Evidence: `hierarchy-deployment.json`,
`hierarchy-rehearsal.json`, `hierarchy-desktop-e2e.log`, `hierarchy-phone-e2e.log`
and `hierarchy-{desktop,phone}-{light,dark}.png` under
`/tmp/kandev-pr3293-test-runtime`.

## Inherited server metadata badges

Continue AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.14 within the reviewed UX
package. Each inherited row now separates the server name and optional
description, neutral setup/transport/catalog badges, and named origin badges.
Profile, repository, task and session use distinct icons and restrained accents.
Every contributing source is retained. Badges wrap rather than hiding long
owner names in a hover, and unknown names keep the existing scope-only fallback.
Existing definition fields are configuration evidence, not live health or a
human editor audit trail. Keep inherited selections read-only, with all API,
polling and save timing contracts unchanged. No runtime alias, endpoint URL,
executable arguments or secret bindings are added to the primary summary.

```text
FROM TASK SETUP [3]
Selected by your profile, repositories or task.
+---------------------------------------------------+
| Atlas shared documentation                        |
| Shared guides for the infrastructure team.         |
| [Remote endpoint] [Server-sent events] [Custom]     |
| [Profile: Atlas tools] [Repository: Atlas API]      |
| [Task: Inheritance across two repositories]        |
+---------------------------------------------------+
```

Phone retains the existing full-height drawer, fixed header/tabs, safe areas,
one scroller and 44px controls. The same grouped metadata and source chips wrap
to its narrower width. Badges are static labels, not tiny interactive controls.
The source accents do not reuse selection-application status meaning.
Both work orders cover the metadata presentation and its delivery evidence.
TDD covers setup label mapping, HTTP alias and legacy-source attribution; the
shared desktop/phone test uses a real definition inherited through three scopes,
checks resolved names and description, and measures badge row containment.

Delivered on 2026-10-08. Metadata/origin unit checks: 27 passed. Desktop
explorer/status: 2/2 passed; final phone run: 2/2 passed, including a 260px
content-column stress check proving long source names wrap to multiple lines
without losing text. The initial browser RED exposed the missing description.
A fixture title exceeded the 60-character API limit and was shortened; a
multi-locator text assertion was corrected to target its single row. Neither
correction changed production selection or status behavior.

Fresh web build, typecheck, warning-free scoped ESLint, formatting, i18n
new-code ratchet and whitespace passed. Existing translation keys were reused;
no locale generation or native rebuild was needed. Public-doc validation:
62 tests and 47 pages passed. Catalog: 349 decisions and 1336 specifications;
all-spec lint passed after keeping the design's existing size limit.

Live cases 01/02 show the real remote/SSE and executable/stdio metadata, all
named origins, and supplied descriptions. Desktop/phone screenshots were
inspected in light/dark themes. Phone: one 390x844 drawer, 44px close target,
badge containment and no horizontal overflow. All four persisted selection
scopes and main/preview process start identities were preserved. Frontend-only
deployment on 48595 retains old hashed assets for open tabs. No preview data
writes, backend restart, inherited exclusion, delegation, commit or push.
Evidence: `badges-deployment.json`, `badges-rehearsal.json`, `badges-unit-tests.log`,
`badges-{desktop,phone}-e2e.log`, `badges-{desktop,phone}-{light,dark}.png`, and
`badges-task-02-desktop.png` under `/tmp/kandev-pr3293-test-runtime`.

## Risks

- A reason from a filtered alternate transport can survive a later Active state.
- Direct third-party connection health is unobservable in the current contract.
- The mock accepts some configurations without launching their executables.
- The saved state does not include a full authoritative previous selection diff.
- Long translated reasons must fit the existing phone drawer and desktop hover.

## Customize scroll and origin-heading repair

Continue the reviewed presentation contract. A coarse-pointer tablet drawer
uses an intrinsic height while constraining its maximum height. Its descendant
percentage-height scroll area expands past the visible drawer instead of
scrolling. Desktop wheel scrolling works at 1280x900 and 900x500; the failure
was reproduced at 820x900 with a touch pointer. Fix the tablet height while
retaining the full-height phone and desktop dialog composition. Header and tabs
stay fixed; Customize owns the single bounded vertical scroller, clears safe
areas, and exposes every final choice to wheel and touch input. Reuse the
existing MCP drawer exemplar and shared state/filter/selection logic.

Each inherited server has a translated **Where it comes from** subheading
above all named profile/repository/task/session badges. These are contributing
configuration scopes, not human owners or connection-health evidence. Keep
setup/transport/catalog badges above this source section.

```text
FROM TASK SETUP [3]
+-------------------------------------------+
| Atlas shared documentation                |
| Shared infrastructure guides.             |
| [Remote endpoint] [SSE] [Custom]           |
| Where it comes from                       |
| [Profile: Atlas tools] [Task: Review]      |
| [Repository: Atlas API]                   |
+-------------------------------------------+
ADDITIONAL SERVERS
[Search servers]
(scrollable choices through the final row)
```

Phone and touch tablet retain a fixed header/tabs and one internal body
scroller. The phone remains full height; the tablet uses its existing 80dvh
height budget. Test the actual last choice inside scroller bounds after a
swipe, not only DOM visibility or a programmatic scrollTop assignment.
Both existing work orders cover this repair and its delivery.

Repair delivered on 2026-10-08. Tablet behavioral RED reproduced a zero scroll
range with the content clipped below the drawer. A definite 80dvh touch-drawer
height fixes percentage-height resolution while preserving phone 100dvh,
desktop composition, safe areas and header/tab placement. Named origin badges
now have a translated source heading and subtle separator in each row. All
seven shipped languages and generated Traditional Chinese/pseudo catalogs
include that heading; unrelated catalog bytes and native mtimes were preserved.

Final browser journeys: desktop mixed status and touch tablet, 2/2 passed;
phone explorer and mixed status, 2/2 passed. The desktop tool-drill journey also
passed on this production build. Long-list fixtures include 11 additional
choices. The first desktop last-choice assertion incorrectly assumed display
name sort rather than runtime name sort; zero-padded fixture runtime names
correct that assumption. Wheel events now wait for actual movement between
inputs. No further desktop production layout change was required.

Typecheck, warning-free scoped ESLint, formatting, full i18n completeness and
new-code ratchet, fresh web build and whitespace checks passed. Public docs:
62 tests and 47 pages. Catalog: 349 decisions/1336 specs; all-spec lint passed.
Live preview touch rehearsal at 820x900 and 390x844 reaches the last checkbox
inside the scroller, keeps tabs fixed and the close target 44px, and has no
horizontal document overflow. Light/dark screenshots were inspected.

Frontend-only preview deployment on 48595 retains existing hashed assets.
All four saved selection scopes and main/preview process start identities
match both before/after deployment and after the live rehearsal. No backend
restart, preview data write, inherited exclusion, commit or push. Evidence
under `/tmp/kandev-pr3293-test-runtime`: `scroll-deployment.json`,
`scroll-rehearsal.json`, `scroll-tablet-red.log`,
`scroll-desktop-final-e2e.log`, `scroll-phone-e2e.log`,
`scroll-after-{tablet,phone}-{light,dark,bottom}.png`,
`scroll-after-desktop.png`, and check/build logs with the `scroll-` prefix.

## Advanced settings MCP row polish

Presentation repair within AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.4/.6/.8.
The generic full-width MCP disclosure does not match the existing dependency
and priority label/control/help rows. Replace only its task-create caller with
a compact count selector and adjacent info control. Reuse shared selection
content/state. Fine-pointer desktop opens a bounded popover within the parent
dialog's portal container; phone/coarse pointer opens the existing inset drawer
pattern. Help uses the priority tooltip/touch-drawer pattern. No permanent
helper paragraph, inherited exclusion, or change to draft submission semantics.

```text
ADVANCED SETTINGS
Depends on (i) [No dependency v]     Priority (i) [Medium v]
MCP servers (i) [0 selected v]

Phone: same compact labelled row, 44px controls.
Tap MCP selector -> inset drawer with fixed title/close, bounded scrolling
inherited summaries and searchable task additions; selection survives closing.
```

Desktop controls measure 28px; touch controls at least 44px. Preserve inherited
read-only rows and task-level persistence. The current priority field and
MobilePickerSheet are the visual exemplars; use the existing Drawer primitive
with one safe-area-aware body scroller. Profile/repository/session pickers retain
their current appearance. [Task 03](task-03-advanced-mcp-row.md) owns the repair,
rendered checks and frontend-only deployment to 48595.

Task 03 delivered on 2026-10-09. The desktop width regression was reproduced
before implementation; final desktop and phone task-creation journeys pass,
one each, plus nine existing Advanced/priority unit tests. Rendered controls
match Priority at 128x28 desktop and 128x44 phone. Help moved behind the info
control, loading retains that size, and inherited summaries/task additions open
in a bounded desktop popover or a touch drawer with a fixed title/close and one
body scroller. Search, keyboard/touch selection, dismissal, focus return, draft
retention and task persistence remain intact. Existing localization, typecheck,
lint, build and docs checks passed. Frontend-only deployment and read-only live
rehearsals preserve all four selection scopes and main/preview process
identities. See Task 03 for evidence paths and detailed validation.


## Proposed origin-section continuation (2026-10-14)

The [Customize origin sections package](../mcp-customize-origins/plan.md)
proposes replacing the inherited origin-chip presentation and enriching session
choice rows. This record's delivered status, historical previews and checks
remain unchanged. The continuation is not implemented; its work order owns the
new composition and regression evidence. Application and pending-change
semantics from this delivery remain in force.
