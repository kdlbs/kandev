---
id: "06-plugin-pin-and-floor"
title: "Finalize the plugin pin and minimum Host version"
status: blocked
wave: 6
depends_on:
  - "05-ci-audit-and-pr-records"
plan: "plan.md"
requirements:
  - REQ-COSTS-USAGE-IMPORT-005
acceptance_criteria:
  - AC-COSTS-USAGE-IMPORT-005.4
system_design:
  - ../../specs/costs/system-design/usage-history-import.md
---

# Task 06: Finalize the plugin pin and minimum Host version

## Summary

Point the plugin at the SDK commit on core `main`. Today it names `9a751dd`, a branch commit whose SDK tree (`a9a0a09`) still has the write API that core removed (current tree `1b3b88d`). Then declare the minimum Host. This stays blocked until the user merges core.

## In scope

Part A, after #3660 merges:
- Set `.kandev-sdk-ref` to the squash commit on core `main`. Update the pin and tree mentioned in the plugin `README.md`.
- Check out the core merge as the sibling `../kandev`, and run the commands below.
- Rerun the installed-Host smokes, then push.

Part B, after a core release containing that commit exists:
- Add `min_kandev_version` to `manifest.yaml` with that release, and satisfy `apps/backend/internal/plugins/manifest/min_version_policy.go`.

## Out of scope

- Tagging or publishing 0.5.0. That needs `features.usageHistory` on in `prod` and a user decision.

## Acceptance

- Plugin CI passes against the pin on core `main`.
- Packaging validates the manifest with `min_kandev_version` set.
- The release version isn't guessed. If no release exists yet, leave Part B `blocked`.

## Verification

```bash
(cd "$PLUGIN_ROOT" && make test)
(cd "$PLUGIN_ROOT" && make package)
(cd "$PLUGIN_ROOT" && make verify-package)
(cd "$PLUGIN_ROOT" && sh scripts/verify-release-version.sh)
```

For the smokes, follow the Host E2E section of the plugin `README.md` with retries disabled.

## Files likely touched

- `.kandev-sdk-ref`
- `manifest.yaml`
- `README.md`

## Dependencies

Task 05 and the core merge.

## Risks

- `test/` includes a helper that satisfies the older reader interface. It may need removing once the pin moves.

## Parallelism

`sequential`

## Inputs

- `apps/backend/internal/plugins/manifest/distribution.go`.
- `docs/public/plugins-authoring.md`.

## Results

Pending.
