---
id: "04-publish-and-prove"
title: "Wire release channels and packaged evidence"
status: complete
wave: 4
depends_on:
  - "03-build-runtime-assets"
plan: "plan.md"
requirements:
  - REQ-RELEASE-COMPACT-RUNTIME-001
  - REQ-RELEASE-COMPACT-RUNTIME-002
  - REQ-RELEASE-COMPACT-RUNTIME-003
acceptance_criteria:
  - AC-RELEASE-COMPACT-RUNTIME-001.1
  - AC-RELEASE-COMPACT-RUNTIME-001.2
  - AC-RELEASE-COMPACT-RUNTIME-002.1
  - AC-RELEASE-COMPACT-RUNTIME-002.2
  - AC-RELEASE-COMPACT-RUNTIME-002.3
  - AC-RELEASE-COMPACT-RUNTIME-003.1
  - AC-RELEASE-COMPACT-RUNTIME-003.2
  - AC-RELEASE-COMPACT-RUNTIME-003.4
  - AC-RELEASE-COMPACT-RUNTIME-003.5
  - AC-RELEASE-COMPACT-RUNTIME-003.6
system_design:
  - ../../specs/release/system-design/compact-runtime-distribution.md
---

# Task 04: Wire release channels and packaged evidence

## Summary

Atomically give existing Stable TAR and Windows ZIP names to compact archives
after the resolver, Desktop validator, and staged assets pass their checks.
Package the normal Desktop installer and updater from the standard archive and
move both Docker architectures to the full archives in the same workflow edit.
Document both choices and verify actual packaged remote launches.

## In scope

- Rename staged compact archives to the existing Stable TAR/ZIP names and
  publish those, the `-full` variants, helper assets, and sidecars together.
  Refuse to overwrite an existing same-named helper asset with different bytes.
  Make `scripts/release/package-npm-runtime.sh` copy the Stable manifest
  beside `bin/`.
- Keep npm Nightly and container inputs complete. Confirm the tap and Scoop
  consume the standard URLs. Change both Docker build contexts to `-full`,
  keep Desktop on the default standard archive, and confirm winget/Chocolatey
  consume the compact Windows ZIP. Make Desktop preparation and shell
  validation copy and accept the host binaries plus root manifest without
  helpers. On macOS sign only the host binaries; keep Windows host signing and
  the existing single signed installer/updater artifact and `latest.json`
  track. Test an update from older complete resources to slim resources.
  Add a manifest assertion to the in-repo Homebrew formula template and
  coordinate retirement of the external tap's foreign-binary audit exception
  with its first compact formula.
- Add channel contract tests, selected Docker/SSH packaged E2E scenarios
  using a verified cache or full offline bundle. Update `backend.ts` to launch
  the test's packaged bundle with its manifest and **without**
  `KANDEV_AGENTCTL_LINUX_BINARY`; seed the verified cache and assert the
  selected path. Apply the same no-override rule in `kubernetes-tools.ts` for
  a Kubernetes compact-bundle scenario. The resolver integration tests own
  first-fetch proof before the GitHub Release exists.
- Add packaged Desktop smoke coverage: local app launch requires no fetch;
  a remote launch selects a verified preseeded helper without a path override.
  Assert that missing Desktop manifest/resources fail before publication.
- Record a same-build size report for standard, full, and helper assets.
- Update public install, offline, service, and troubleshooting guidance and
  release/engineering instructions affected by the new artifact matrix.
  Include `github.com` and the redirected Release asset CDN in firewall
  guidance. At cutover, set the proposed ADR to accepted, supersede the old
  Homebrew ADR, and update its acceptance criterion to historical wording.

## Out of scope

- Adding a separate full offline Desktop installer/updater track or slimming
  container and Nightly distribution.

## Acceptance

- Each Stable CLI channel, including winget and Chocolatey, installs the
  standard layout with its manifest. The normal Desktop installer and updater
  carry that same slim runtime on the existing update track; containers and
  Nightly still install complete helpers.
- A slim Desktop app starts and runs local sessions without a fetch, and an
  older complete Desktop app remains launchable through the update transition.
- A packaged standard install starts locally without a fetch and launches a
  Docker or SSH remote from its verified cache; a full install performs the
  same launch without release-asset access. Local HTTP integration tests
  prove that an empty standard cache fetches only the selected helper. The
  E2E fixture must prove that no helper override bypasses the cache.
- Publication checks block a missing channel artifact, and release guidance
  states the network/offline tradeoff with measured archive sizes.

## ASCII UI preview

This work order does not change the panel built in Task 02. Packaged E2E
checks the existing `UI-01` preview in the [plan](plan.md#ascii-ui-preview):

```text
Preparing environment
  [spinner] Downloading remote helper (linux/amd64)
```

The phone and desktop layouts keep the same inline progress row; this task
proves the packaged wiring, not a new interaction.

## Verification

```bash
python3 .github/scripts/release-workflow-contract_test.py
node --test scripts/release/publish-npm.test.mjs scripts/release/nightly-release.test.mjs scripts/release/update-scoop-bucket.test.mjs
bash scripts/release/runtime-bundle.test.sh
bash scripts/release-desktop.test.sh
(cd apps/desktop/src-tauri && cargo test --features desktop-runtime)
(cd apps && pnpm --filter @kandev/desktop e2e)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/session/mobile-remote-helper-progress.spec.ts)
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --shards 1 --project containers tests/docker/docker-launch.spec.ts)
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --shards 1 --project containers tests/ssh/launch-task.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Files likely touched

- `.github/workflows/release.yml`
- `.github/scripts/release-workflow-contract_test.py`
- `scripts/release/publish-npm.sh`
- `scripts/release/package-npm-runtime.sh`
- `scripts/release/publish-npm.test.mjs`
- `scripts/release/nightly-release.test.mjs`
- `scripts/release/update-scoop-bucket.test.mjs`
- `scripts/release/kandev.rb`
- `scripts/release/update-homebrew-tap.sh`
- `scripts/release/prepare-desktop-runtime.sh`
- `scripts/release/verify-desktop-runtime.sh`
- `scripts/release-desktop.test.sh`
- `apps/desktop/e2e/desktop-launch-smoke.test.mjs`
- `apps/desktop/AGENTS.md`
- `apps/cli/bin/native-shim.js`
- `apps/web/e2e/fixtures/backend.ts`
- `apps/web/e2e/fixtures/kubernetes-tools.ts`
- `apps/web/e2e/fixtures/compact-runtime.ts`
- `apps/web/e2e/tests/docker/docker-launch.spec.ts`
- `apps/web/e2e/tests/ssh/launch-task.spec.ts`
- `docs/public/release-process.md`
- `docs/public/use-kandev.md`
- `docs/public/windows-support.md`
- `docs/decisions/2026-08-05-homebrew-remote-helper-audit.md`
- `docs/decisions/2026-09-23-compact-runtime-and-remote-helper-assets.md`
- `docs/specs/release/requirements/homebrew-core.md`
- `AGENTS.md`
- `.agents/skills/release/SKILL.md`

## Dependencies

Task 03, which depends on Task 02. No standard channel may ship before the
resolver and canonical assets are both verified.

## Risks

- The tap formula is published in a separate repository and must match the
  new archive contents at cutover.
- CI runners may not cover every remote CPU/OS combination; format/signature
  and resolver tests cover the remaining targets.
- Desktop packaging and signing must carry the root manifest while preserving
  the existing updater identity; otherwise a slim update can fail at startup.

## Parallelism

`sequential`

## Inputs

- All compact-runtime requirements, the channel-consumption design, and the
  superseding ADR.
- Existing release workflow, npm/desktop contract tests, and container
  project Docker/SSH scenarios.

## Results

Completed and verified on 2026-09-25. Stable release archives and Desktop use
the standard bundle. Full command-line archives remain available for offline
use, Docker still uses full archives, and Nightly remains full. Channel
contract, package, Desktop, resolver, and release-script checks passed. The
mobile helper-progress E2E passed (2 tests), Docker passed (12 tests; the
externally removed-container case remains a documented backend FIXME), and SSH
passed (9 tests). Go launcher/resolver tests, Desktop Rust tests and packaged
smoke, public documentation validation, specification validation, and
formatting checks passed. The Homebrew formula checks the standard manifest,
and the tap updater removes its known foreign-binary audit exception with the
first compact formula update. Ruby syntax validation was unavailable because
Ruby is not installed; the formula contract is covered by the release workflow
test. No release was published and no commit was created.
