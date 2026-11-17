---
status: active
system: office
created: 2026-11-16
owners:
  - kandev
---

# Office Review Author-Provider Exclusion Requirements

## Overview

An Office review is an independent check only when the reviewer runs on a
different provider from the author. Reviewer and approver seats are filled
from an agent-role pool, and each seated agent then resolves its provider
through normal routing: a tier, a provider order, and fallback. Fallback can
land the reviewer on the same provider that did the work, and nothing records
that it happened.

This capability adds an opt-in setting to the seat-ensuring step action. When
it is set, runs for that seat never launch on the provider that did the task's
work. If no other provider is available, the run waits instead of reviewing
its own work. The run detail shows whether the exclusion applied.

Because the setting can halt a review flow, the system also warns, without
blocking, when the workspace's current setup predicts that a declaring step
can park reviews (whenever the work ran on a reviewer's only provider) or
will not enforce the exclusion at all. The warnings appear
where the setting arrives (workflow import preview and result, workflow sync
result) and where the setup that causes them is changed (Office provider
routing settings).

The seat-ensuring action and its seating rules are owned by
[review-participant-seats.md](review-participant-seats.md). Provider routing
is owned by the agents system
([dynamic-agent-routing](../../agents/system-design/dynamic-agent-routing-01.md)).
This document owns only the author-independence contract for Office seat runs.

## Terminology

- **Seat-ensuring action**: the `ensure_participant_seat` step `on_enter`
  action defined by REQ-OFFICE-REVIEW-SEATS-001.
- **Declaring step**: a workflow step whose seat-ensuring action for a
  participant role sets `exclude_author_provider: true`.
- **Seat run**: an Office run that was queued at a declaring step, for a task,
  whose agent holds a seat on that task in the role the step declares the
  exclusion for. Runs queued at that step by its fan-out on entry and by its
  comment fan-out are seat runs.
- **Work-stage run**: an Office run for the task that was queued at a workflow
  step whose stage type is `work`.
- **Author provider**: the provider that the most recently launched work-stage
  run for the task launched on, read at the moment the seat run resolves its
  route. A provider is the agent CLI that routing launches, identified by its
  agent type ID (for example `claude-acp`, `codex-acp`, `opencode-acp`). It is
  not the model vendor and not the model. Two models run through the same CLI
  count as the same author. The same vendor's model run through two different
  CLIs counts as two authors.
- **Parked run**: a run held under a routing blocked status until a provider
  becomes eligible, as defined by provider routing.
- **Seat pool**: the Office agents eligible to fill a participant role's seat,
  as defined by AC-OFFICE-REVIEW-SEATS-002.1 and .2.
- **Exclusion setup warning**: a non-blocking, localized notice that names a
  declaring workflow step, its participant role, and a predicted problem.

## Requirements

### REQ-OFFICE-REVIEW-AUTHOR-EXCL-001: The exclusion is a seat-action setting

The seat-ensuring action shall accept an opt-in `exclude_author_provider`
setting that is preserved like any other action configuration.

#### Acceptance criteria

- **AC-OFFICE-REVIEW-AUTHOR-EXCL-001.1:** When a workflow template or an
  imported workflow declares `exclude_author_provider: true` on a seat-ensuring
  action, the system shall accept the workflow without error.
- **AC-OFFICE-REVIEW-AUTHOR-EXCL-001.2:** When a workflow carrying
  `exclude_author_provider` on a seat-ensuring action is exported and then
  imported, the imported action shall carry the same value.
- **AC-OFFICE-REVIEW-AUTHOR-EXCL-001.3:** When a seat-ensuring action omits the
  setting or sets it to `false`, its seat runs shall resolve and launch exactly
  as they do without this capability, with no added route attempts and no
  added run events.
- **AC-OFFICE-REVIEW-AUTHOR-EXCL-001.4:** The built-in `office-default`
  workflow shall not set the exclusion.
- **AC-OFFICE-REVIEW-AUTHOR-EXCL-001.5:** The setting shall not change which
  agent is seated; seating follows REQ-OFFICE-REVIEW-SEATS-002 unchanged.

### REQ-OFFICE-REVIEW-AUTHOR-EXCL-002: Seat runs never launch on the author provider

When routing is enabled for the workspace, a seat run shall never launch on
the author provider.

#### Acceptance criteria

- **AC-OFFICE-REVIEW-AUTHOR-EXCL-002.1:** When a seat run resolves its route and
  an author provider is recorded, the system shall not launch the run on the
  author provider, even when it is first in the agent's effective provider
  order.
- **AC-OFFICE-REVIEW-AUTHOR-EXCL-002.2:** When a seat run falls back after a
  launch failure, retries after a park, or is retried manually, every
  resolution shall exclude the author provider.
- **AC-OFFICE-REVIEW-AUTHOR-EXCL-002.3:** When the task returned to a work-stage
  step and a later work-stage run launched on a different provider, the next
  seat-run resolution shall exclude the provider of that later run and shall no
  longer exclude the earlier one.
- **AC-OFFICE-REVIEW-AUTHOR-EXCL-002.4:** The exclusion shall apply only to seat
  runs. A run for the same agent queued at a step that does not declare the
  exclusion, or for a role the step does not declare it for, shall be
  unaffected.
- **AC-OFFICE-REVIEW-AUTHOR-EXCL-002.5:** When workspace routing is disabled, a
  seat run shall launch through the existing non-routed path, and the run
  detail shall record that the exclusion did not apply because routing is
  disabled.

### REQ-OFFICE-REVIEW-AUTHOR-EXCL-003: No candidate parks the run

When excluding the author provider leaves no launchable provider, the seat run
shall wait rather than fall back to the author provider.

#### Acceptance criteria

- **AC-OFFICE-REVIEW-AUTHOR-EXCL-003.1:** When the author provider is the only
  provider left after exclusion and health filtering, the system shall park the
  seat run and shall not launch it.
- **AC-OFFICE-REVIEW-AUTHOR-EXCL-003.2:** When a seat run parks, its route-attempt
  history shall contain an attempt for the author provider with the outcome
  `skipped_author_provider`, and the run detail shall present that outcome as
  a localized label that names the author exclusion.
- **AC-OFFICE-REVIEW-AUTHOR-EXCL-003.3:** When the other providers are skipped
  for an auto-retryable health state, the run shall park as waiting for
  provider capacity, and shall become eligible again when one of those
  providers recovers.
- **AC-OFFICE-REVIEW-AUTHOR-EXCL-003.4:** When no provider other than the author
  provider exists in the effective order, the run shall park as requiring
  operator action and shall not be retried automatically.

### REQ-OFFICE-REVIEW-AUTHOR-EXCL-004: Exclusion decisions are visible

Every seat-run resolution shall leave a record of whether the exclusion
applied.

#### Acceptance criteria

- **AC-OFFICE-REVIEW-AUTHOR-EXCL-004.1:** When a seat run resolves with an
  author provider excluded, the run's event log shall contain an event naming
  the excluded provider.
- **AC-OFFICE-REVIEW-AUTHOR-EXCL-004.2:** When a seat run resolves and no author
  provider is recorded for the task, the system shall launch the run as normal,
  and the run's event log shall contain an event stating that no exclusion
  applied because no author provider was recorded.
- **AC-OFFICE-REVIEW-AUTHOR-EXCL-004.3:** When a seat run resolves more than once
  (fallback, park lift, manual retry), the event log shall carry one event per
  resolution.
- **AC-OFFICE-REVIEW-AUTHOR-EXCL-004.4:** When the system cannot read the author
  provider or the step's declaration because of a storage error, the seat run
  shall not launch on that resolution, and the dispatcher shall treat the error
  as an infrastructure failure of the run.

### REQ-OFFICE-REVIEW-AUTHOR-EXCL-005: Predictable halts are warned about, not blocked

When the workspace's current setup predicts that a declaring step will halt
its reviews or will not enforce the exclusion, the system shall warn the
operator where the setting arrives and where the setup is changed. The
prediction uses the setup at that moment; it is a hint, not a guarantee.

#### Acceptance criteria

- **AC-OFFICE-REVIEW-AUTHOR-EXCL-005.1:** The system shall predict a possible
  halt for a declaring step when workspace routing is enabled and the
  effective provider order of the workspace, or of any agent in the declared
  role's seat pool, contains exactly one provider. Whether a run actually
  waits depends on the author provider, so the warning shall name that one
  provider and state that review runs wait only when the work ran on it.
- **AC-OFFICE-REVIEW-AUTHOR-EXCL-005.2:** The system shall predict that the
  exclusion will not be enforced for a declaring step when workspace routing
  is disabled or not configured.
- **AC-OFFICE-REVIEW-AUTHOR-EXCL-005.3:** When a workflow import is previewed
  or applied for a workspace and an imported declaring step meets
  AC-OFFICE-REVIEW-AUTHOR-EXCL-005.1 or .2, the preview and the result shall list an exclusion
  setup warning for that step, and the import shall proceed unchanged.
- **AC-OFFICE-REVIEW-AUTHOR-EXCL-005.4:** When a workflow sync applies a
  declaring step that meets AC-OFFICE-REVIEW-AUTHOR-EXCL-005.1 or .2, the sync result's warnings
  shall include an exclusion setup warning for that step.
- **AC-OFFICE-REVIEW-AUTHOR-EXCL-005.5:** When the Office provider routing
  settings page loads, or its settings are saved, and any declaring step in
  the workspace meets AC-OFFICE-REVIEW-AUTHOR-EXCL-005.1 or .2, the page shall show a warning
  banner listing each affected workflow, step, and role, and, for an
  agent-level halt, the agent. The save shall succeed unchanged.
- **AC-OFFICE-REVIEW-AUTHOR-EXCL-005.6:** When no declaring step meets
  AC-OFFICE-REVIEW-AUTHOR-EXCL-005.1 or .2, the system shall show no exclusion setup warning on
  any of these surfaces.
- **AC-OFFICE-REVIEW-AUTHOR-EXCL-005.7:** Exclusion setup warnings on the import
  dialog and the routing settings banner shall be localized in every supported
  locale and readable on desktop and phone widths without horizontal
  scrolling. Workflow sync warnings shall use the existing sync-warning text
  format, which the sync status banner shows verbatim.
- **AC-OFFICE-REVIEW-AUTHOR-EXCL-005.8:** When the prediction cannot be computed
  because of a storage error, the import, sync, or save shall proceed, and the
  surface shall state that the exclusion setup could not be checked.

## Exclusions

- Model-level comparison. Two models run through the same CLI count as the
  same author.
- Vendor-level comparison. A multi-vendor CLI running a model from the
  author's vendor is not excluded.
- A list of steps or a policy expression selecting which providers to exclude.
- Turning the setting on in the built-in `office-default` workflow.
- A workflow-editor control for the setting. The editable Office workflow
  surface (issue #5347) is separate work; this capability is configured
  through workflow templates, import, and sync. When that editor adds the
  control, it is expected to reuse the REQ-OFFICE-REVIEW-AUTHOR-EXCL-005
  prediction.
- Blocking any save, import, or sync because of a predicted halt.
- Warnings on the per-agent routing override form. Its effect appears on the
  provider routing settings banner (AC-OFFICE-REVIEW-AUTHOR-EXCL-005.5).
- Predicting provider health. A provider that is configured but currently
  degraded does not produce a warning.
- Author detection for work done by a person or outside an Office run. Such
  work records no author provider and falls under
  AC-OFFICE-REVIEW-AUTHOR-EXCL-004.2.
