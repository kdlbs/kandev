---
created: 2026-10-05
status: complete
requirements:
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-003
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-005
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-006
system_design:
  - ../../specs/agents/system-design/workspace-mcp-configuration.md
legacy_specs: []
---

# Composer MCP Customization

## Overview

Replace the existing-session bar with customization in the composer plug's
existing explorer. First stabilize background refreshes, then integrate one
session controller with the dialog/drawer and pending indicator. Keep the
previous feedback package's readable names and correct rejection ownership.

Agents own this vertical contract because selections, application revisions,
and provider capabilities belong to agent configuration. Platform connection
evidence stays independent; its explorer remains reachable.

The user requested the entry-point change and a pending-idle icon or color.
Choose a clock badge on the neutral plug, a read-only hover summary, and click
to customize inside the existing dialog. These are reversible presentation
choices within the requested scope. No material user question remains.

## Scope

One composer entry point, stable background loading, readable inherited names,
shared desktop/phone behavior, and pending/deferred/failed feedback. Preserve
task/session creation controls, additive selection, rejection rollback, retry,
and the connection/tool explorer. No backend API or schema change, built-in
tool pruning, marketplace redesign, quick-chat selection feature, proxy change,
PR publication, or main-instance mutation.

## Evidence and root cause

Source: `useMCPSelectionEditor.reload` sets `loading` on every call.
`TaskSessionMCPSettings` refreshes pending-idle application state sequentially
after each response plus 500 ms. `SessionSelectionBody` replaces the mounted
choices whenever that loading flag is true. Repeated flashes therefore occur
while the application is pending, not merely because an accordion is opened.

A browser-only reproduction against the owned :48593 instance intercepted
selection GET responses to hold pending state, without selection/database
writes. Across 32 samples it observed choices in 22 samples and loading with
zero choices in 9. Eight selection responses arrived about 521-759 ms apart.
The real seeded session was deferred-next-start at inspection time; the trace
proves the pending lifecycle defect rather than claiming that seed was pending.
Evidence: `/tmp/kandev-pr3293-test-runtime/mcp-flicker-diagnosis.json`.

The plug already renders `McpServerExplorer`: desktop tooltip plus dialog,
phone/coarse-pointer drawer. Desktop currently places it among collapsible
toolbar items, so moving the only configuration entry there requires keeping
the plug visible in compact toolbars. `ChatInputToolbar` chooses one responsive
composition; its controller must outlive that branch so a viewport transition
does not reset selection ownership or start a second poller.

## Technical approach

Task 01 distinguishes first load from background refresh in the selection
editor and masks stale context snapshots. Audit the workspace-definition hook
for equivalent context leakage; an empty successful inventory is still ready.
Keep save/load invalidation and expose errors without discarding loaded data.

Task 02 extracts `TaskSessionMCPSettings` state into one composer-bound hook
above responsive toolbar branches. The plug receives this controller, shows
application markers, and extends its existing explorer with Connections and
Customize views. Reuse selection content and application notices without the
old disclosure. Keep query/view/navigation state local to the mounted context,
and close the overlay when shared API recovery handles an error.

Provider compatibility stays unchanged:

| Runtime capability | Saved change | Closed plug | Evidence |
| --- | --- | --- | --- |
| Safe reconnect after idle | Pending until backend completion | Clock while pending, clears when applied | Selection revisions and separate attachment report |
| Reconnect unsupported | Saved for next start | Next-start marker | Deferred notice; no Active claim |
| Reconnect fails | Prior applied revision remains | Failure marker | Sanitized failure and retry |
| PUT rejected | Prior desired/applied state remains | Prior marker retained | Local error or shared reload recovery |
| No valid task-session context | No customization writes | Existing connection viewer | Existing session-owned report |

No new ADR is needed: additive scope and session-owned evidence boundaries are
unchanged. The clock is a saved-selection marker, not a connection-health color.

## ASCII UI preview

### UI-01: Closed task composer

Current: `[Session MCP additions ...]` above the conversation, then the composer.
Proposed: no MCP bar above the transcript. Existing composer actions include:

```text
[plan] [plug] [attach] [context] ... [model] [send]
        +clock when changes await idle
```

Keep the plug visible in compact desktop chrome. Hover/focus is a read-only
status summary: `Changes waiting for idle` plus existing server evidence.

### UI-02: Desktop plug click

```text
+----------------------------------------------------------+
| MCP servers                                      [Close] |
| [Connections] [Customize]                                |
|----------------------------------------------------------|
| Changes waiting for idle. Current tools remain in use.    |
| Inherited                                                |
|   Team documentation     Profile: MCP sandbox agent       |
|   Repository tools       Repository: MCP sandbox repo     |
| Add to this session                                      |
| [Search MCP servers....................................] |
| [ ] Issue tracker                                        |
| [x] Session extras                                       |
+----------------------------------------------------------+
```

Connections retains its server/tool panes and drill-down. Customize has one
scrolling list. Open Customize initially when there are unapplied changes;
otherwise default to Connections. Switching views never creates another overlay.

### UI-03: Phone or coarse-pointer plug tap

```text
+---------------------------------------+
| MCP servers                   [Close] | fixed header
| [Connections] [Customize]             | fixed view control
|---------------------------------------|
| Changes waiting for idle              |
| Inherited                             |
| Team documentation                    |
|   Profile: MCP sandbox agent          |
| Add to this session                   |
| [Search MCP servers.................] |
| [ ] Issue tracker                     |
| [x] Session extras                    | one scroll body
+---------------------------------------+
```

Phone uses the explorer's existing full-height drawer; coarse-pointer tablets
use its bounded drawer. Apply 44px touch targets and safe-area padding. Preserve
focus return, Escape/back, keyboard search, and tool drill-down. No hover-only
action or nested customization drawer.

### UI-04: Refresh and error

First load: a loading notice inside Customize. Later refresh: keep rows mounted,
query/focus/scroll unchanged. Ordinary error: local selection error plus retained
rows and authoritative IDs. A pending notice remains until authoritative state
changes. Failed apply offers Retry. Handled stale-token failure closes the whole
overlay so the application reload control is visible, with no duplicate error.

Entry point, overlay ownership, stable loaded rows, and pending clock are required
by AC-006.9 through 006.12. Labels/spacing in sketches are illustrative and must
use translated design-system controls; final copy goes into every shipped locale.

## Tests

- `use-mcp-selection-editor.test.ts`: deferred initial/refresh loads, empty
  selection, refresh failure recovery, context switch, stale poll versus save.
- `use-mcp-workspace-settings.test.ts`: context masking and late catalog loads.
- Existing origin/client tests retain name isolation and recovery classification.
- New presentation helper tests cover pending/applied/deferred/failed markers
  and absent context; existing explorer tests preserve navigation/evidence.

## E2E tests

The task MCP desktop and phone suites cover AC-003.7/003.8, AC-005.1/005.5-005.7,
and AC-006.9-006.12: plug entry, union/names, stable focused rows through held GET
responses, closed pending-to-terminal markers, errors, and persisted selection.
Keep existing task-create Advanced coverage. The desktop/phone MCP status suites
retain connection and tool inspection. Toolbar-overflow covers the pinned plug.
Add a targeted coarse-pointer tablet case to the task suite, setting touch media
before navigation and asserting the bounded drawer and 44px plug/Close targets.

## Work orders

- [x] [Task 01: Stabilize refreshes](task-01-stabilize-selection-refresh.md)
- [x] [Task 02: Integrate composer customization](task-02-composer-customization.md), after 01

Execute sequentially in the primary session. These waves do not authorize agents.

## Verification results

The user authorized implementation in a later turn with "go for it". Both work
orders are complete. Final checks passed: 102 unit tests, 8 desktop/tablet and
6 phone browser cases (sequential, zero retries), typecheck, targeted ESLint,
localization, public documentation, specification catalog/lint, package-reference
preflight, and whitespace. Exact commands and evidence are in
[Task 02](task-02-composer-customization.md#inputs-and-results).

The owned :48593 sandbox now runs the tested artifacts (backend PID 2107007).
Its DB was backed up and existing selections retained. Live direct save/restore
returned 200; desktop/phone surfaces passed inspection. The external forwarder
still returns an ordinary 403 without false reload. Main :9998 remains PID
3009187 unchanged. Diagnostic browser closed. Source/tests remain uncommitted;
no push, PR mutation, or merge. PR CI/review and built-in pruning remain pending.

Historical design-checkpoint evidence:

Design gates passed: specification catalog (348 decisions, 1334 specifications),
all specification lint, 36 specification-linter tests, and whitespace checks.
The repository PR-documentation validator's local `validateCoverage` preflight
passed for both new work orders against the planned editor path, checking their
plan/requirement/acceptance/design references. This is package-reference evidence,
not a claim about current GitHub PR status. At that checkpoint the work orders were untracked and uncommitted. The
diagnostic browser was closed and its GET interception removed.

## Risks

- Responsive toolbar changes must retain one controller and timer.
- A late response must not restore old workspace/session data or a pre-save revision.
- Refresh failure and successful empty selections must not restart blocking loading.
- The external :38383 forwarder still injects bearer credentials. Successful
  save verification must use direct pod-local access; preserve the interlock.
- This package supersedes the old bar's composition only. Historical results
  in the original and feedback packages do not prove this new UI.


## Follow-up forwarding diagnosis (2026-10-06)

The user's renewed save-failure report confirmed that :38383 is Kandev's own
main-runtime tunnel. Its authenticated agentctl proxy passes a consumed control
credential to the nested test application. The earlier characterization as only
an external forwarding limitation is superseded. Existing direct browser/test
results remain historical; this application package did not repair that executor
boundary. See [the follow-up package](../mcp-forwarding-session-clarity/plan.md)
for control-credential containment and clearer session timing/error copy. No
main/test runtime restart or new production edit occurred in this diagnosis.
