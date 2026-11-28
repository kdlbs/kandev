---
created: 2026-11-05
status: done
requirements:
  - REQ-AGENTS-PROVIDER-ERROR-SIGNATURES-001
system_design:
  - ../../specs/agents/system-design/provider-error-signatures.md
legacy_specs: []
---

# OpenCode period relative reset

An OpenCode period-limit diagnostic may state `Resets in 4hr 19min`. The
adapter can project this timing, but direct classifier inputs may contain only
the diagnostic text. Preserve the stated duration using its observation time,
without taking a reset from an unrelated line or changing routing policy.

The [work order](task-01-period-relative-reset.md) records the narrow path,
boundaries and validation. Other provider signatures and relative timing with
unknown observation time remain outside this increment.
