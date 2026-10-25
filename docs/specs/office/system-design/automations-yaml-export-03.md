---
status: draft
system: office
requirements:
  - REQ-OFFICE-AUTOMATIONS-YAML-EXPORT-001
created: 2026-08-19
updated: 2026-08-20
owners:
  - tbd
---
# Automations YAML Export System Design Part 3

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-AUTOMATIONS-YAML-EXPORT-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-AUTOMATIONS-YAML-EXPORT-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Data model

The exported document. Field names are the YAML keys.

```yaml
version: 1
type: kandev_automations
automations:
  - name: Daily Review — @kegmil/offline-first
    enabled: false
    max_concurrent_runs: 1
    task_title_template: Daily Review — offline-first ({{trigger.timestamp}})
    prompt: |
      ...
    agent_profile:
      agent_name: Claude Code
      model: opus[1m]
      mode: auto
    executor_profile:
      executor: exec-worktree
      name: Worktree
    workflow:
      name: Kanban
      step: In Progress
    repositories:
      - kegmil-offline-first
    triggers:
      - type: scheduled
        enabled: true
        config:
          cron_expression: 0 9 * * *
          timezone: Asia/Singapore
```

**Included, always emitted:** `name`, `enabled`, `max_concurrent_runs`, `triggers`
(possibly empty), and per trigger `type`, `enabled`, `config` (possibly empty).

**Included, omitted when empty:** `description`, `prompt`, `task_title_template`,
`agent_profile`, `executor_profile`, `workflow`, `repositories`, and the document-level
`warnings` (AC-20).

**Excluded — secret:** `webhook_secret`.

**Excluded — runtime state:** `last_triggered_at`, `created_at`, `updated_at` (automation);
`last_evaluated_at`, `created_at`, `updated_at` (trigger); everything in `automation_runs`.

**Excluded — instance identity:** `id`, `workspace_id`, `automation_id`, `trigger.id`.

**Excluded — withdrawn or legacy columns:** `execution_mode` (withdrawn; live values are
`''` on six rows and `'run'` on one, and no firing path reads it), `automations.repository_id`
(inert legacy column superseded by `automation_repositories`), and the derived
`legacy_board_card` field.

### Field disposition

This table is the contract AC-22 tests against. Every field of `Automation` and
`AutomationTrigger` appears exactly once. `exported` names the YAML key it becomes;
`excluded` names why.

| Struct | Field | Disposition |
|---|---|---|
| `Automation` | `Name` | exported → `name` |
| `Automation` | `Description` | exported → `description` |
| `Automation` | `Prompt` | exported → `prompt` |
| `Automation` | `TaskTitleTemplate` | exported → `task_title_template` |
| `Automation` | `Enabled` | exported → `enabled` |
| `Automation` | `MaxConcurrentRuns` | exported → `max_concurrent_runs` |
| `Automation` | `Triggers` | exported → `triggers` |
| `Automation` | `WorkflowID` | exported → `workflow.name` (renamed; resolved to a descriptor) |
| `Automation` | `WorkflowStepID` | exported → `workflow.step` (renamed; resolved to a descriptor) |
| `Automation` | `AgentProfileID` | exported → `agent_profile` (renamed; resolved to a descriptor) |
| `Automation` | `ExecutorProfileID` | exported → `executor_profile` (renamed; resolved to a descriptor) |
| `Automation` | `RepositoryIDs` | exported → `repositories` (renamed; resolved to names) |
| `Automation` | `WebhookSecret` | excluded — secret |
| `Automation` | `ID` | excluded — instance identity |
| `Automation` | `WorkspaceID` | excluded — instance identity |
| `Automation` | `LastTriggeredAt` | excluded — runtime state |
| `Automation` | `CreatedAt` | excluded — runtime state / fire anchor |
| `Automation` | `UpdatedAt` | excluded — runtime state |
| `Automation` | `LegacyBoardCard` | excluded — derived from a withdrawn column |
| `AutomationTrigger` | `Type` | exported → `type` |
| `AutomationTrigger` | `Enabled` | exported → `enabled` |
| `AutomationTrigger` | `Config` | exported → `config` (the decoded form; see AC-11, AC-39, AC-41) |
| `AutomationTrigger` | `ConfigJSON` | excluded — raw storage of `Config`, same value, not a second concept |
| `AutomationTrigger` | `ID` | excluded — instance identity |
| `AutomationTrigger` | `AutomationID` | excluded — instance identity |
| `AutomationTrigger` | `LastEvaluatedAt` | excluded — runtime state / fire anchor |
| `AutomationTrigger` | `CreatedAt` | excluded — runtime state / fire anchor |
| `AutomationTrigger` | `UpdatedAt` | excluded — runtime state |

Two notes a builder needs. `AutomationTrigger` carries **two** fields for one concept —
`Config json.RawMessage` (tagged `db:"-"`) and `ConfigJSON string` (tagged `db:"config"`) —
and the DTO has a single `config`; the table settles which is which rather than leaving it
to a tag convention. And the columns excluded above that are **not** struct fields
(`execution_mode`, `automations.repository_id`) are deliberately absent from this table,
because reflection cannot see them; they are covered by AC-43 instead.

## API surface

Two read-only endpoints, mirroring the shape Office's config export already established
(`GET /workspaces/:wsId/config/export` and `.../export/zip`):

| Method | Path | Response |
|---|---|---|
| `GET` | `/api/v1/workspaces/:wsId/automations/export` | `200`, `Content-Type: application/yaml`, body is the single YAML document |
| `GET` | `/api/v1/workspaces/:wsId/automations/export/zip` | `200`, `Content-Type: application/zip`, `Content-Disposition: attachment; filename=kandev-automations.zip` |

REST rather than a WebSocket action: every other automation operation is a WS action because
it is interactive state, but an export is a bulk byte stream a client retrieves at a URL — a plain
HTTP `GET` a `fetch` can read a status and a `Blob` from, and that a human or a script can also
curl directly. `Content-Disposition` is set for that direct-access case; it is **not** the
mechanism the in-app control uses, which is `fetch` + `Blob` per AC-37. Office set the REST
precedent for the same job, though not the frontend one.
