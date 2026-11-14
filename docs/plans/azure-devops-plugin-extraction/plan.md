---
created: 2026-11-02
status: draft
requirements:
  - REQ-PLUGINS-AZURE-DEVOPS-EXTRACTION-001
  - REQ-PLUGINS-PLUGINS-001
  - REQ-PLUGINS-REPOSITORY-TASK-CREATION-001
system_design:
  - ../../specs/plugins/system-design/azure-devops-extraction.md
  - ../../specs/plugins/system-design/plugins-03.md
  - ../../specs/plugins/system-design/repository-provider-task-creation.md
legacy_specs:
  - ../../specs/integrations/requirements/azure-devops-integration.md
---

# Implementation Plan: Azure DevOps Plugin Extraction

## Overview

The first-party Azure DevOps integration is replaced by the runtime plugin
[`kandev-plugin-azure-devops`](https://github.com/JnManso/kandev-plugin-azure-devops).
As agreed with the maintainer on 2026-11-03, the Kandev repository receives
exactly two changes: one that removes the built-in integration, and one that
lists the plugin in the registry. No data is migrated; the user reconnects
after installing the plugin.

This plan covers the Kandev side only: the extraction contract, the retirement
migration, the generic host changes, the registry entry, and the public
installation and reconnect guidance. The plugin's product requirements,
system designs, and work orders live in the plugin repository, so its
maintainer can evolve Azure DevOps behavior without Kandev PRs.

## Migration checklist

- [ ] Plugin parity, packaged E2E, Microsoft sign-in, teams and sprints, and
      board card children, per the plugin-owned
      [plan](https://github.com/JnManso/kandev-plugin-azure-devops/blob/main/docs/plans/azure-devops-plugin/plan.md),
      [requirements](https://github.com/JnManso/kandev-plugin-azure-devops/blob/main/docs/specs/requirements/azure-devops-plugin.md),
      and [system design](https://github.com/JnManso/kandev-plugin-azure-devops/blob/main/docs/specs/system-design/azure-devops-plugin.md).
- [ ] K-A: the built-in integration is removed (task 08), reviewed, merged,
      and released.
- [ ] The plugin pins `min_kandev_version` to that release and publishes
      `v0.1.0` with its package and checksums.
- [ ] K-B: the registry lists the plugin (task 09).

## Scope

### In scope

- Removal of the built-in integration from Kandev, including its tables and
  credentials, and release of its provider reservations.
- Host changes the extraction relies on: plugin-managed clone credentials and
  repository provider task creation.
- Registry listing and public installation and reconnect guidance.

### Out of scope

- Data migration.
- The plugin's own behavior, which the plugin repository specifies.
- New host contracts. Possible follow-ups, with their earlier designs archived
  outside the repository:
  - a fence so a plugin can reveal only its own secrets (security gap found in
    `internal/plugins/host.go` `RevealSecret`; to be reported to the
    maintainer separately);
  - broker addresses for local Docker and SSH without a public URL;
  - delivery-ledger links and sidebar watch classification for plugin
    providers;
  - launch delegations for plugin watches on owned executors.
- Transferring the plugin repository to `kdlbs`.

## Delivery and review gates

| Gate | Condition | Owner |
| --- | --- | --- |
| G0 Maintainer alignment | Done 2026-11-03: the maintainer asked for one removal PR and one registry PR, and confirmed no migration is needed. | done |
| G1 Removal builds | Task 08 complete: backend and web build, tests pass, inventory check empty. | agent |
| G2 Plugin parity | The plugin's work orders pass their verification against the removal checkout. | plugin owner |
| G3 Removal merged and released | K-A reviewed and merged; a Kandev release contains it. | maintainer |
| G4 Plugin released | `min_kandev_version` pinned to that release; GitHub release `v0.1.0` with package and checksums. | plugin owner |
| G5 Listed | K-B merged; the registry index resolves the release. | maintainer |

Rollback before G3: nothing changed for the user. After G3: downgrade and
restore the boot backup.

### PR grouping

| PR | Work orders | Notes |
| --- | --- | --- |
| K-A `kdlbs/kandev` | 08 | Removal, one PR, breaking-change title |
| K-B `kdlbs/kandev` | 09 | Registry entry |

The plugin ships in its own repository, in one PR.

## Technical approach

- **Removal (task 08):** the inventory in the extraction design; a migration
  dropping tables and secrets; reservations released; docs and spec
  lifecycle; `rg` completion check.
- **Registry (task 09):** `plugin-registry/plugins.yaml` entry;
  `docs/public/integrations.md` pointer if not already in K-A.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| EXTRACTION-001.1 | removal inventory `rg` check in the K-A PR body; `go build ./...`, `pnpm run typecheck` |
| EXTRACTION-001.2 | `apps/web/lib/plugins/registry-provider-ownership.test.ts`, `registry.test.ts`; `orchestrator/executor/executor_resume_test.go` plugin-managed cases |
| EXTRACTION-001.3 | `task/repository/sqlite/retired_azure_devops_*_test.go` (SQLite and PostgreSQL); `storeconformance` upgrade fixture |
| EXTRACTION-001.4 | `node scripts/validate-public-docs.mjs`; `python3 scripts/lint-spec-files.py --all` |
| EXTRACTION-001.5 | `plugin-registry` node tests |

## E2E tests

| Flow | AC | File and project |
| --- | --- | --- |
| Removed routes and reservations released | EXTRACTION-001.1, 001.2 | `apps/web/e2e/tests/plugins/bitbucket-plugin-contract.spec.ts` stays green; removed Azure specs deleted |

## Work orders

- [ ] [Task 08: Remove the built-in integration from Kandev](task-08-core-removal.md)
- [ ] [Task 09: Registry entry and public docs](task-09-registry-entry.md)

## Verification results

Recorded in each work order's Results section.

## Risks

- **Removal size.** K-A deletes about 17k lines plus tests across backend and
  web. The inventory check and a green CI are the review aids; the PR body
  lists the inventory.
- **Release ordering.** The plugin cannot pin `min_kandev_version` to the
  removal until K-A is released. Until the registry lists the plugin, users
  install it from the built package.
- **Executors.** Docker, SSH, and Kubernetes need the public HTTPS broker
  address for private Azure repos, as for Bitbucket. The plugin README
  documents this; it is not solved in this initiative.

## Open questions

- Whether the maintainer wants the secret reveal fence reported as a separate
  security issue now. The design here does not depend on it.
