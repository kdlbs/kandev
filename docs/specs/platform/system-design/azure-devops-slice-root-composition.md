---
status: current
system: platform
requirements:
  - REQ-PLATFORM-AZURE-DEVOPS-SLICE-COMPOSITION-001
---

# Azure DevOps slice root composition system design

## Purpose and boundaries

This design records the TypeScript boundary between the Azure DevOps slice
creator and the Zustand application root store. It covers only the slice setter
capability and regression evidence. It does not change state shape, runtime
behavior, provider composition, or other slices.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-PLATFORM-AZURE-DEVOPS-SLICE-COMPOSITION-001` | Setter capability, State behavior, Verification |

## Setter capability

`apps/web/lib/state/slices/azure-devops/azure-devops-slice.ts` defines the
creator's setter as a function that accepts an Immer recipe over
`Draft<AzureDevOpsSlice>`. The slice actions use that capability to update the
pull-request and work-item maps.

`apps/web/lib/state/store.ts` passes the root store's Immer-enabled `set`
directly to `createAzureDevOpsSlice`. The creator does not require Zustand's
`get`, store API, replacement overload, or whole-root state type. The local
setter type does not introduce a generic slice abstraction.

The recipe-only setter follows the precedent in
[Features slice state](features-slice-state.md). That design is a typing
pattern reference, not an Azure DevOps product requirement.

## State behavior

The slice starts with empty pull-request and work-item maps. Snapshot actions
replace their respective maps. Association actions update an item with the same
association ID or insert a new item. Reset actions clear only their respective
maps. Immer composition preserves references for unrelated root state when
these slice actions run.

## Verification

`apps/web/lib/state/slices/azure-devops/azure-devops-slice.test.ts` exercises the
creator in an isolated Immer store, including defaults and association
insert/update/reset behavior. It also exercises `createAppStore` composition and
checks that unrelated task and workspace references remain stable.
Web typecheck verifies that the root store can pass its setter without an
assertion.
