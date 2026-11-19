---
created: 2026-11-16
status: implemented
requirements:
  - REQ-OFFICE-REVIEW-AUTHOR-EXCL-001
  - REQ-OFFICE-REVIEW-AUTHOR-EXCL-002
  - REQ-OFFICE-REVIEW-AUTHOR-EXCL-003
  - REQ-OFFICE-REVIEW-AUTHOR-EXCL-004
  - REQ-OFFICE-REVIEW-AUTHOR-EXCL-005
  - REQ-OFFICE-REVIEW-AUTHOR-EXCL-006
system_design:
  - ../../specs/office/system-design/review-author-provider-exclusion.md
legacy_specs: []
---

# Implementation Plan: Keep Office Reviewers off the Author's Provider

## Overview

Issue #5349. Add an opt-in `exclude_author_provider` setting to
`ensure_participant_seat`. Seat runs at a declaring step then never launch on
the provider that did the task's work. If excluding the author leaves no
candidate, the run parks with a `skipped_author_provider` route attempt, and
every resolution writes a `routing.author_exclusion` run event.

Because this is the first release of the setting, it also lands with
non-blocking setup warnings (REQ-OFFICE-REVIEW-AUTHOR-EXCL-005). Import preview
and result, workflow sync, and the provider routing settings page warn when the
current setup means a declaring step can park reviews (the workspace or a
seat-pool agent has only one provider, so reviews wait whenever the work ran
on it) or will not enforce the exclusion (routing off).

The policy is evaluated in `DispatchWithRouting` on each resolution. Nothing
is persisted on seats or runs, and no migration is needed. See the
[system design](../../specs/office/system-design/review-author-provider-exclusion.md).

## Delivery order

| Wave | Work order | Result |
|---|---|---|
| 1 | [task-01-dispatch-author-exclusion](task-01-dispatch-author-exclusion.md) | Backend: config parse, repo reads, resolver skip, dispatcher wiring, run events |
| 2 | [task-02-route-outcome-label](task-02-route-outcome-label.md) | Web: localized `skipped_author_provider` label, plus an E2E proving a reviewer run lands on a non-author provider |

| 2 | [task-03-exclusion-setup-warnings-backend](task-03-exclusion-setup-warnings-backend.md) | Backend: setup evaluator, routing GET/PUT warnings, import preview/result warnings via a workflow-service port, sync warnings |
| 3 | [task-04-exclusion-setup-warnings-web](task-04-exclusion-setup-warnings-web.md) | Web: routing settings banner, import dialog warnings, locales, desktop and phone E2E |

| 4 | [task-05-routing-control-backend](task-05-routing-control-backend.md) | Backend: list seat steps on routing GET, batch write endpoint through the workflow service, step events, three-part sync warning text |
| 5 | [task-06-routing-control-web](task-06-routing-control-web.md) | Web: Review on a different provider section in the routing draft, three-part warning copy, import dialog submitting state, locales, desktop and phone E2E |

| 6 | [task-07-grouped-warnings-save-bar](task-07-grouped-warnings-save-bar.md) | Backend and web: group warnings that share a cause and fix; floating settings save bar on the routing page |
| 7 | [task-08-seat-write-integrity](task-08-seat-write-integrity.md) | Backend and web: all-or-nothing, version-guarded switch saves; release parked runs when a switch turns off; author skip on exhausted fallback; no stale switches after a workspace change |
| 8 | [task-09-maintainer-review-scope-and-safety](task-09-maintainer-review-scope-and-safety.md) | Backend and web: legacy-routing scope with a bound-profile warning; manage-only switch endpoint; release only author-exclusion parks; record the replayed work provider; report a committed save as saved |

Task 02 depends on Task 01, because its E2E needs the backend behavior. Task 03
depends on Task 01 (the shared declaration helper). Task 04 depends on Task 03.
Tasks 02 and 03 can run in parallel. Tasks 05 and 06 come from the first
manual test round: operators need to turn the setting off without editing
YAML, and the warnings must say what is true, why it is bad, and how to fix
it. Task 06 depends on Task 05. Task 07 comes from the second manual test
round: many steps with the same warning must read as one warning with a list
of steps, and unsaved routing changes must be hard to miss. It depends on
Task 06. Task 08 comes from PR review: switch saves must be all-or-nothing
and must not lose a concurrent change, turning a switch off must release
reviews it parked, and every park must record the author skip. It depends on
Task 07.
Task 09 comes from maintainer review: the setting is scoped to agents on
workspace provider routing until the Office routing handoff (dynamic-agent-routing
task 10) carries it into the shared resolver, the switch endpoint needs
workspace-manage authority, the release must not touch session-ceiling holds,
a replayed work launch must record its provider, and a committed save must not
read as a failure. It depends on Task 08.

## ASCII UI preview

### UI-01: Run detail, Route attempts panel (parked seat run)

Entry point: Office → Agents → reviewer agent → Runs → a parked review run.
Only the outcome pill is new. The layout is unchanged and is the same on phone
(the row wraps).

```text
+-- Route attempts -----------------------------------------------+
| Provider candidates tried for this run                          |
|-----------------------------------------------------------------|
| #1 (!) claude-acp/sonnet  [balanced] [skipped: wrote this task] |
| #2 (!) codex-acp/gpt      [balanced] [skipped degraded]  ...    |
+-----------------------------------------------------------------+
+-- Events -------------------------------------------------------+
| routing.author_exclusion  <ts>  {"applied":true,                |
|                                  "author_provider_id":"claude-acp"}
+-----------------------------------------------------------------+
```

Required: the new outcome gets its own localized label with the `outline`
variant. Illustrative: the label wording and the payload formatting (the
Events log already renders payloads verbatim). Maps to
AC-OFFICE-REVIEW-AUTHOR-EXCL-003.2 and AC-OFFICE-REVIEW-AUTHOR-EXCL-004.1.

### UI-02: Provider routing settings, exclusion setup banner

Entry point: Office → Workspace → Provider routing. State: one declaring step
predicts a halt. The banner sits above the enable card and is absent when there
is nothing to warn about.

```text
Desktop
+-----------------------------------------------------------------+
| Provider routing                                                |
| Map Office agents to providers and ...                          |
| +-------------------------------------------------------------+ |
| | /!\ Reviewer exclusion setup                                 | |
| |  - Office Review / Review (reviewer): only claude-acp is     | |
| |    configured, so review runs wait whenever the work ran on  | |
| |    claude-acp.                                               | |
| |  - Office Review / Review (reviewer): agent "QA" can only    | |
| |    use codex-acp, so its review runs wait whenever the work  | |
| |    ran on codex-acp.                                         | |
| +-------------------------------------------------------------+ |
| [Routing enabled  (o)]                                          |
| ...                                                             |
+-----------------------------------------------------------------+

Phone (same order; text wraps, no horizontal scroll)
+---------------------------+
| Provider routing          |
| +-----------------------+ |
| | /!\ Reviewer exclusion | |
| |    setup              | |
| | - Office Review /     | |
| |   Review (reviewer):  | |
| |   only claude-acp is  | |
| |   configured, so      | |
| |   review runs wait    | |
| |   whenever the work   | |
| |   ran on claude-acp.  | |
| +-----------------------+ |
| [Routing enabled (o)]     |
+---------------------------+
```

When routing is off, the item reads "routing is off, so the exclusion is not
enforced." Required: placement above the enable card, one item per warning,
naming workflow, step, role, the one provider (and agent), and no horizontal
scroll on phone.
Illustrative: the wording. Maps to AC-OFFICE-REVIEW-AUTHOR-EXCL-005.5 to .7.

### UI-04: Provider routing settings, Review on a different provider

Entry point: Office → Workspace → Provider routing. The section sits under the
provider order and is part of the page draft, so the existing Save applies it.
Until issue #5347 ships an Office workflow editor, this is the control's home.

```text
Desktop
+-- Review on a different provider -------------------------------+
| Keep a step's reviewer or approver off the provider that did    |
| the task's work, so the review is an independent check. If no   |
| other provider is available, the review waits.                  |
|-----------------------------------------------------------------|
| Office Default workflow, Review step     [Reviewer]      ( o)   |
| Office Default workflow, Approval step   [Approver]      (o )   |
+-----------------------------------------------------------------+
                                                         [Save]

Phone (rows wrap; 44px switch targets)
+---------------------------+
| Review on a different     |
| provider                  |
| Keep a step's reviewer... |
|---------------------------|
| Office Default workflow,  |
| Review step               |
| [Reviewer]          ( o)  |
|---------------------------|
| Office Default workflow,  |
| Approval step             |
| [Approver]          (o )  |
+---------------------------+
```

Empty state: "No workflow in this workspace has a reviewer or approver step."
Required: one row per seat action, workflow and step named by type, role
badge, switch in the page draft. Illustrative: the description wording. Maps
to AC-OFFICE-REVIEW-AUTHOR-EXCL-006.1, .5 and .6.

Revised warning item (replaces the UI-02 and UI-03 wording, per
AC-OFFICE-REVIEW-AUTHOR-EXCL-005.9):

```text
| /!\ Review on a different provider can't work as set up          |
|  - Office Default workflow, Review step (Reviewer): Claude (ACP) |
|    is the only provider, so the reviewer can't use a different   |
|    one from the work. Reviews will wait until you act. Add a     |
|    second provider, or turn off Review on a different provider   |
|    for this step.                                                |
```

Grouped warning (replaces the per-step items above, per
AC-OFFICE-REVIEW-AUTHOR-EXCL-005.10):

```text
Desktop
+-----------------------------------------------------------------+
| /!\ Review on a different provider can't work as set up         |
|  Claude (ACP) is the only provider, so reviews can never run on |
|  a different one from the work. Reviews will wait until you act.|
|   - Exclusion Check workflow, Review step (Reviewer)            |
|   - Office Default workflow, Review step (Reviewer)             |
|   - Probe A workflow, Review step (Reviewer)                    |
|   [Show all 6 steps]                                            |
|  Add a second provider, or turn off Review on a different       |
|  provider for these steps.                                      |
+-----------------------------------------------------------------+

Phone (same order; text wraps)
+---------------------------+
| /!\ Review on a different |
|    provider can't work as |
|    set up                 |
| Claude (ACP) is the only  |
| provider, so reviews ...  |
|  - Exclusion Check        |
|    workflow, Review step  |
|    (Reviewer)             |
|  [Show all 6 steps]       |
| Add a second provider ... |
+---------------------------+
```

### UI-05: Provider routing settings, floating save bar

Same bar as Office > Workspace > Settings. It appears while the page has
unsaved changes and stays above the page bottom while scrolling.

```text
+-----------------------------------------------------------------+
| ...page content scrolls under the bar...                        |
|        +-----------------------------------------------+        |
|        | Unsaved changes            [Discard]  [Save]  |        |
|        +-----------------------------------------------+        |
+-----------------------------------------------------------------+
```

Required: shared `SettingsFloatingSave`, no separate bottom Save button,
leave-page guard. Maps to AC-OFFICE-REVIEW-AUTHOR-EXCL-006.7.

### UI-03: Workflow import dialog, preview step

Entry point: Settings → Workspace → Workflows → Import. State: the previewed
document declares the exclusion and the setup predicts a halt.

```text
+-- Import workflows ------------------------------------------+
| (existing profile-selection content)                          |
| +----------------------------------------------------------+ |
| | /!\ Reviewer exclusion setup                              | |
| |  - Office Review / Review (reviewer): routing is off, so  | |
| |    the exclusion is not enforced.                         | |
| +----------------------------------------------------------+ |
|                                    [Cancel]  [Import]         |
+---------------------------------------------------------------+
```

Import stays enabled. Phone: the dialog already stacks full-width, and the
list wraps the same way. Maps to AC-OFFICE-REVIEW-AUTHOR-EXCL-005.3 and .7.

## Risks

- **Routing disabled.** A seat run is not excluded when workspace routing is
  disabled (AC-OFFICE-REVIEW-AUTHOR-EXCL-002.5). It is recorded, not blocked.
  With routing disabled, the run launches on the agent's own execution profile
  (or on only the first provider in the order), so there is no alternative to
  move to, and blocking would park every such review.
- **Single-provider routing.** Routing can be enabled with one provider in the
  order, with a different model per tier. Once the setting is on, every seat
  run in such a workspace parks as requiring operator action
  (AC-OFFICE-REVIEW-AUTHOR-EXCL-003.4), because the comparison is by provider,
  not by model.
- **Author lookup scans by JSON payload.** `GetTaskAuthorProvider` filters
  `runs` with `dialect.JSONExtract` on `task_id`, which matches the existing
  `runs_inflight.go` predicate. It runs only for seat runs at declaring steps,
  so its cost stays off every other dispatch.
- **Legacy work runs.** Work-stage runs queued before payloads carried
  `workflow_step_id` record no author. They fall under
  AC-OFFICE-REVIEW-AUTHOR-EXCL-004.2.
- **Same provider, different model** counts as the same author, by design.
- **Warnings are a snapshot.** Provider health and later settings changes are
  not predicted. Runtime parking with the `skipped_author_provider` outcome
  remains the backstop.
- **Two writes per save.** The routing page saves routing first and the
  switches second. If the second request fails, routing stays saved and the
  switch changes stay in the draft with an error toast. Each request is
  validated before it writes.
- **Hidden Office workflow.** The built-in Office workflow is hidden from the
  workflow settings list, so the routing page is the only UI for its seat
  steps until #5347.
- **Seat-pool helper move.** Moving `seatRolePools` into `office/models` touches
  the seat caster. Its existing tests must pass unchanged.

## Verification strategy

- Go unit tests in `routing`, `scheduler`, `office/repository/sqlite`
  (SQLite, plus PostgreSQL when the fixture DSN is available), `workflow/engine`,
  `workflow/models`, and `config/workflows`.
- Web Vitest for the route-panel label, plus the i18n checks.
- One Playwright spec in the routing harness (mock providers) for the
  end-to-end routing evidence, plus desktop and phone specs for the routing
  banner and the import-dialog warning.
- Finish with `make fmt`, then `make typecheck test lint`.
