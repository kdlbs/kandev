---
id: "13-plugin-release-readiness"
title: "Finalize the plugin SDK pin and compatibility"
status: cancelled
wave: 7
depends_on:
  - "06-import-e2e-docs"
  - "07-remove-host-write-api"
  - "12-close-pr-gates"
plan: "plan.md"
requirements:
  - REQ-COSTS-USAGE-HISTORY-004
acceptance_criteria:
  - AC-COSTS-USAGE-HISTORY-004.4
system_design:
  - ../../specs/costs/system-design/usage-history.md
---

# Task 13: Finalize the plugin SDK pin and compatibility

## Summary

Point the plugin at a durable SDK commit, and declare the minimum Host it supports. Today `.kandev-sdk-ref` names a PR-branch commit, and the manifest declares no minimum.

## In scope

- Set `.kandev-sdk-ref` to the SDK commit on core `main` after #3660 merges, and update the pinned tree in the PR body and README.
- Add `min_kandev_version` to `manifest.yaml` with the first core release containing the Host API, and satisfy `min_version_policy.go`.
- Rebuild all five platform archives, and record the checksum.

## Out of scope

- Publishing the plugin or marketplace entry, which needs user authorization and `/create-kandev-plugin`.

## Acceptance

- CI checks out the pinned commit from `main`, and `make test` passes against it.
- `plugin-pack` validates the manifest, and an older Host rejects or clearly explains the version requirement.

## Verification

```bash
(cd "$PLUGIN_ROOT" && make test)
(cd "$PLUGIN_ROOT" && make package)
(cd "$PLUGIN_ROOT" && make verify-package)
(cd "$PLUGIN_ROOT" && sh scripts/verify-release-version.sh)
```

## Files likely touched

- `.kandev-sdk-ref`
- `manifest.yaml`
- `Makefile`
- `README.md`
- `scripts/test-verify-package.sh`

## Dependencies

Tasks 06, 07, and 12. The final pin also requires #3660 to be merged.

## Risks

- The core release number is unknown until release. Do not guess it. This work order stays `blocked` until it exists.
- A pinned PR-branch commit is not guaranteed to stay reachable once its branch is gone, which is why this order matters.

## Parallelism

`sequential`

## Inputs

- `apps/backend/internal/plugins/manifest/min_version_policy.go` and `distribution.go`.
- `docs/public/plugins-authoring.md`.

## Results

Pending.

Superseded on 2026-11-01. Moved to [Task 06](../usage-history-merge-readiness/task-06-plugin-pin-and-floor.md).
