---
created: 2026-10-10
status: done
requirements:
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-005
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-006
system_design:
  - ../../specs/agents/system-design/workspace-mcp-pending-changes.md
  - ../../specs/agents/system-design/workspace-mcp-configuration.md
legacy_specs: []
---

# Implementation Plan: Explain exact pending MCP changes

## Overview and assumptions

Make Customize answer which saved servers will change and when. First provide
an authoritative accepted-versus-saved comparison; then render its actions next
to the existing application explanation and selection rows.

Confirmed: the user cannot identify what remains to apply; retain the plug,
Customize, current automatic saving and previous inherited read-only decision.
At planning: revision/attempt metadata was persisted, accepted membership was not,
and frontend attachment rows lack definition identities. Agent design choices:
compact pending-action list, inline row annotation and a safe backend snapshot.
The user explicitly requested implementation after reviewing this package.
No delegated work is authorized.

Agents owns selection/application truth and its UI outcome; Platform retains
connection/tool evidence. The existing observability ADR already establishes
that boundary. A separate ADR is unnecessary: this snapshot extends the same
desired/applied invariant. The companion designs preserve rationale for rejecting
browser-open baselines and name/health comparisons.

## Scope and technical approach

Task 01 extends the existing state table with a nullable safe accepted snapshot,
captures exact request descriptors through lifecycle start/reconfiguration, and
projects a revision-consistent comparison on session selection responses.
Apply failures and overlapping saves preserve accepted authority. Old databases
remain comparison-unavailable until a successful captured application.

Task 02 extends the shared Customize composition and row presentation. A known
change list includes removals independent of checked/search/catalog membership.
Known-empty, unknown, saving, save rejection, failed apply, deferred and applied
are distinct states. Translate all copy, keep query/scroll/focus stable, and
update public docs after implementation. Do not alter the current Connections
surface or use health as membership evidence.

No inherited disabling, new Apply/Save button, new agent start action, remote
marketplace change, connection probing, full configuration persistence, forced
reset or main-instance mutation. Existing completed status-clarity and Advanced
row packages remain done; this is a dependent extension, not their reopening.

## Compatibility

| Path | Capture/application | Evidence |
| --- | --- | --- |
| ACP with resume/load | Accepted request snapshot; current idle gate | Lifecycle success/failure/race tests |
| ACP without reconnect | Keep baseline and queued changes until next normal start | Deferred/restart test |
| Passthrough | Accepted normal-start snapshot; no live replacement | Existing start boundary test |
| Legacy session without snapshot | Unavailable comparison, no invented changes | Store migration and UI fallback |
| Filtered transport/disabled definition | Accepted intent can include excluded definitions; health stays separate | Projection and mixed-state journey |

## ASCII UI preview

UI-01: Current source-backed composition, plug > Customize, pending state.

```text
Changes saved, waiting to apply
Timing explanation

FROM TASK SETUP
Inherited rows...
ADDITIONAL SERVERS
[x] Atlas docs
[ ] Issue tracker
```

UI-02: Proposed desktop Customize, known comparison; one existing body scroller.

```text
MCP servers                                  [Close]
Connections | Customize
---------------------------------------------------
Changes waiting to apply                       [2]
Your agent keeps its previous setup until idle.
  [+] Atlas docs                  Add to setup
  [-] Issue tracker               Remove from setup

FROM TASK SETUP
Inherited rows and named source badges...
ADDITIONAL SERVERS
[Search servers]
[x] Atlas docs                    Add to setup
[ ] Issue tracker                 Remove from setup
    In the last accepted setup until this applies.
```

UI-03: Proposed phone/coarse-pointer Customize, same shared comparison.

```text
MCP servers                   [Close]
Connections | Customize
------------------------------------
Changes waiting to apply [2]
Keeps previous setup until idle.
[+] Atlas docs
    Add to setup
[-] Issue tracker
    Remove from setup

FROM TASK SETUP
Named inherited sources...
ADDITIONAL SERVERS
[Search servers]
[x] Atlas docs     Add to setup
[ ] Issue tracker
    Remove from setup
```

Header/views fixed, one safe-area-aware bounded body scroller; no added drawer,
bar or sticky summary consuming list space. Action/timing words and icons are
required; counts/colors/spacing are illustrative and use existing tokens.
Phones retain 44px controls and wrapping. Nearest exemplar: existing MCP explorer.

UI-04: Supporting states in the same position on both viewports.

```text
Next start: Saved for the next agent start + named actions
Apply failed: Changes could not be applied + named actions + [Retry]
Unavailable: Saved choices shown. Exact changes cannot be compared yet.
Known empty, pending: Saved choices match the last accepted setup.
Applied: Setup up to date. No pending actions or row annotations.
Saving/rejected save: existing saving/error UI; committed comparison retained.
```

## Tests and E2E

AC-006.17/.18: snapshot migration/store, handler guards and projection,
resolver duplicate inheritance, lifecycle accepted revision/failure/race tests.
AC-006.15/.16/.18: shared presenter tests for add/remove/update, unknown/empty,
deleted names, search independence and unchanged inherited union.
AC-006.15-.19: desktop and phone pending-change browser journeys with explicit
controlled configuration evidence, long names/lists, save failure versus apply
failure, reopening/reload and sibling sessions. Include supported idle apply
and deferred start paths without claiming fixtures prove server connectivity.
Exact commands and owned files are in each work order.

## Work orders

1. [Task 01: Persist and project accepted setup](task-01-accepted-setup.md).
2. [Task 02: Show exact changes in Customize](task-02-customize-changes.md).

Execute sequentially in the primary session after an explicit implementation
request. No delegated work is authorized.

## Verification results

Design validation passed on 2026-10-10: catalog validated 349 decisions and
1,337 specifications, all-spec lint passed, and whitespace checks passed.
The first lint run caught the existing requirement file's 20 KiB limit; concise
criteria preserve the full contract within that limit. Implementation completed after the user's explicit follow-up request. All five
focused backend packages and three race packages passed; final changed-package
Go lint reported zero issues. All 15 focused frontend unit tests, scoped ESLint,
TypeScript, localization checks and production native/web builds passed.
Desktop and phone managed browser journeys passed sequentially, including real
idle acceptance clearing the open view. The work orders record exact commands,
harness corrections and their limits.

Owned preview updated on 48595 after a fresh DB backup and migration rehearsal.
All four saved-selection tables were unchanged by the deployment; new task 06
was seeded independently, preserving every pre-existing selection row. Dark,
light and phone presentation was verified and visually inspected. Main :9998
process/start identity remained unchanged. Public validators passed 62 tests
and 47 pages; specification catalog and all-spec lint passed. No commit/push,
merge or delegation was performed.

## Risks

Initial/restart producer coverage and exact revision capture are the main
risks. Returning a successful saved write must not depend on comparison reads.
Unknown old snapshots require honest fallback, not inferred empty membership.
Catalog and scope edits must not be advertised as live-applied without the
existing session lifecycle scheduling. Preserve last accepted descriptors for
removed definitions and keep configuration values outside storage/projection.


## Proposed origin-section continuation (2026-10-14)

The [Customize origin sections package](../mcp-customize-origins/plan.md)
proposes replacing the inherited origin-chip presentation and enriching session
choice rows. This record's delivered status, historical previews and checks
remain unchanged. The continuation is not implemented; its work order owns the
new composition and regression evidence. Application and pending-change
semantics from this delivery remain in force.
