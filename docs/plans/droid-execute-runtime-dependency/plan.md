---
created: 2026-09-29
status: done
requirements:
  - REQ-EXECUTORS-CONTAINER-TOOLS-001
system_design:
  - ../../specs/executors/system-design/container-runtime-tools.md
legacy_specs: []
---

# Implementation Plan: Droid Execute Runtime Dependency

## Overview

Install `procps` in the root runtime Dockerfile so Droid's foreground command
supervisor can inspect its parent. Universal inherits the correction. One
sequential work order adds a real-image regression smoke check, the dependency,
and the image documentation updates.

This repairs [Kandev #3617](https://github.com/kdlbs/kandev/issues/3617).
Implementation and delivery through PR merge were authorized on 2026-09-29.

## Confirmed diagnosis

The reporter's follow-up reproduces mixed foreground failures directly in
Droid 0.218.0, outside ACP, with zero cgroup OOM counters. The
[upstream report](https://github.com/Factory-AI/factory/issues/33) identifies
an owner-liveness probe that treats a failed `ps` command as owner death.
Neither report alone was accepted as proof about Kandev's image.

Investigation on 2026-09-29 established:

- The checkout and remote main were `320f050e12071342522a7ba0730af03e6ce1b386`.
  Published images label revision `60edf466cf935870a834a9396f5b69466569625f`.
  `Dockerfile`, `Dockerfile.universal`, and `docker-entrypoint.sh` have no diff
  between those revisions or the reporter's `c21f1f09ce34877195c4bec1cabec7513cb88523`.
- Actual base and universal Linux/amd64 containers lack both `ps` and the
  `procps` package. Under their normal entrypoints, UID/GID is 1000/999 and the
  exact probe exits 127. Transitive dependencies do not supply it.
- Official npm `@factory/cli-linux-x64` binaries 0.218.0 and 0.229.0 were
  downloaded and checked against npm's SHA-512 integrity metadata. Both report
  their expected versions and contain the same 602-byte supervisor script,
  SHA-256 `40175c344ae4f34d5fc4b5fd540734c91bb51ec5b0f7b0e2747eecf9686e904c`.
- A Python parent spawned that unchanged script through Bash, with
  `start_new_session=True`, the real parent PID, and a separate shell command.
  This preserves the owner and process-group relationship used by Droid's
  detached spawn. Twenty ordered repetitions of the reporter's seven commands
  produced the results below. The parent recorded each actual process status;
  this was not an agent's interpretation of a command failure.
- Bash tracing on a separate three-second control showed `ps` failing,
  `owner_is_alive` returning 1, and the supervisor issuing `kill -KILL` to its
  own process group. Direct shell controls succeeded. OOM counters stayed zero.
- Throwaway derivatives installing only `procps` plus its required
  `libproc2-0` dependency removed the failures. No security or credential
  changes were made. Debian installed `procps` version `2:4.0.2-3`, adding
  approximately 2.4 MB; `/usr/bin/ps` is mode 0755, owned by root.

| Actual image | Original ordered commands | With only procps added |
| --- | --- | --- |
| Base, normal entrypoint, UID 1000 | 140/140 SIGKILL | 140/140 exit 0 |
| Universal, normal entrypoint, UID 1000 | 140/140 SIGKILL | 140/140 exit 0 |

Both corrected images also passed the three-second command across a watchdog
poll and preserved an intentional exit 7 with stderr. One short command in
each original-image run produced output before being killed. Completion races
can explain the reporter's mixture of success, output-then-kill, and failure;
the exact historical race distribution was not reproduced or claimed.

### Immutable inputs

| Input | Identity |
| --- | --- |
| Base image index | `ghcr.io/kdlbs/kandev@sha256:33c9211aea048af542fb68da80088dc28ab3ed947378dd12f97d98e7d2d2bf93` |
| Base amd64 manifest | `sha256:d8165a5de70fce0c648c27434c0255c696bb4851cadd324d934e353b9d85ba1e` |
| Universal amd64 manifest | `ghcr.io/kdlbs/kandev@sha256:5a7558e4ed370652a8128b49d8d8701cd0639ba83b6648ca139e103d06c450de` |
| Droid 0.218.0 binary SHA-256 | `84fac4b23eee347278aca0f0ae216ec26b8a589bfda0044474a8daffc458fc13` |
| Droid 0.229.0 binary SHA-256 | `c942ae034201554a91df92b78c5c77351c353c1312b33159db895838ffa978d4` |

### Smallest reproduction

Run this against either original image above. It fails with `ps: command not
found`, status 127, after showing the normal unprivileged identity:

```bash
docker run --rm --network=none \
  ghcr.io/kdlbs/kandev@sha256:33c9211aea048af542fb68da80088dc28ab3ed947378dd12f97d98e7d2d2bf93 \
  bash -c 'id; ps -o ppid= -p "$$"'
```

For the causal signal reproduction, extract bytes from the leading newline
before `owner_is_alive() {` through the first NUL in a verified binary. The
extracted script must match the SHA-256 above. Inside a disposable container,
have a Python parent launch this exact argument shape:

```python
child = subprocess.Popen(
    ["/bin/bash", "-c", supervisor, "factory-execute-supervisor",
     str(os.getpid()), "/bin/bash", command],
    start_new_session=True, stdout=out_file, stderr=err_file,
)
```

Use disposable files for output, a ten-second wait bound, and a `finally`
cleanup that signals only this child's process group and reaps the child.
Separate process groups are essential: never run the group-killing supervisor
directly in the caller's own group. Test the seven commands individually in
order, `printf 'before-probe\n'; sleep 3; printf 'after-probe\n'`, and
`printf 'expected-stderr\n' >&2; exit 7`. Compare the original image with a
temporary derivative installing `procps`; keep credentials and live volumes
out of both. This is a diagnostic method, not a permanent supervisor fork.

## Scope

### In scope

- The final-stage dependency in the root `Dockerfile`.
- Real-image smoke coverage for both flavors and public tool inventory updates.
- [REQ-EXECUTORS-CONTAINER-TOOLS-001](../../specs/executors/requirements/container-runtime-tools.md),
  acceptance criteria .1 through .3, and its linked system design.

### Out of scope

- ACP, process supervision, provider updates, permission or credential changes.
- Native host, custom Docker image, and denied-`ps` sandbox remediation.
- Publishing releases, updating pinned Kubernetes images, and commenting on
  or closing the issue.

## Technical approach

Add `procps` once to the existing runtime apt list. Leave the universal recipe
unchanged and build it with the corrected base. Add
`scripts/test-docker-runtime.sh IMAGE [IMAGE ...]` as a bounded, credential-free
image smoke entry point using actual process observations. Follow
`scripts/test-kubernetes-worker-images.sh` for disposable resources and cleanup,
without adopting its Kubernetes scope. Update `docs/public/docker.md` and
`docs/images.md` to identify the base tool inherited by universal.

## Tests

| Acceptance | Evidence |
| --- | --- |
| .1 | Smoke script runs the exact `ps` invocation in a real child Bash process and checks its live parent PID on both images. Original images fail before the dependency change. |
| .2 | Smoke checks ordered shell commands, output, delay, stderr and nonzero status; the causal reproduction above exercises Droid's actual extracted supervisor. Authenticated coverage is separate below. |
| .3 | Smoke uses normal entrypoints, asserts UID 1000, disables networking, and adds no capabilities, privileged mode, security overrides or credential mounts. |

## End-to-end check and limitations

In a fresh container, Droid 0.218.0 reports its version but `droid exec --auto low
'Run pwd using Execute'` exits 1 with an authentication error. No credentials
were mounted or searched for. Full authenticated Execute/ACP coverage is unrun.

When credentials are explicitly available for a disposable test, run the
original ordered prompt through Droid and inspect each Execute result, not
just the CLI's final exit code. The [work order](task-01-runtime-process-tools.md)
contains the exact prompt. Browser E2E adds no evidence for this image dependency.
Linux/amd64 was tested; arm64 image execution is unverified. No full current-main
application rebuild or corrected universal inheritance build was performed in
the diagnostic phase. Those builds belong to implementation validation.

## Work orders

- [x] [Task 01: Supply and verify runtime process tools](task-01-runtime-process-tools.md)

## Verification results

Diagnostic comparisons are recorded above. The following design checks passed:

- `python3 scripts/list-docs.py validate`: 330 decisions and 1250 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- The catalog discovers both new executor documents.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: `covered`, with no errors,
  using the actual four documents and the planned production/test paths.
  Host Node was unavailable, so the local preflight ran as UID 1000 in the
  pre-existing CI runtime image, with a read-only checkout and no networking.
- `git diff --check` and an explicit inventory of the four untracked documents.

Implementation passed on Linux/amd64. The new smoke script failed on both
original images with probe exit 127 before the dependency edit. The actual
root Dockerfile then built successfully using published application binaries;
the unmodified universal recipe built from that corrected base. Both passed
`bash scripts/test-docker-runtime.sh kandev-3617-implementation:base kandev-3617-implementation:universal`
with UID 1000, parent identity, 140 ordered shell commands per image, delayed
probes, stderr and intentional exit 7. See the work order for exact image IDs
and commands. The host's existing nvm Node 24.18.0 toolchain was activated for
the implementation checks and normal commit hooks.

PR review clarified that the smoke requires locally available images. The work
order now pulls its two immutable registry inputs before running the red checks,
and the script documents that prerequisite. After removing the owned original
image references, both documented pull-then-smoke sequences reached the expected
missing-`ps` failure. Both locally built corrected images still passed the smoke.
Shell syntax, specification catalog/lint and diff checks passed again.

The branch was fast-forwarded to main `abc7a85f1aa16762f33aca7c8d1f8947a206f939`
before delivery; its unrelated worktree recovery change leaves both Docker
recipes, entrypoint and smoke script unchanged. PR CI/review and merge evidence
is tracked in the persistent task plan rather than treated as completed here.

Owned design-phase diagnostic image tags,
containers, temporary binary downloads, and logs were removed after recording
this evidence. The pre-existing CI image is retained. No live Kandev
data or pre-existing containers were modified, and no shared cache was pruned.

## Risks

- A missing dependency repair cannot make a denied or shadowed `ps` executable.
- Fast commands can beat the broken watchdog; one successful command is not
  proof of a healthy image. Keep the long control and ordered repetitions.
- Rebuilds need network access for apt and universal toolchain downloads.
- Reused bundle binaries validate packaging and tools, not current-main
  application compilation. Keep that distinction in the implementation results.
