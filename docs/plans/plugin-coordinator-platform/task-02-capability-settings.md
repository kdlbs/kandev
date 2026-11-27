---
id: "02-capability-settings"
title: "Workspace capability approval UI"
status: done
wave: 2
depends_on: ["01-exact-host-foundation"]
plan: "plan.md"
requirements:
  - REQ-PLUGINS-MANAGED-COORDINATION-001
acceptance_criteria:
  - AC-PLUGINS-MANAGED-COORDINATION-001.3
system_design:
  - ../../specs/plugins/system-design/managed-coordination.md
---

# Task 02: Workspace capability approval UI

## Summary

Expose the existing grant/revoke/audit substrate in native plugin settings. Present the selected installation and workspace on desktop and phone.

## In scope

- Add typed frontend queries/actions and approval controls to plugin detail, with manifest digest and revision checks.
- Show stale, revoked, and upgrade-review states and append-only audit history. Localize host copy in all supported catalogs.

## Out of scope

- Work owned by other orders, publishing external repositories, and unapproved production rollout.
- Coordinator-specific host roles, private API shortcuts, and changes to unrelated v1 behavior.

## Acceptance

- A human can grant, narrow, and revoke declared workspace capabilities and inspect their audit records.
- Desktop and mobile tests prove stale grants cannot overwrite newer approvals and revoked commands stop working.

## ASCII UI preview

See the [combined preview](plan.md#ascii-ui-preview). These excerpts keep its stable labels.

### UI-01: Capability settings

```text
Desktop: Settings > Plugins > selected plugin > workspace
+----------------------------------------------------------------+
| Coordinator plugin                 Workspace: Product            |
| Requested capabilities     Current grant      Audit history       |
| [x] Read tasks             [ ] Manage tasks                      |
| [ ] Run agents             [ ] Write linked issues               |
| Manifest changed: review added permissions                       |
| [Revoke access]                           [Save approval]         |
+----------------------------------------------------------------+
Phone: same entry, full-height settings
+----------------------------+
| < Plugin access   Product  |
| Read capabilities          |
| [x] Tasks                  |
| Write capabilities         |
| [ ] Manage tasks           |
| [ ] Run agents             |
| [ ] Linked issues          |
| [View audit history]       |
| [Save approval]            |
| [Revoke access]            |
+----------------------------+
```

Permission groups and explicit workspace are required. The phone body scrolls; actions stay reachable above the safe area. Saving, stale revision, missing approval, and revoked states appear inline. A plugin cannot render a grant as its own agent action.

Applicable criteria: `AC-PLUGINS-MANAGED-COORDINATION-001.3`.

## Verification

Add the named tests before implementation using TDD. These commands run after the work order is implemented; they are not results from planning.

```bash
(cd apps/web && pnpm run typecheck && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/plugins/managed-capabilities.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/plugins/mobile-managed-capabilities.spec.ts)
```

Evidence to create:

- `apps/web/e2e/tests/plugins/managed-capabilities.spec.ts`: `grant, revoke, upgrade review, stale revision`.
- `apps/web/e2e/tests/plugins/mobile-managed-capabilities.spec.ts`: `phone grant and revoke`.

Follow the [plan verification rules](plan.md#tests), including expected test presence, generated contracts, and public documentation checks.

## Files likely touched

- `apps/web/components/settings/plugins/plugin-detail.tsx`
- `apps/web/lib/plugins/`
- `apps/web/src/locales/`
- `apps/backend/internal/plugins/approval_api.go`

New test paths above are deliverables. Existing directories indicate the owning boundary; inspect scoped AGENTS.md before editing.

## Dependencies

- [Task 01](task-01-exact-host-foundation.md)

## Risks

A route-level workspace switch must not reuse a previous workspace grant or approve through a plugin-owned callback.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/plugins/requirements/managed-coordination.md) and [design](../../specs/plugins/system-design/managed-coordination.md).
- [Ownership decision](../../decisions/2026-09-25-plugin-coordination-platform.md).
- [Source baseline and fork mapping](plan.md#source-baseline-and-fork-mapping).

## Results

Added workspace-scoped capability context, grant, and revoke endpoints. The host
authorizes each request through `workspace.manage`, checks the manifest digest and
approval revision, and returns immutable audit history. Plugin settings now show
the installation, selected workspace, declared read/write capabilities, review
and revoked states, reasons, and audit events. Changed manifests require an
explicit reapproval, including when the selected capabilities are unchanged.
The panel is workspace-keyed so switching workspaces cannot reuse prior grant
state. Phone controls use 44px targets and a safe-area-aware sticky action row.

Added frontend API tests, a desktop E2E for grant, stale-update conflict, newer
grant preservation, revoke, audit history, and upgrade-review reapproval, plus a
phone E2E for grant/revoke, touch target sizes, and horizontal overflow. The
backend upgrade handler test verifies that a package update creates a review
state while retaining the prior approval revision. Updated plugin authoring
docs and all supported locale catalogs.

Validation passed: focused Go approval handler tests; backend app compile; web
typecheck; focused plugin API tests (30 tests); changed-file ESLint; all i18n
checks; public-doc validation (62 tests, 47 pages); backend `make build`; web
`build:e2e`; E2E fixture packaging; desktop E2E (2 tests); phone E2E (1 test);
and whitespace validation.
