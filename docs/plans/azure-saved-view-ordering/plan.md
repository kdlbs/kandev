---
created: 2026-10-20
status: done
requirements:
  - REQ-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001
system_design:
  - ../../specs/integrations/system-design/azure-devops-integration-02.md
legacy_specs: []
---

# Azure saved-view ordering

Status: done

## Scope and references

Preserve sequential user intent when same-page Azure saves/deletes overlap.
The user authorized autonomous design and implementation without a separate
planning handoff. No backend API, schema, or shared scope-bar redesign.

- [Requirements](../../specs/integrations/requirements/azure-devops-integration.md): REQ-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001, criteria .15-.18.
- [Design](../../specs/integrations/system-design/azure-devops-integration-02.md#saved-view-mutation-settlement).
- [Work order](task-01-order-mutations.md): done; sole work package.

## Evidence and risk

Source baseline: `74427cb50c24e281e5b3fa826c7136ffed5be5db`.
The page permits consecutive confirmed deletes while writes are pending.
Unconditional PUT responses and rollback snapshots can resurrect deleted views;
independent whole-list writes can also persist in the wrong order. A stale GET
can overwrite admitted edits. No live external Azure incident is claimed.

Use one local queue with confirmed baseline and reapplied pending operations.
Limit: request errors can be ambiguous; multi-tab and remount ordering are out
of scope. Keep error rejection for callers and handle the page's void delete.

## Validation

The work order owns exact commands and results. Use deterministic deferred API
responses without sleeps, and a narrow browser flow on desktop and phone.
Parent validation completed before publication. Delivery follows the normal CI
and review gates in [PR #4639](https://github.com/kdlbs/kandev/pull/4639).

## Results

Implementation and local task-defined checks are complete: 21 focused hook
cases, three desktop browser flows, and three phone browser flows passed.
Scoped lint, typecheck (6 GB Node heap), i18n, specs, public-doc validators, and
PR documentation coverage preflight passed. The work order records exact
commands, RED/GREEN evidence, and the failed-baseline review remediation.
Live CI, review, and merge state are tracked in PR #4639.

## ASCII UI preview

UI-01: Existing Azure saved-view deletion and failure state.

```text
Desktop: [Saved views v] -> [Delete view] -> [Cancel] [Delete view]
Phone:   [Saved views v] -> bottom menu -> inset confirmation
Failure: [Failed to delete saved query] (existing reporting toast)
         [Saved views v] -> restored view available for retry
```

The existing scope menu, confirmation, scrolling, focus return, and phone
44px targets are retained. Only settlement and failure feedback change.
The phone exemplar is the shipped Azure saved-view confirmation in
`mobile-azure-devops.spec.ts`, using shared phone menus/confirmation drawers.
Copy is illustrative English backed by the existing localization key. Browser
proof covers UI-01 and AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.18.
