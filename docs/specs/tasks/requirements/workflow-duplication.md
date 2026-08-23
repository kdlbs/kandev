---
status: draft
system: tasks
created: 2026-08-11
owners:
  - kandev
---
# Workflow Duplication Requirements

## Overview

This document is the migrated task-system source for the capability. The source detail below remains authoritative while the system is migrated into separate requirement and design records.

## Requirements

### REQ-TASKS-WORKFLOW-DUPLICATION-001: Workflow Duplication

**Intent:** Preserve the observable task or workflow behavior recorded by the legacy specification.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-DUPLICATION-001.1:** When a consumer uses this capability, the system shall provide the observable behavior and exclusions documented below.

## Migrated source detail

## Why

Workflow authors often need a new pipeline that differs only slightly from an existing one. Rebuilding its metadata, prompts, steps, and transitions is slow and risks omitting configuration that already works.

## What

- Every persisted Kanban workflow on an editable workspace's Workflows settings page offers a **Duplicate** action.
- Duplicate reads the source's saved configuration and creates a route-local, unsaved workflow draft directly after the source card. It does not persist a workflow until the user presses the route-level **Save changes** action.
- The draft copies the source description, workflow prompt, default agent profile, and all persisted step configuration.
- Copied step configuration includes prompts, colors, order, transitions, and start-step state.
- It preserves each step's semantic stage type, including `work`, `review`, `approval`, and `custom`.
- It also includes command-panel visibility, manual-move policy, auto-archive policy, and step agent profiles.
- It also includes completion-signal policy, cancellation policy, WIP limits, and pull-from relationships.
- The copy receives new workflow and step identities. Every transition or relationship that referred to a source step refers to the corresponding copied step in the draft.
- The draft does not inherit template identity, workflow-sync ownership, source path, tasks, task sessions, execution history, or workflow history. A copy of a sync-managed workflow is a manual workflow that can be edited independently.
- The generated name uses `<base> (copy)`. If that name already exists, including as another unsaved draft, Kandev uses the lowest available numbered name: `<base> (copy 2)`, `<base> (copy 3)`, and so on. Duplicating a name that already ends in `(copy)` or `(copy N)` reuses its original base.
- Duplicate is unavailable for a new unsaved workflow or a workflow with unsaved metadata or step changes. The UI explains that the source must be saved before it can be duplicated.
- Duplicate is unavailable in the read-only Improve Kandev workspace. It remains available for a sync-managed workflow in a normal editable workspace because creating the independent manual draft does not mutate the source.
- The source workflow remains unchanged. Editing, saving, deleting, or reordering the duplicate after creation has no ongoing effect on the source.
- Discarding or removing the duplicate draft creates no durable workflow. Reloading before Save also discards it, following the existing [Settings Manual Save](../../ui/settings-manual-save.md) contract.

## Permissions

The user must be able to create workflows in the source workspace. A sync-managed source does not bypass workspace ownership or the Improve Kandev read-only boundary.

## Failure Modes

- If Kandev cannot load the source's saved steps, it shows a duplication error and creates no draft.
- If the duplicated draft fails to save, the existing workflow-draft retry and partial-save cleanup behavior applies. Save must not create a second copy when retried.
- A server or WebSocket refresh must not replace the duplicate's unsaved local edits.

## Persistence Guarantees

- Duplication itself performs no write. The draft is route-local and does not survive reload, process restart, or explicit discard.
- A successful **Save changes** persists a new workflow and new steps in the same workspace.
- Save also persists the list order. The duplicate remains after its source unless the user moves it first.
- The persisted duplicate has no reference that keeps it synchronized with the source.
- Existing tasks, task sessions, and workflow history remain attached only to the source.

## Scenarios

- **GIVEN** a clean persisted workflow, **WHEN** the user chooses Duplicate, **THEN** an editable draft appears after the source and no write occurs.
- **GIVEN** `Review` and `Review (copy)` already exist, **WHEN** the user duplicates `Review`, **THEN** the draft name is `Review (copy 2)`.
- **GIVEN** `Review (copy 2)` is duplicated while `Review (copy)` also exists, **WHEN** the draft is created, **THEN** its name is `Review (copy 3)`.
- **GIVEN** source relationships refer to source steps, **WHEN** the draft is created, **THEN** those relationships refer only to copied steps.
- **GIVEN** a source contains tasks and history, **WHEN** the user saves its duplicate, **THEN** the new workflow contains no copied tasks or history.
- **GIVEN** an unsaved duplicate draft, **WHEN** the user removes it, resets the route, discards while leaving, or reloads, **THEN** no durable workflow is created.
- **GIVEN** an unsaved duplicate, **WHEN** the user saves and reloads, **THEN** the copied workflow and its remapped steps remain visible.
- **GIVEN** a sync-managed workflow, **WHEN** the user duplicates and saves it, **THEN** the source remains read-only and the manual copy is editable.
- **GIVEN** a new or dirty workflow, **WHEN** its actions appear, **THEN** Duplicate is disabled with a save-first explanation.
- **GIVEN** a 390px touch viewport, **WHEN** the user duplicates and saves, **THEN** all required controls fit without document-level horizontal overflow.

## Out of Scope

- Copying workflows between workspaces.
- Copying tasks, task sessions, execution history, or workflow history.
- Bulk duplication.
- Keeping a duplicate synchronized with its source or workflow-sync repository.
- Duplicating Office-style workflows from the Kanban workflow settings page.

## Implementation Plan

See [the implementation plan](../../../plans/workflow-duplication/plan.md).