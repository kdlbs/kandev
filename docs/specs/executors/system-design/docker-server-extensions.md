---
status: current
system: executors
requirements:
  - REQ-EXECUTORS-DOCKER-SERVER-EXTENSIONS-001
created: 2026-10-24
owners:
  - kandev
---

# Docker Server Dependency Extension Design

## Purpose and boundary

Use the published server image as the parent of a small Dockerfile that installs
one optional OS client at build time. The existing base entrypoint starts as
root to repair `/data` ownership, then launches Kandev and its Local or Worktree
agents as `kandev`. The example leaves this contract intact. It does not replace
the image configured for the Local Docker executor.

## Requirement mapping

| Acceptance criteria | Design section |
| --- | --- |
| AC-EXECUTORS-DOCKER-SERVER-EXTENSIONS-001.1 | Documentation ownership |
| AC-EXECUTORS-DOCKER-SERVER-EXTENSIONS-001.2 | Derived image |
| AC-EXECUTORS-DOCKER-SERVER-EXTENSIONS-001.3, AC-EXECUTORS-DOCKER-SERVER-EXTENSIONS-001.4 | Disposable verification |

## Documentation ownership

`docs/public/docker.md` owns the operator procedure. `docs/images.md` explains
image versus volume persistence and links to the public guide instead of
maintaining a competing source-build or `USER` recipe. The root Dockerfile and
`docker-entrypoint.sh` remain the authority for release packaging and startup.

## Derived image

`examples/docker-server-extension/Dockerfile` pins the Kandev 0.98.0 manifest
digest and requests Debian Bookworm's `postgresql-client-15=15.19-0+deb12u1`.
It retains the inherited entrypoint. The base digest fixes Kandev image bytes;
the signed Debian repository and transitive packages can still vary or stop
offering that version. One amd64 build does not prove an arm64 build, and these
glibc packages do not make host SDKs or musl binaries portable.

## Disposable verification

Build the example, launch a fresh loopback-bound server with a labelled empty
`/data` volume, and inspect server health and startup errors. In a disposable
Local or Worktree task, ask the running agent to execute the guide's identity,
tool, path, and cache commands through its command tool. Recreate only that
server with the same image and volume; a new agent session should read the
first session's marker. A deliberately unavailable package version should
fail during build. Remove only the test container and volume afterward.

Task-terminal and inherited-entrypoint shells are useful comparisons but are
different processes from the agent. A task-private synthetic ACP fixture can
verify the actual launched child environment without proving third-party
provider authentication or Worktree-specific execution; state those gaps when
reporting validation.

See the [requirements](../requirements/docker-server-extensions.md) and
[work order](../../../plans/docker-server-extensions/task-01-document-and-validate.md).
