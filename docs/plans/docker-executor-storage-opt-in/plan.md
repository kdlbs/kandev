---
created: 2026-11-19
status: complete
requirements:
  - REQ-EXECUTORS-STORAGE-001
system_design:
  - ../../specs/executors/system-design/docker-storage-bounds.md
legacy_specs: []
---

# Fix plan for Docker executor storage limits

## Root cause and reproduction

`buildHostConfig` calls `applyContainerStorageBounds` for both create methods.
The helper assigns `json-file` and its rotation options for every request.
It assigns a 2 GiB tmpfs at `/tmp` unless the request includes an equivalent
mount target. This behavior shipped in [PR #5147](https://github.com/kdlbs/kandev/pull/5147)
and accounts for the storage and logging regressions in
[issue #5240](https://github.com/kdlbs/kandev/issues/5240).

`TestContainerCreateSubmitsStorageBounds` reproduces the request for both normal
and interactive creation using an empty configuration and a fake Docker server.
For TDD, start with `TestNewContainerInheritsDockerStorageDefaults` in
`internal/agent/docker/storage_bounds_test.go`. It expects empty log options
and no injected `/tmp` mount from `buildHostConfig(ContainerConfig{}, nil, nil)`.
The current helper fails that expectation.

## Scope

Restore Docker and image defaults for new containers. Expose three optional
limits in the canonical Local Docker and Remote Docker profile editor. Update
the public executor reference in the same delivery as the implementation.

The fix applies on fresh creation. Existing containers keep their original
settings across reconnect. Persistent storage quotas, evidence retention,
and changes to daemon configuration are outside this fix.

## Technical approach

Task 01 implements and verifies the profile and launch contract. Task 02 uses
that contract in Settings and documents the operator behavior. The executor
system owns the contract because the settings belong to the resolved profile.

1. Store the three strings in `ExecutorProfile.Config`. Reuse the profile
   metadata projection from Docker network settings.
2. Parse `/tmp` size once at fresh launch with `RAMInBytes`. In the shared
   `buildHostConfig`, set only the supplied overrides. Docker parses the two
   log options and chooses its own driver.
3. Add the three fields to `DockerSections` and use the existing Save changes
   and discard actions.

| Runtime | Transport | Policy and proof |
| --- | --- | --- |
| Local Docker | Local Docker API | Project options from the selected profile. Probe request payloads and run the local daemon canary. |
| Remote Docker | Docker API through the SSH dialer | Use the same configuration builder and request options. Test the remote profile projection and the shared client. |
| Retained Docker container | Existing container reference | Preserve its ID and original storage settings. Verify the existing reconnect paths only reach creation on cold launch. |
| Driver that rejects rotation options | Selected Docker daemon | Surface Docker's launch error. Verify the create request leaves the driver empty and makes one attempt. |

## ASCII UI preview

**UI-01 Desktop profile editor**

Open Settings > Executors > a Local Docker or Remote Docker profile. The
existing Container networks card precedes the new card.

```text
Container networks

Storage limits
Temporary storage (/tmp) [       ]    Example 2g
Maximum log file size    [       ]    Example 100m
Maximum log files        [       ]    Example 3
Blank fields use Docker or image defaults. Applies to new containers.

                         [Reset]   [Save changes]
```

**UI-02 Phone profile editor**

Open the same profile from the settings index or a profile link.

```text
Storage limits
Temporary storage (/tmp)
[                          ]
Example 2g

Maximum log file size
[                          ]
Example 100m

Maximum log files
[                          ]
Example 3

[Reset]    [Save changes]
```

The settings page owns vertical scrolling. The save action stays in its
existing floating region with safe-area clearance on phones. Phone inputs
fill the card width. Use the existing coarse-pointer control sizes and text
rules. Field copy and spacing are illustrative. Order, blank-field semantics,
and reachability are required by AC-EXECUTORS-STORAGE-001.8.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| AC-EXECUTORS-STORAGE-001.3 | Review the local `tryReconnect` and remote recover/reattach paths. Run the focused existing Docker reconnect tests. |
| AC-EXECUTORS-STORAGE-001.4 | `TestNewContainerInheritsDockerStorageDefaults`, `TestContainerCreateSubmitsStorageBounds`, and `TestResolveExecutorConfig_DockerStorageProfileIsAuthoritative`. |
| AC-EXECUTORS-STORAGE-001.5 | `TestContainerCreateSubmitsStorageBounds` and `TestContainerLogRotationOnCompatibleLocalDaemon`. Check that the driver is inherited and only supplied options are sent. |
| AC-EXECUTORS-STORAGE-001.6 | `TestExplicitTemporaryMountIsNotShadowed` and `TestContainerTmpfsOnLocalDaemon`. |
| AC-EXECUTORS-STORAGE-001.7 | `TestBuildDockerContainerConfigCarriesStorageBounds` covers valid, blank, and malformed `/tmp` values. |
| AC-EXECUTORS-STORAGE-001.8 | `serialize-executor-config.test.ts`, `executor-profile-reset.test.tsx`, and the profile editor E2E cases below. |

## E2E tests

- `e2e/tests/settings/docker-storage-limits.spec.ts` in `chromium`. Save and
  reload values in each Docker profile type, then clear one value. Assert that
  an unrelated configuration key survives. Covers AC-EXECUTORS-STORAGE-001.8.
- `e2e/tests/settings/mobile-docker-storage-limits.spec.ts` in `mobile-chrome`.
  Change a value, discard the draft, then save an edit by touch. Reload to
  confirm persistence and check for horizontal overflow. Covers
  AC-EXECUTORS-STORAGE-001.8.

The browser tests use the existing mock backend for executor operations. The
daemon canary tests logging and mount behavior separately.

## Work orders

- [x] [Task 01 Docker profile and launch settings](task-01-profile-and-launch.md)
- [x] [Task 02 Profile editor and public reference](task-02-profile-editor.md)

## Investigation results

The existing Docker request and mount tests passed on `70875454752`. They
assert the unconditional defaults under investigation. The temporary
`TestNewContainerInheritsDockerStorageDefaults` overlay failed on both forced
settings. Local reconnect, Remote Docker reconnect, and Remote Docker recovery
tests passed.

The separate tmpfs and log-rotation canaries passed with Docker Engine
`26.1.5+dfsg1` and the local `postgres:16-alpine` image. The tmpfs test ran
without log options. The rotation test used the daemon's `json-file` driver.

A disposable Docker container accepted `--log-opt max-size=10m` and
`--log-opt max-file=2` with the logging driver omitted. Docker inspected it
with its `json-file` default and no `/tmp` tmpfs. With `journald`, Docker
returned `unknown log opt 'max-size' for journald log driver`.

## Verification results

Task 01 passed its profile projection, launch parsing, client request, and
container configuration tests. The independent tmpfs and rotation canaries
passed on Engine `26.1.5+dfsg1` with the `json-file` driver. The unsupported
log-option test passed under `-race`, and the backend binary built. Task 02
passed 26 focused profile tests, web typecheck and lint, i18n checks and
ratchet, desktop and mobile E2E tests, public-doc tests and validation. The
refreshed-profile and whitespace-normalization save regressions passed. The
post-clear config reload E2E assertion passed. The changed Go files passed
`lint-go-changed` with 0 issues.

## Risks

- Docker Engine checks driver compatibility at launch. A profile that can
  use rotation options on one daemon can fail on a daemon with a different
  driver or incompatible inherited options.
- Old containers keep their original tmpfs and logging driver until replaced
  through their normal lifecycle. The public reference identifies this boundary.
- On an opted-in profile, tmpfs still uses host memory and swap. File count
  alone has no effect on `json-file` rotation without an effective size limit.

[Requirements](../../specs/executors/requirements/docker-storage-bounds.md) · [System design](../../specs/executors/system-design/docker-storage-bounds.md)
