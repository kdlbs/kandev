---
status: active
system: executors
created: 2026-10-24
owners:
  - kandev
---

# Docker Server Dependency Extensions

## Scope

Operators need a reproducible way to add an optional OS client to the published
Kandev server image for Local and Worktree agents. This is documentation and an
example of the existing derived-image mechanism, not a change to executor
behavior or the separate Local Docker task image.

## Requirements

### REQ-EXECUTORS-DOCKER-SERVER-EXTENSIONS-001: Document a bounded server image extension

**Intent:** An operator can build, inspect, and recreate a server image with an
optional client while preserving Kandev's entrypoint, unprivileged agent runtime,
and persistent data ownership.

#### Acceptance criteria

- **AC-EXECUTORS-DOCKER-SERVER-EXTENSIONS-001.1:** The public Docker guide and
  repository image guide agree on release-bundle packaging, server versus task
  images, inherited entrypoint behavior, volume ownership, and image versus
  persistent-volume installs.
- **AC-EXECUTORS-DOCKER-SERVER-EXTENSIONS-001.2:** A buildable PostgreSQL-client
  example uses an immutable Kandev base digest and an explicit Debian package
  version. It adds no host mount, runtime installer, capability, or root agent
  execution. The guide states architecture, libc, repository, and transitive
  package limitations.
- **AC-EXECUTORS-DOCKER-SERVER-EXTENSIONS-001.3:** A disposable validation
  procedure checks the tool, effective user, HOME, PATH, and writable cache from
  an agent's actual command result before and after server recreation with the
  same disposable volume. It distinguishes task terminals and entrypoint shells
  from the agent process, and reports any unavailable real-agent coverage.
- **AC-EXECUTORS-DOCKER-SERVER-EXTENSIONS-001.4:** A missing requested package
  version causes a visible build failure; cleanup affects only labelled
  disposable containers and volumes.

## Out of scope

- Automatically installing optional dependencies, altering executor profiles,
  adding a new extension system, or changing Host and sandbox behavior.
- Guaranteeing a third-party provider CLI, its authentication, or every CPU
  architecture from a synthetic agent fixture or one amd64 build.

## Related documents

- [System design](../system-design/docker-server-extensions.md)
- [Implementation plan](../../../plans/docker-server-extensions/plan.md)
