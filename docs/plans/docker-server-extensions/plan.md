---
created: 2026-10-24
status: in_progress
requirements:
  - REQ-EXECUTORS-DOCKER-SERVER-EXTENSIONS-001
system_design:
  - ../../specs/executors/system-design/docker-server-extensions.md
legacy_specs: []
---

# Implementation Plan: Docker Server Dependency Extension Example

## Scope

Issue [#4424](https://github.com/kdlbs/kandev/issues/4424) asks for a supported
way to supply optional tools to agents in a Docker-hosted Kandev server. This
focused increment reconciles the two existing image guides, adds one pinned
PostgreSQL-client derivative, and documents a disposable verification journey.
It uses the current entrypoint and agent launch path; no runtime implementation
or automatic extension mechanism is planned.

## Delivery

One [work order](task-01-document-and-validate.md) owns
`docs/images.md`, `docs/public/docker.md`, and
`examples/docker-server-extension/Dockerfile`. The [requirement](../../specs/executors/requirements/docker-server-extensions.md)
and [design](../../specs/executors/system-design/docker-server-extensions.md)
record the boundary and evidence standard.

## Validation and open gates

The preceding example, which pinned PostgreSQL 15.19, built on linux/amd64. A
fresh task-owned server and entrypoint shell showed UID 1000, `HOME=/data/home`,
`/usr/bin/psql`, writable cache ownership, and a marker retained across server
recreation. An invalid package version failed visibly during build. Independent
QA also observed a task-private synthetic Local ACP agent execute real child
commands before and after recreation and read the marker through its output.
This does not verify a real provider CLI or Worktree-specific launch. The
disposable servers and volumes were removed. The public-doc validator and its
tests passed on the preceding documentation commit. Follow-up commit
`c7a7cc69d61774764f227b59eaad3a3a1e5f3edd` pins PostgreSQL 15.20. Its
linux/amd64 image build passed and `psql --version` reported 15.20. The public-
doc tests passed 63/63, the validator checked 55 pages, specification lint
passed, and the PR-doc tests passed 101/101. The 15.20 image has not been
checked through a Local or Worktree agent session or after container
recreation. The prior synthetic Local agent results used the 15.19 image.

The PR remains open while its required documentation-coverage and Claude review
checks are resolved. A new package commit needs current-head Review and
distinct QA before a readiness claim. No PR merge or deployment is part of
this work order.
