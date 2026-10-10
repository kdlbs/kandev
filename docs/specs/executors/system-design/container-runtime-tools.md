---
status: current
system: executors
requirements:
  - REQ-EXECUTORS-CONTAINER-TOOLS-001
---

# Published Container Runtime Tools Design

## Purpose and boundaries

This design supplies the process-inspection dependency for agents executing
inside Kandev's published control-plane container. It applies to the Local
executor in that container; the separate Docker executor's configured image is
not automatically replaced. See the [requirements](../requirements/container-runtime-tools.md).

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-EXECUTORS-CONTAINER-TOOLS-001 | Image dependency, Runtime identity, Verification |

## Image dependency

The final runtime stage in the root `Dockerfile` installs Debian's `procps`
package through its existing `apt-get install --no-install-recommends` list.
It provides `/usr/bin/ps` and its package-managed dependencies. The temporary
`apt-keys` stage does not need this runtime dependency.

`Dockerfile.universal` already derives from `BASE_IMAGE`. Rebuilding it from
the corrected base carries the dependency without a second install list.
Other derivatives receive the tool when rebuilt from that corrected base;
an existing pinned digest remains unchanged.

The Droid provider in `apps/backend/internal/agent/agents/droid_acp.go` keeps
its existing command and runtime contract. No ACP adapter, process supervisor,
permission option, or credential path changes.

## Runtime identity and recovery

The base image retains its `tini` entrypoint and `docker-entrypoint.sh` privilege
drop through `gosu`. The universal image retains `USER kandev`. Both continue
to run agent commands as UID 1000 with the existing image PATH. Debian's `ps`
is an ordinary executable, not a setuid program.

Recovery for an affected deployment is to recreate the container using an
image containing the dependency and retain its existing data volume. This
does not update Droid, rewrite authentication state, or change saved profiles.
The dependency does not bypass policies that prohibit process inspection.

## Verification

A focused `scripts/test-docker-runtime.sh IMAGE [IMAGE ...]` smoke entry point
runs against real image contents, under each image's inherited entrypoint.
It uses disposable data, disables networking, and verifies UID 1000, the exact
parent-PID probe, and the resulting parent identity. It runs commands in real
child shells with bounded timeouts, including the ordered issue sequence,
a command lasting longer than the supervisor's two-second poll interval,
and an intentional stderr/nonzero-exit case. It asserts output and status.

This script tests the image prerequisite and shell behavior. It must not claim
to be an authenticated Droid or ACP end-to-end test. It must not vendor or
reimplement Factory's supervisor as production behavior, inspect host agent
credentials, or require a provider account.

The repair plan records a separate causal reproduction using the unchanged
supervisor extracted from integrity-verified Droid binaries. A disposable,
authenticated Droid session can additionally exercise the original prompt.
Record the actual CLI version and image digest, inspect individual Execute
results, and report unavailable credentials as an unrun live check.

## Documentation and delivery

Update the base-image tool inventory in `docs/public/docker.md` and
`docs/images.md` when implementation lands. Keep the public explanation scoped
to the included tool and inherited availability, without promising a fix for
every SIGKILL cause. This routine dependency correction needs no new ADR.

See the [fix plan](../../../plans/droid-execute-runtime-dependency/plan.md) and
[work order](../../../plans/droid-execute-runtime-dependency/task-01-runtime-process-tools.md).
