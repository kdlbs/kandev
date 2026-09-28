---
requirements:
  - REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-001
system_design:
  - ../../specs/system-page/system-design/storage-maintenance-01.md
status: complete
---

# Implementation plan: Docker network reclamation

Docker bridge networks consume finite daemon address pools. This delivery adds a read-only census
and an opt-in, fail-closed reclaimer to Storage maintenance. It retains all networks with connected
containers, active task ownership, ambiguous ownership, or insufficient stable observation. Eligible
networks enter a persisted quarantine period and are revalidated immediately before per-ID removal.

The design extends the existing Storage Maintenance page and activity coordinator. It does not run a
daemon-wide prune, edit Docker configuration, restart Docker, or give task sessions Docker access.
Operators use the documented host-level pool expansion only when reclamation has proven insufficient.

## Work orders

- [x] [Task 01: Reclaim stale Docker networks](task-01-docker-network-reclamation.md)
