---
created: 2026-09-14
status: complete
requirements:
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-001
system_design:
  - ../../specs/agents/system-design/no-silent-model-fallback-01.md
  - ../../specs/agents/system-design/no-silent-model-fallback-02.md
legacy_specs: []
---

# Implementation Plan: Exact Profile Model Identity

## Overview

A configured agent profile is a runtime identity policy, not a preference.
The executor ACP catalog stays authoritative for what can be selected, and
Kandev never sends a speculative `SetModel` for an unadvertised model. When
the requested exact model cannot be attested, the launch fails before
inference instead of silently substituting a provider default.

The requirement and system designs in `docs/specs/agents/` define the
semantic contract for exact profile model identity. This delivery updates
that contract so launch and workflow entry never infer a model variation or
silently accept a provider default for an exact profile.

## Tasks

- [x] [Task 01: Enforce exact profile model identity](task-01-enforce-launch-identity.md)

## Notes

- The work order is the delivery package for this change. The requirement
  and system-design files record the semantic changes for exact model
  attestation, strict failure, and explicit fallback behavior.
