---
status: draft
system: agents
created: 2026-08-23
owners:
  - kandev
---
# No Silent Model Fallback Requirements

## Overview

**Status**: implemented (exact-profile amendment 2026-09-06) **Date**: 2026-08-04 **Slug**: `no-silent-model-fallback`

## Requirements

### REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-001: No Silent Model Fallback

**Intent:** A configured exact model is a runtime identity policy. The executor
catalog remains authoritative, but an unadvertised or unselectable exact model
must stop the launch before inference rather than silently run another model.

#### Acceptance criteria

- **AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.1:** Given a profile with a
  non-empty model, `auto_fallback=false`, and no `fallback_model`, if the
  executor does not advertise or cannot apply that model, the session shall
  fail before a prompt, tool call, or agent output.
- **AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.2:** An advertised explicit
  `fallback_model` may be selected with one durable warning. When
  `auto_fallback=true`, provider-default continuation remains explicitly
  authorized and visible.

### REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-002: Resolve one advertised model variation

**Status:** Superseded by the exact-profile amendment to
`REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-001`. An inferred variation is not an
exact model identity and therefore cannot authorize inference.

- **AC-AGENTS-NO-SILENT-MODEL-FALLBACK-002.1:** This superseded requirement
  has no active runtime behavior; model selection follows
  `REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-001`.

## System design

The migrated technical source is split into [part 1](../system-design/no-silent-model-fallback-01.md), [part 2](../system-design/no-silent-model-fallback-02.md).
