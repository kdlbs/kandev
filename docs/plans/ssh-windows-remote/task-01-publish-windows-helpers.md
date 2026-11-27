---
id: "01-publish-windows-helpers"
title: "Publish Windows agentctl remote helpers"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-WINDOWS-002
acceptance_criteria:
  - AC-EXECUTORS-SSH-WINDOWS-002.1
  - AC-EXECUTORS-SSH-WINDOWS-002.2
  - AC-EXECUTORS-SSH-WINDOWS-002.3
  - AC-EXECUTORS-SSH-WINDOWS-002.4
system_design:
  - ../../specs/executors/system-design/ssh-windows-remote.md
  - ../../specs/release/system-design/compact-runtime-distribution.md
---

# Task 01: Publish Windows agentctl Remote Helpers

## Summary

Build, publish, bundle, and resolve the `windows/amd64` helper. The fork also
adds `windows/arm64`, which is out of scope (F12), so remove it. Do not change
SSH executor platform acceptance. This work order is PR A in full.

## In scope

- Run `git checkout 0191092a3 -- <file>` for each of these files:
  - `.github/scripts/e2e-tests-workflow-contract_test.py`
  - `.github/scripts/release-workflow-contract_test.py`
  - `.github/workflows/e2e-tests.yml`
  - `.github/workflows/release.yml`
  - `Dockerfile`
  - `Makefile`
  - `apps/backend/internal/launcher/bundle.go`
  - `apps/backend/internal/launcher/bundle_test.go`
  - `apps/backend/internal/agent/runtime/lifecycle/remote_helper_manifest.go`
  - `apps/backend/internal/agent/runtime/lifecycle/remote_helper_manifest_test.go`
  - `apps/backend/internal/backendapp/ssh_resolver_wiring_test.go`
  - `apps/desktop/AGENTS.md`
  - `apps/desktop/e2e/desktop-launch-smoke.mjs`
  - `apps/desktop/e2e/desktop-launch-smoke.test.mjs`
  - `apps/desktop/src-tauri/src/backend.rs`
  - `apps/web/e2e/fixtures/compact-runtime.ts`
  - `scripts/check-agentctl-deps.sh`
  - `scripts/check-make-shells`
  - `scripts/release-desktop.test.sh`
  - All eight `scripts/release/*` files changed by the fork commit
- In `apps/backend/Makefile`, apply only the hunks for the `GO_WINDOWS_AMD64`
  prefix, `.PHONY`, `build-agentctl-remote`, and the
  `build-agentctl-windows-amd64` target. Task 05 owns the `test-windows`
  hunk. Use
  `git diff 0191092a3^ 0191092a3 -- apps/backend/Makefile > /tmp/mk.patch`,
  edit the patch, and run `git apply`.
- Port the `agentctl_resolver.go` changes: `.exe` naming in `cachePath`,
  `agentctlBinaryName`, and `agentctlBinaryCandidates`.
  - Define `sshRemoteGOOSWindows` beside `sshRemoteGOOSLinux` in
    `executor_ssh_operations.go`.
  - Keep `defaultReleaseBaseURL` at `kdlbs/kandev` (F1).
  - Do not add `KANDEV_REMOTE_HELPER_BASE_URL` (F2).
- Move `TestAgentctlResolverManifestDownloadsWindowsExeHelperToCache` from
  `executor_ssh_winremote_deploy_launch_test.go` into the resolver test file.
- Add a resolver test asserting that the default release base URL is the
  `kdlbs/kandev` URL.
- Remove every `windows/arm64` addition from the checked-out files (F12):
  - The `GO_WINDOWS_ARM64` prefix, the `build-agentctl-windows-arm64` target,
    and its `.PHONY` entry.
  - The arm64 build, `cmp`, and `file` lines in `release.yml`.
  - Its `COPY` and `chmod` lines in `Dockerfile`, and its `Makefile`
    `runtime-bundle` line.
  - The `e2e-tests.yml` target and its contract-test strings.
  - The Go, JavaScript, Rust, and shell table entries and their test fixtures.
  - The `windows/arm64/0` target in `check-agentctl-deps.sh`.
- Narrow `HELPER_ASSET_PATTERN` to `windows-amd64\.exe`. Size the Rust arrays at
  `[...; 5]`. Change "all six" to "all five" in `apps/desktop/AGENTS.md` and
  `check-agentctl-deps.sh`.
- In `docs/specs/release/system-design/compact-runtime-distribution.md`, change
  each "four helpers" statement to five helpers. Link the Windows requirement.
- Add `TestRemoteHelperManifestRejectsWindowsARM64Record`. A manifest with a
  `windows/arm64` record fails validation as an unsupported platform.

## Exclusions

- SSH platform acceptance, PowerShell, and any executor behavior (task 02).
- The `test-windows` Makefile hunk and CI test step (task 05).
- Authenticode signing of remote helpers.
- Any `windows/arm64` helper.
- The fork commit `32e795de6 release: 0.102.1`.

## Acceptance conditions

1. All helper tables in Go, JavaScript, Rust, shell, Docker, and CI list exactly
   five helpers. The fifth is `windows/amd64`, and nothing names a
   `windows/arm64` asset. Release contract tests assert the Windows asset.
2. The resolver downloads a Windows helper from a manifest into a cache file
   named `agentctl.exe` and verifies its checksum. The default URL is
   `kdlbs/kandev`.
3. SSH still rejects `windows/*` hosts with the existing unsupported-platform
   error.

## Verification

```bash
make -C apps/backend build-agentctl-windows-amd64
file apps/backend/bin/agentctl-windows-amd64.exe
! git grep -n -i 'windows-arm64' -- .github Dockerfile Makefile apps/backend/Makefile apps/backend/internal/launcher scripts apps/desktop apps/web/e2e
(cd apps/backend && go test -trimpath -tags fts5 -count=1 -run 'RemoteHelper|AgentctlResolver|RequiredAgentctlRemoteHelpers|ValidateRuntimeBundle|SSHResolver|RequireSupportedRemotePlatform' ./internal/agent/runtime/lifecycle ./internal/launcher ./internal/backendapp)
python3 .github/scripts/e2e-tests-workflow-contract_test.py
python3 .github/scripts/release-workflow-contract_test.py
node --test scripts/release/remote-helper-assets.test.mjs scripts/release/publish-npm.test.mjs scripts/release/existing-release.test.mjs apps/desktop/e2e/desktop-launch-smoke.test.mjs
bash scripts/release/runtime-bundle.test.sh
bash scripts/release-desktop.test.sh
bash scripts/check-make-shells
bash scripts/check-agentctl-deps.sh
(cd apps/desktop/src-tauri && cargo test)
python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all
bash scripts/lint-go-changed $(git diff --name-only origin/main -- 'apps/backend/*.go')
```

Compare each failure with the same command on `origin/main`. Report only new
failures as regressions. The `gpg`-dependent release contract cases and the two
shell bundle tests fail on this macOS host at baseline.

## Likely files and risks

- Release workflow edits affect publication. Keep each change additive beside
  its Linux and macOS counterpart.
- The Go and Rust manifest validators require an exact helper count. Desktop
  and runtime ship together, so an old manifest is not read by a new binary.
- Size: about 63 MiB uncompressed in complete bundles and the image, and
  about 28 MiB for the compressed asset. Include the measured sizes in the PR
  body.

## Results

Completed on 2026-11-22 on macOS arm64 with Go 1.27.4.

Changes beyond the fork:

- The fork's `windows/arm64` additions are removed from every table.
- `agentctlExecutableName` replaces three repeated `.exe` checks.
- The resolver gates on `requireRemoteHelperPlatform`, the published helper
  table, instead of the SSH executor gate. The resolver therefore serves
  `windows/amd64` in this PR while SSH still rejects Windows hosts. The design's
  "Platform detection and persistence" section records the split.
- `reportedRemotePlatform` gives the gates the same raw uname detail.
- The host fallback candidate is `agentctl.exe` on a Windows control plane.

| Command | Result |
| --- | --- |
| `make -C apps/backend build-agentctl-windows-amd64`, then `file` | PE32+ x86-64 |
| arm64 `git grep` guard | Clean |
| Targeted `go test` on `lifecycle`, `launcher`, and `backendapp` | Passed. Red first: the `.exe` cache name and binary name tests failed |
| `e2e-tests-workflow-contract_test.py` | OK |
| `release-workflow-contract_test.py` | Two errors, both because `gpg` is absent; the same errors occur at baseline |
| Node release and Desktop smoke tests | 44 passed. One failure is the Desktop render test, which needs a built `dist` |
| `runtime-bundle.test.sh`, `check-make-shells`, `check-agentctl-deps.sh` | Passed |
| `release-desktop.test.sh` | Failed at updater signature verification because `cargo` is absent |
| `cargo test` (`apps/desktop/src-tauri`, `RUSTUP_TOOLCHAIN=1.90.0`) | Passed: 119 unit tests, 0 failed. The crate requires rustc 1.90, so the default 1.88 toolchain fails |
| `list-docs.py validate`, `lint-spec-files.py --all` | Passed |
| `lint-go-changed` | 0 issues, after adding the `agentctlExecutable` constant for `goconst` |

PR review (2026-11-27) found that a backfill of the latest pre-Windows Stable
tag would fail. The workflow built the Windows helper, the tag's helper script
rejected it, and the workflow-revision verifier required five manifest records.
`prepare` now sets `windows_helper` from the tag's helper table, and the
helper build, packaging, staged asset checks, and immutable release
verification follow it. `test_pre_windows_backfill_publishes_the_tag_helper_set`
and the pre-Windows case in `existing-release.test.mjs` failed before the fix.
The `v0.104.0` helper script rejects a Windows input and accepts the four-helper
set.
