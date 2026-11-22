---
id: "01-document-and-validate"
title: "Document and validate a pinned Docker server extension"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-DOCKER-SERVER-EXTENSIONS-001
acceptance_criteria:
  - AC-EXECUTORS-DOCKER-SERVER-EXTENSIONS-001.1
  - AC-EXECUTORS-DOCKER-SERVER-EXTENSIONS-001.2
  - AC-EXECUTORS-DOCKER-SERVER-EXTENSIONS-001.3
  - AC-EXECUTORS-DOCKER-SERVER-EXTENSIONS-001.4
system_design:
  - ../../specs/executors/system-design/docker-server-extensions.md
---

# Work Order 01: Document and Validate a Pinned Docker Server Extension

## Summary

Give Docker-hosted Local and Worktree users one buildable optional-client
example and one accurate validation path, using Kandev's published image and
inherited entrypoint.

## Owned delivery paths

- `docs/images.md`: remove stale plain-source-build and unconditional-user
  advice; retain persistence and PATH guidance and link to the public guide.
- `docs/public/docker.md`: explain the derived server image, package and
  architecture limits, and disposable agent-command validation.
- `examples/docker-server-extension/Dockerfile`: pin the published image digest
  and PostgreSQL client package version without changing startup behavior.

## Boundaries

Use only task-owned disposable runtime and storage. Do not alter Host state,
agent authentication, sandbox policy, executor profiles, or production volumes.
The server image is separate from a Local Docker task image. No real provider
or Worktree-specific result may be inferred from a synthetic Local fixture.

## Acceptance and verification

The requirement's four acceptance criteria cover the owned paths. Run the
public-doc test and validator, the repository specification linter, and the
PR documentation-coverage validator. Build the pinned example on supported
task-scoped Docker access and check non-root tool visibility, recreation/cache
continuity, and a visible invalid-package failure. For actual agent-process
coverage, inspect the agent's command output; qualify unavailable provider or
executor variants. Record exact head, image/build evidence, cleanup, and any
unrun checks before delivery.

Run the repository checks from its root:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/lint-spec-files.py --all
node --test .github/scripts/pr-docs.test.cjs
```

Build the example on a supported task-scoped Docker daemon:

```bash
docker build --no-cache -f examples/docker-server-extension/Dockerfile \
  -t kandev-postgresql-client:0.98.0 examples/docker-server-extension
```

Use the commands in the [public Docker guide](../../public/docker.md#check-an-extension-in-a-disposable-deployment)
for agent-command, recreation, and cleanup checks. The required PR
documentation-coverage status must be read back from the exact PR head after
publication; the local validator unit tests alone do not establish that status.

## Results to date

- The preceding documentation candidate passed 63 public-doc tests and the
  54-page validator; its 15.19 package pin built on linux/amd64. An invalid
  package version failed with apt exit 100.
- Follow-up commit `c7a7cc69d61774764f227b59eaad3a3a1e5f3edd` pins package
  version 15.20. Its linux/amd64 image build passed and `psql --version`
  reported 15.20. Public-doc tests passed 63/63, the validator checked 55
  pages, specification lint passed, and PR-doc tests passed 101/101.
- Independent QA observed a synthetic Local ACP agent's actual child command
  output on the preceding 15.19 image, before and after same-volume server
  recreation: UID 1000, persistent home and cache ownership, `/usr/bin/psql`,
  and the retained marker. The synthetic fixture did not authenticate a real
  provider or exercise Worktree. The updated 15.20 image has not received this
  agent-session or container-recreation check, nor an arm64 build.
- The earlier documentation-coverage correction left the example and guide
  unchanged. The follow-up refreshes the package pin and upgrade guidance.
- The source Dockerfile and user guide are unchanged by this documentation
  coverage correction. Fresh exact-head checks and independent Review/QA are
  pending for the added work-order package.

## Related documents

- [Plan](plan.md)
- [Requirements](../../specs/executors/requirements/docker-server-extensions.md)
- [System design](../../specs/executors/system-design/docker-server-extensions.md)
