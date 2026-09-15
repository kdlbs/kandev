---
id: "01-docker-network-reclamation"
title: "Reclaim stale Docker networks"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-001
acceptance_criteria:
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.3
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.6
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.7
system_design:
  - ../../specs/system-page/system-design/storage-maintenance-01.md
---

# Task 01: Reclaim stale Docker networks

## Scope

Add a Docker-network Storage provider that inventories bridge networks, classifies task ownership
fail-closed, records first-seen and quarantine state, and removes only an individually revalidated
eligible network. The default is dry-run analysis; enabling removal requires the existing dedicated
Docker-daemon acknowledgement. The provider runs under the shared maintenance lease and records the
capacity probe result in the maintenance run.

## Acceptance

- Storage analysis remains independent from policy and history loading while Docker-network census
  results are exposed with availability, classifications, candidates, and warnings.
- The Storage policy explains the disabled-by-default reclamation setting, thresholds, quarantine,
  revalidation, and Docker-host scope through localized desktop and mobile-capable controls.
- Read-only analysis runs while scheduled maintenance is disabled. A task-owned or connected network
  is never eligible; ambiguity remains retained; removal is only per network ID after quarantine and
  fresh inspection.

## Verification

```bash
cd apps/backend && go test ./internal/system/storage/docknet/... ./internal/backendapp/... -count=1
cd apps/web && pnpm run typecheck
KANDEV_E2E_CONTAINERS=1 pnpm e2e:raw -- --project=containers e2e/tests/docker/docker-network-reclamation.spec.ts
```

## Results

Implemented the Docker network provider, persisted ledger, lease/revalidation gates, capacity probe,
Storage UI, localized copy, public Docker operations guidance, focused backend tests, and a real-Docker
containers E2E scenario. No task-side Docker authority, daemon-wide network prune, Docker restart, or
daemon configuration mutation was added.
