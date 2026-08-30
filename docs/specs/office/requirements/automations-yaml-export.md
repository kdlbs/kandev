---
status: draft
system: office
created: 2026-08-19
updated: 2026-08-20
owners:
  - tbd
---
# Automations YAML Export Requirements

## Overview

A workspace's automations are operational prose with no version control. Measured against the live store on 2026-08-19: **7 automations, 78,589 bytes of prompt across 1,338 lines, spread over 3 workspaces**. The largest single automation prompt is 404 lines, longer than most workflow steps. All 7 carry a `webhook_secret`.

## Requirements

### REQ-OFFICE-AUTOMATIONS-YAML-EXPORT-001: Automations YAML Export

**Intent:** A workspace's automations are operational prose with no version control. Measured against the live store on 2026-08-19: **7 automations, 78,589 bytes of prompt across 1,338 lines, spread over 3 workspaces**. The largest single automation prompt is 404 lines, longer than most workflow steps. All 7 carry a `webhook_secret`.

#### Acceptance criteria

- **AC-OFFICE-AUTOMATIONS-YAML-EXPORT-001.1:** **A single YAML document** containing every automation in the workspace, for reading and for API consumers.
- **AC-OFFICE-AUTOMATIONS-YAML-EXPORT-001.2:** **A zip of one file per automation**, laid out as `.kandev/automations/<slug>.yml`, which is the shape a human unpacks into a repository so each automation gets its own diff.
- **AC-OFFICE-AUTOMATIONS-YAML-EXPORT-001.3:** **`webhook_secret` is never exported**, and the export type is a purpose-built DTO rather than the `Automation` domain struct, because the existing `json:"-"` tag does not redact under a YAML marshaller.
- **AC-OFFICE-AUTOMATIONS-YAML-EXPORT-001.4:** **Scheduler state is never exported.** `last_evaluated_at`, `created_at`, `last_triggered_at`, and `updated_at` are excluded. The first two are the cron scheduler's fire anchor.
- **AC-OFFICE-AUTOMATIONS-YAML-EXPORT-001.5:** **Foreign keys are exported as portable descriptors, not UUIDs.** A UUID is not "enough to recreate the automation by hand".
- **AC-OFFICE-AUTOMATIONS-YAML-EXPORT-001.6:** **Trigger config is carried as an order-normalized generic mapping**, not decoded into per-type structs, so a config key the exporter does not know about survives.
- **AC-OFFICE-AUTOMATIONS-YAML-EXPORT-001.7:** **Output is byte-deterministic** for a given database state, with every ordering resolved by a named column. This binds the zip archive's bytes as well as the YAML.
- **AC-OFFICE-AUTOMATIONS-YAML-EXPORT-001.8:** **Numbers in trigger config are copied character-for-character**, never converted through a Go numeric type, because every conversion route corrupts some class of input — rounding large integers, adding exponents to plain ones, or quoting all of them.

## System design

The migrated technical source is split into [part 1](../system-design/automations-yaml-export-01.md), [part 2](../system-design/automations-yaml-export-02.md), [part 3](../system-design/automations-yaml-export-03.md), [part 4](../system-design/automations-yaml-export-04.md), [part 5](../system-design/automations-yaml-export-05.md), [part 6](../system-design/automations-yaml-export-06.md).
