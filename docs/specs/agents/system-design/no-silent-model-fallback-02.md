---
status: draft
system: agents
requirements:
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-001
created: 2026-08-23
owners:
  - kandev
---
# No Silent Model Fallback System Design Part 2

## Purpose and boundaries

This design preserves the technical source detail for `REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Risks & Open Questions

- **Behavior change for existing profiles**: an unadvertised start model no
  longer stops the launch. Kandev uses the executor agent default and persists
  an actionable warning.
- **Office post-start fallback remains workspace-routing-governed**: a
  default-mode profile's Office run can still be re-dispatched to another
  provider mid-session by the ADR office policy (availability codes →
  `DecisionFallback`). This is intentional — office authorization is the
  workspace routing configuration, not the execution profile — and is
  documented in the behavior matrix above.
- **Probe staleness**: the advertised list can be stale (probe cached).
  The profile picker uses it only as a hint. The executor session catalog owns
  the launch decision.
- **Cold Claude model lists**: a valid restricted model can be absent from a
  cold bridge's initial list. Pre-session exposure lets the bridge include and
  select the configured model. If the bridge still omits it, Kandev uses the
  agent default and persists a warning.
- **Context reset model changes**: this amendment covers the model selected
  before the initial process starts. It does not restart a live ACP bridge to
  expose a newly selected hidden model during context reset.
- **Office vs. kanban surfaces**: both share the same agent-profile rows.
  The advisory picker behavior covers kanban task creation and Office setup.
  Office run-detail routing surfaces are unchanged.
- **Collapsed controls remain legible**: the disclosure header summarizes the
  effective mode, and dirty-state decoration is applied to the disclosure
  container so a collapsed section cannot conceal that it has unsaved changes.
- **Hover is supplementary**: every info icon is focusable, and coarse-pointer
  devices receive the same content in a drawer. The visible option helper copy
  remains the baseline explanation.
