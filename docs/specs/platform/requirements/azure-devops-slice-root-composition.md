---
status: active
system: platform
created: 2026-09-27
owners:
  - kandev
---

# Azure DevOps slice root composition

## Overview

This requirement defines an internal TypeScript composition contract for the
Azure DevOps slice in the web root store. It protects compile-time setter
capability and state preservation. It does not define user-visible behavior or
change a public API.

## Terminology

- **Recipe setter:** A setter that accepts an Immer recipe for the slice's draft
  state.
- **Root composition:** Passing the root store's Immer-enabled setter to a slice
  creator while composing the application store.

## Requirements

### REQ-PLATFORM-AZURE-DEVOPS-SLICE-COMPOSITION-001: Preserve typed root composition

**Intent:** Keep the Azure DevOps slice compatible with the application store
through the setter capability that its Immer recipes use.

#### Acceptance criteria

- **AC-PLATFORM-AZURE-DEVOPS-SLICE-COMPOSITION-001.1:** The slice creator shall
  accept a recipe setter over `Draft<AzureDevOpsSlice>`, and root composition
  shall type-check without a type assertion or a broader root-state cast.
- **AC-PLATFORM-AZURE-DEVOPS-SLICE-COMPOSITION-001.2:** Slice composition shall
  preserve default state, pull-request and work-item insert/update/reset
  behavior, and references to unrelated root state.

## Out of scope

- Changes to rendered behavior, public APIs, or Azure DevOps state shape.
- Changes to other slices or the root store's provider composition.
- A generic slice factory or a wider root-store typing migration.

## Related design and delivery

- [System design](../system-design/azure-devops-slice-root-composition.md)
- [Implementation plan](../../../plans/azure-devops-slice-root-composition/plan.md)
