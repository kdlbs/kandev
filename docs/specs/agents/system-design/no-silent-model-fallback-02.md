---
status: current
system: agents
requirements:
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-001
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-002
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-003
created: 2026-08-23
owners:
  - kandev
---
# No Silent Model Fallback System Design Part 2

## Purpose and boundaries

This design records risks for executor-authoritative model fallback and unique
variation resolution.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-001` | [Migrated source detail](#migrated-source-detail) |
| `REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-002` | [Risks & Open Questions](#risks--open-questions) |
| `REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-003` | [Risks & Open Questions](#risks--open-questions) |

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
  The profile editor uses it only as a hint. Profile selectors do not render
  host model advisories. The executor session catalog owns the launch decision.
- **Cold Claude model lists**: a valid restricted model can be absent from a
  cold bridge's initial list. Pre-session exposure lets the bridge include and
  select the configured model. If the bridge still omits it, Kandev uses the
  agent default and persists a warning.
- **Context reset model changes**: this amendment covers the model selected
  before the initial process starts. It does not restart a live ACP bridge to
  expose a newly selected hidden model during context reset.
- **Office vs. kanban surfaces**: both share the same agent-profile rows.
  The shared selector behavior covers kanban task creation and Office setup.
  Office run-detail routing surfaces are unchanged.
- **Collapsed controls remain legible**: the disclosure header summarizes the
  effective mode, and dirty-state decoration is applied to the disclosure
  container so a collapsed section cannot conceal that it has unsaved changes.
- **Hover is supplementary**: every info icon is focusable, and coarse-pointer
  devices receive the same content in a drawer. The visible option helper copy
  remains the baseline explanation.
- **Provider-specific labels remain opaque**: `1m`, `270k`, and `fast` have no
  Kandev-defined order. More than one variation therefore fails closed to the
  existing provider-default path.
- **Host and executor decisions can differ**: a unique host variation is only
  an advisory. The selected executor can advertise zero, one, or multiple
  variations at launch.
- **Bracketed requests do not drift sideways**: a request such as `opus[1m]`
  does not infer `opus[270k]`. The user must select another explicit model.
- **Legacy automatic fallback remains unchanged**: when `auto_fallback` is
  enabled and the requested model is absent, Kandev does not apply an explicit
  fallback or infer a variation. It continues with the provider default.
- **Removal must be narrow**: authentication, missing-CLI, and failed-probe
  indicators remain. Profile-editor advisories and persisted task warnings
  retain their existing behavior.
- **Evidence has limits**: a model warning does not prove a cache defect.
  Discovery refresh or normalization changes require separate root-cause
  evidence; this amendment does not change those contracts.
