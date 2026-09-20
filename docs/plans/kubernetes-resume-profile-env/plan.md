---
created: 2026-09-20
status: done
requirements:
  - REQ-EXECUTORS-K8S-FAILURE-RECOVERY-001
system_design:
  - ../../specs/executors/system-design/kubernetes-failure-recovery.md
legacy_specs: []
---

# Kubernetes Resume Profile Environment Repair

## Overview

Record the already implemented resume-environment regression fix and its
verification. The executor system owns both retained-runtime recovery and
profile environment construction. The user authorized PR publication and
required fixups; merging and deployment remain separate actions.

## Scope and technical approach

Restore environment definitions from the runtime inventory's recorded executor
profile before launch-environment construction. Preserve workload/storage
snapshots and the existing secret-resolution checkpoint. Missing profiles allow
retained recovery; lookup errors and foreign ownership fail closed.

This repair does not change provider mode selection, resource cleanup, schemas,
or rendered UI. The existing recovery specifications retain their draft status.

## Tests and acceptance

- Criteria .6/.8: `TestKubernetesResumeRestoresRecordedProfileEnvironment`.
- Subsequent-start profile identity: `TestKubernetesResumePreservesProfileForExistingWorkspaceStart`.
- Criterion .7: `TestKubernetesResumeProfileEnvironmentFailures`.
- Full executor package and focused Kubernetes recovery race tests.
- Isolated synthetic acceptance: same-Pod Stop/Resume, lost-Pod replacement with
  retained workspace, missing provider rollout fallback, and idempotent replay
  of previous submissions without manual environment overrides. Independent
  database baseline and submission hashes remained unchanged. Temporary test
  resources were removed. No browser layout change requires new screenshots.

## Work orders

- [x] [Restore recorded profile environment](task-01-restore-profile-env.md)

## Verification results

Implementation and acceptance passed before PR publication. PR follow-up adds
these linked records after the documentation coverage check identified the gap.
See the work order for commands and results; remote CI/review is tracked in the
PR rather than treated as complete here.

## Risks

Current profile environment edits affect future resumed processes. A deleted
profile cannot supply its former definitions; this is documented publicly.
