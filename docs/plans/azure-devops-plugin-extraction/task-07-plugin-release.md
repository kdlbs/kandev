---
id: "07-plugin-release"
title: "Packaged E2E and release"
status: in_progress
wave: 4
depends_on: 
  - "03-plugin-microsoft-sign-in"
  - "04-plugin-browse-screens"
  - "05-plugin-tasks-links"
  - "06-plugin-watches"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-001
acceptance_criteria:
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-001.1
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-001.4
system_design:
  - ../../specs/integrations/system-design/azure-devops-plugin.md
---

# Task 07: Packaged E2E and release

## Summary

Finish the packaged-host Playwright suite on desktop and mobile, pin `min_kandev_version` to the first Kandev release that contains the removal, and publish GitHub release `v0.1.0` with the package and checksums.

## In scope

- `ui/e2e/desktop-packaged-plugin.spec.ts`, `mobile-packaged-plugin.spec.ts` covering tasks 02 to 06 flows.
- `.kandev-sdk-ref` on the released Kandev commit; `min_kandev_version` set; `scripts/verify-release-version.sh` green.
- Release workflow run; README install steps from the package for the period before registry listing.

## Out of scope

- Registry listing (task 09).

## Acceptance

- The packaged plugin installs on the released Kandev version and the packaged E2E passes on both projects.
- The release assets contain the package and `checksums.txt`.

## Verification

Commands run from the Kandev repository root; the plugin worktree is its sibling.

```bash
(cd ../JnManso-kandev-plugin-azure-devops-main && npm run e2e)
(cd ../JnManso-kandev-plugin-azure-devops-main && make verify-package && scripts/verify-release-version.sh)
```

## Files likely touched

- `../JnManso-kandev-plugin-azure-devops-main/ui/e2e/`
- `../JnManso-kandev-plugin-azure-devops-main/manifest.yaml`
- `../JnManso-kandev-plugin-azure-devops-main/.kandev-sdk-ref`
- `../JnManso-kandev-plugin-azure-devops-main/CHANGELOG.md`

## Dependencies

Task 03-plugin-microsoft-sign-in, Task 04-plugin-browse-screens, Task 05-plugin-tasks-links, Task 06-plugin-watches

## Risks

- Blocked until the Kandev release that contains K-A exists (G3).

## Parallelism

`sequential`

## Inputs

- [System design](../../specs/integrations/system-design/azure-devops-plugin.md)
- `apps/backend/AGENTS.md`, `apps/web/AGENTS.md`, `docs/public/plugins-authoring.md`, and the `/create-kandev-plugin`, `/e2e`, and `/mobile-parity` skills where relevant

## Results

- 2026-11-06: packaged E2E implemented on plugin branch `feat/bootstrap` in commit `e3c5129`.
  - `make e2e` (`npm run e2e`) packages the plugin, starts a disposable Kandev host from the sibling checkout with `KANDEV_E2E_MOCK=true` and `KANDEV_AZURE_DEVOPS_PLUGIN_MOCK=true`, installs the package, and runs Playwright with a desktop and a Pixel 5 project. `KANDEV_PLUGIN_E2E_URL` runs it against an existing fresh host.
  - Desktop covers:
    - connecting with a PAT;
    - Microsoft sign-in through the mock authority;
    - browsing work items and pull requests;
    - starting a linked task from a work item;
    - linking a pull request from the Link menu, with its status indicator;
    - creating and running a watch once per item;
    - the disabled watch reason.
  - Phone covers settings without horizontal overflow, the watch editor drawer, the disabled reason, and work-item browsing.
  - README and CHANGELOG describe the complete plugin and its known differences.
- Verification: `make e2e` passes 11 of 11, twice in a row. `make test`, `make verify-package-host`, and `npm audit` pass.
- Blocked (G3):
  - `min_kandev_version` and `.kandev-sdk-ref` can only be pinned to the first Kandev release that contains the removal (K-A).
  - `scripts/verify-release-version.sh`, the release workflow run, and GitHub release `v0.1.0` follow that release.
