---
created: 2026-10-07
status: completed
requirements:
  - REQ-UI-SIDEBAR-CUSTOMIZATION-007
system_design:
  - ../../specs/ui/system-design/sidebar-customization.md
legacy_specs: []
---

# Sidebar browser isolation

## Scope

Reset the shared E2E workspace layout between tests while preserving saved
navigation height across reloads within one test. Production settings and
sidebar behavior remain governed by the existing customization contract.

The original feature and its other presentation contracts remain in the
[sidebar presentation plan](../sidebar-presentation-preferences/plan.md).
This package records only the PR #3598 browser-fixture remediation and its
first-attempt verification. It does not add production behavior or a toggle.

## Work orders

1. [Isolate browser layout](task-01-isolate-layout.md): completed.

## Validation

The work order preserves the original remediation commands and outcomes.
Fresh PR-head CI is an external pending gate.
