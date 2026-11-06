---
created: 2026-10-18
status: implemented
requirements:
  - REQ-INTEGRATIONS-JIRA-PROJECT-SELECTION-001
system_design:
  - ../../specs/integrations/system-design/jira-project-selection.md
legacy_specs: []
---

# Implementation Plan: Jira Server Project Selection

## Overview

Restore Server project inventory by choosing its documented endpoint and array
response at the existing REST client boundary. One sequential work order adds
independent HTTP regressions, makes the minimal correction, and validates the
owning Jira package. ROOT reviewed the complete design package and explicitly released implementation
in this same primary session on 2026-10-18. Delivery remains subject to the
checks below; merge requires a separate later exact-head grant.

Integration owns the provider inventory contract. Catalog and adjacent-source
discovery found Jira default-view, status-filter, and watcher-prompt contracts,
but no dedicated project inventory contract. The saved-view pair owns personal
preferences, not inventory. The new vertical pair includes the existing UI
outcome without changing those adjacent contracts or the system boundary.

## Scope

In scope: `CloudClient.ListProjects` Server endpoint/array decoding, accurate
method comments, a dedicated test file, and this four-document package.

Excluded: frontend code/typecheck/E2E, saved-view or initial-load changes,
auth/OAuth policy, general pagination/truncation repair, retry/fallback/version
probing, search/status/transitions, new settings, public model changes, and ADRs.
Preserve others' changes and all managed worktrees, branches, dependencies,
caches, processes, and protected resources. No native delegates or new sessions.

## Root cause and evidence

Current `ListProjects` sends `/rest/api/2/project/search?maxResults=200` for
Server and decodes a Cloud `values` object. The official Server 9.12 contract
instead documents `/rest/api/2/project` with a bare array; the current Server
8+ compatibility comment is contradicted by that contract.

ROOT's qualification receipts at
`/tmp/kandev-root-jira-project-contract-69iwq25o/` report one causal Server
failure (404 on the old search path), two passing controls (Cloud envelope and
direct documented Server array with PAT transport), actual Go exit 1, wrapper
exit 0, and package duration 0.092s. Native handle 5209 actually joined as
617ef2. This is an independent official-contract HTTP fixture, not a live Jira
deployment. Only the four named receipt files were read; private source and
runner are excluded from this work and must never be read, copied, or replayed.
Permanent tests will be independently authored after implementation release.

## Technical approach

Branch on `c.instanceType == InstanceTypeServer` inside `ListProjects`, request
`c.apiBase + "/project"` through `do`, and decode the bare `[]JiraProject`.
Keep Cloud's current path, envelope, projection, and constructor default.
Preserve the existing transport and error semantics. No other production
boundary changes are planned.

| Provider / transport | Identity shape | Intended behavior | Evidence / unsupported shape |
| --- | --- | --- | --- |
| Server PAT / REST | Array of ID/key/name projects | Documented v2 list, configured context path | Real client HTTP regression, Bearer and direct schema control; errors propagate, no fallback |
| Cloud API token / REST | Paged `values` entries | Existing v3 search with `maxResults=200` | Explicit Cloud and empty-instance controls; existing auth tests cover other REST auth modes |
| Cloud OAuth / MCP | Existing MCP projection | Unchanged factory routing | Existing owning-package tests; no new OAuth claims or policy |

## Tests

All new cases live in
`apps/backend/internal/jira/cloud_client_projects_test.go`, avoiding the existing
large `cloud_client_test.go` and its 800-effective-line limit.

| AC suffix (same requirement) | Planned test name | Evidence |
| --- | --- | --- |
| `.1`, `.3` | `TestCloudClient_ListProjects_ServerContract` | Exact method/path/query, bare array, all ID/key/name values and order, PAT header |
| `.1`, `.3` | `TestCloudClient_ListProjects_ServerTransportControl` | Direct `do` to independently specified documented path proves auth/array transport before correction |
| `.2` | `TestCloudClient_ListProjects_CloudContract` | Explicit Cloud and empty instance, exact v3 query/envelope, Basic auth |
| `.3` | `TestCloudClient_ListProjects_ContextPath` | Server and Cloud with `/jira` base prefix |
| `.4` | `TestCloudClient_ListProjects_EmptyAndErrors` | Provider-specific empty responses and meaningful non-2xx `APIError`; no success masquerade |
| `.5` | Same real-client contract cases plus inspected unchanged consumer path | Restored inventory feeds existing intersection and shared selector; no new UI logic |

The causal Server test must fail before production changes, while the Cloud and
direct Server transport controls pass. Avoid implementation-mirroring mocks.

## E2E boundary and documentation impact

Real HTTP requests through the production client exercise the changed external
boundary end to end. Existing My Jira intersection and `ProjectPill` remain
unchanged, including their shared desktop/phone behavior. No browser or OS gate
is warranted for this nonvisual protocol correction.

Docs-maintainer assessment: existing public integration instructions, README,
and screenshot catalog contain no changed endpoint/schema promise or user step.
The fix restores an existing selector without changing configuration, labels,
or public API shape. Internal docs only; no public page or screenshot change.
Reassess if implementation exceeds this scope.

## Work orders

- [x] [Task 01: Load Server projects](task-01-load-server-projects.md) (`done`, wave 1, no dependencies)

## Verification strategy

After explicit release and the ROOT heavy lease: install fresh-worktree pnpm
dependencies once from `apps` using installed Node 24.21.0 on task-local PATH.
No mise trust/settings changes. Follow the work order's bounded Go 1.26.0
RED/controls/GREEN commands and scoped canonical lint. CI owns full backend
testing, persistence cohorts, and full lint; no task/repository/sqlite scan.
Every `run-quiet` invocation gets a unique task-owned directory. Retain every
native handle, join to actual completion, and record actual exit codes.

Light design checks: catalog validation, specification linter/tests,
documentation coverage preflight, and whitespace/status checks. Normal commit
hooks and delivery belong to the later authorized implementation phase.

## Verification results

Design checkpoint on 2026-10-18, base HEAD
`bc777d866ffaf5e2b0b864c50255762e57a8d594`:

- `python3 scripts/list-docs.py validate`: exit 0, 386 decisions and 1648
  specifications validated; new requirement/design discoverable in Integration.
- `python3 scripts/lint-spec-files.test.py`: exit 0, 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: exit 0, all specification files passed.
- Exported `.github/scripts/pr-docs.cjs` `validateCoverage`, using complete
  four-file contents: exit 0, actual design-only paths `exempt`; prospective
  two Go paths `covered`, linked work order/design/requirement accepted,
  zero errors. Prospective coverage is not a claim of production changes.
- Initial bare `node` preflight launch exited 127 because that shell lacked
  Node on PATH. Corrected invocation used the installed absolute
  `/home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node` and passed.
- `git diff --check`: exit 0. Status shows only this four-file untracked
  package; tracked production and permanent tests unchanged.

All light commands completed synchronously with actual exit results; no native
background handles or heavy lease acquired. No installs, product tests, hooks,
or heavy work run. Implementation and its RED/GREEN evidence remain pending.

## Risks

- The existing response-body bound and single Cloud page remain limitations;
  do not claim exhaustive inventory beyond those bounds.
- My Jira still restricts options to projects in its existing user-ticket result;
  listing a project alone does not make it eligible there.
- Official-contract fixtures prove protocol behavior, not live deployment
  compatibility across every Jira release or authentication configuration.

### Implementation validation (2026-10-18)

- Fresh frozen pnpm install: exit 0, native session 75393 joined as 07de61.
- Independently authored RED: native session 39004 joined as 9866ba, actual
  Go/wrapper exit 1. Only Server contract failed on the old search route with
  404; direct Server PAT/schema and Cloud explicit/default controls passed.
  Package 0.103s.
- Complete owning Jira package GREEN: native session 92843 joined as 839f8c,
  Go/wrapper exit 0, package 1.387s. Exact work-order race/trimpath/fts5/-p1
  command used Go 1.26.0, GOTOOLCHAIN=local, GOMAXPROCS=2, GOMEMLIMIT=512MiB,
  Go 5m and GNU 6m/kill10 timeouts.
- Canonical changed-Go lint for the two owned Go files: native session 1542
  joined as 047448, actual/wrapper exit 0, no lint findings. Same CPU/memory
  bounds, canonical lint 5m within GNU 6m/kill10.
- Catalog, all specification lint, 36 linter tests, actual runtime-path
  documentation coverage (`covered`, zero errors), and diff checks passed.
- Normal commit hooks are required at delivery. No frontend/E2E/full-backend
  local checks or live Jira deployment were used.

Task-owned logs:
`/tmp/jira-project-selection-install.J3akDL/kandev-run.install.XJticOY7.log`,
`/tmp/jira-project-selection-red.9xXRIF/kandev-run.backend-test.2p58dQe1.log`,
`/tmp/jira-project-selection-green.YaKsHd/kandev-run.backend-test.44yNuO3j.log`,
and `/tmp/jira-project-selection-lint.rGaUUA/kandev-run.backend-lint.3puZm3R1.log`.
All native local check handles actually joined; no cleanup of managed resources.
