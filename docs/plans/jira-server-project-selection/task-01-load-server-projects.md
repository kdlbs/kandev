---
id: "01-load-server-projects"
title: "Load Server projects"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-JIRA-PROJECT-SELECTION-001
acceptance_criteria:
  - AC-INTEGRATIONS-JIRA-PROJECT-SELECTION-001.1
  - AC-INTEGRATIONS-JIRA-PROJECT-SELECTION-001.2
  - AC-INTEGRATIONS-JIRA-PROJECT-SELECTION-001.3
  - AC-INTEGRATIONS-JIRA-PROJECT-SELECTION-001.4
  - AC-INTEGRATIONS-JIRA-PROJECT-SELECTION-001.5
system_design:
  - ../../specs/integrations/system-design/jira-project-selection.md
---

# Task 01: Load Server Projects

## Summary

Correct Server project listing at `CloudClient.ListProjects` after independently
authored HTTP tests establish RED and passing controls. Preserve the Cloud
operation and all shared transport behavior.

## In scope

- `apps/backend/internal/jira/cloud_client.go`: only `ListProjects` endpoint,
  decoding, and accurate comments.
- New `apps/backend/internal/jira/cloud_client_projects_test.go`: test names
  and AC mapping are in the [plan](plan.md#tests).
- Update results/statuses in this four-file design package after implementation.

## Out of scope

No auth/OAuth, retry/fallback/probing, pagination/truncation, search/status,
transition, saved-view, frontend, typecheck, E2E, UI copy, or persistence changes.
Do not read/copy/replay ROOT's private qualification source or runner. Preserve
all managed resources and others' changes. No delegates or new task/session.

## Acceptance

1. Independent Server HTTP contract test fails on the existing search path
   before production edits; Cloud and direct documented Server auth/schema
   controls pass on those same pre-fix inputs.
2. Server listing then uses its documented array endpoint, while Cloud/default
   mode, ID/key/name projection, auth, context paths, empty success, and
   meaningful upstream errors pass the bounded contract cases.
3. Final owning-package race tests, canonical changed-Go lint, light docs/diff
   checks, and normal hooks pass with actual exits/joins recorded. The shared
   UI consumers remain unchanged.

## Dependencies and execution barrier

None between work orders. Start only after ROOT reviews the package and sends a
later explicit implementation interrupt to this same primary. Await ROOT's
heavy lease before dependency installation, product checks, lint, or hooks.
No installation or heavy command is authorized by creating this file.

## Verification

Run from the repository root after release. Use the installed Node 24.21.0
binary directory on task-local PATH without changing mise settings
and Go 1.26.0 at the exact path below. First, once for this fresh worktree:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
(cd apps && pnpm install --frozen-lockfile)
```

Use `/tdd`: author new tests, then run the causal RED and control selection
before editing production. RED must report the Server contract failure and
both controls passing; any toolchain/setup failure is not RED.

```bash
export PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:$PATH"
export GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB
export GOFLAGS=-p=1
export KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/jira-project-selection-red.XXXXXX)"
scripts/run-quiet backend-test -- timeout --kill-after=10s 6m bash -c '
  cd apps/backend && exec /home/jcfs/.local/share/mise/installs/go/1.26.0/bin/go test -race -trimpath -tags fts5 -p 1 -count=1 -timeout 5m -v -run "^TestCloudClient_ListProjects_(ServerContract|ServerTransportControl|CloudContract)$" ./internal/jira
'
```

After the minimal correction, run the complete owning package:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:$PATH"
export GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB
export GOFLAGS=-p=1
export KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/jira-project-selection-green.XXXXXX)"
scripts/run-quiet backend-test -- timeout --kill-after=10s 6m bash -c '
  cd apps/backend && exec /home/jcfs/.local/share/mise/installs/go/1.26.0/bin/go test -race -trimpath -tags fts5 -p 1 -count=1 -timeout 5m ./internal/jira
'
```

Then scoped canonical lint from the repository root, using only actual changed
Go paths (the two paths below are the intended final change):

```bash
export KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/jira-project-selection-lint.XXXXXX)"
scripts/run-quiet backend-lint -- timeout --kill-after=10s 6m bash scripts/lint-go-changed apps/backend/internal/jira/cloud_client.go apps/backend/internal/jira/cloud_client_projects_test.go
```

The lint command inherits the bounded Go environment above. Diagnose actual
owned errors and rerun checks on corrected inputs within the lease; do not
blindly retry unchanged failures or expand resource limits without ROOT.
Record wrapper and actual tool exits, native handles and joins, timeout if any,
and known-current owned process absence before returning the heavy lease.
No broad backend, unsharded repository/sqlite, frontend, or browser gate.

Light checks, from repository root:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/jira-server-project-selection docs/specs/integrations
```

Use `.github/scripts/pr-docs.cjs`'s exported `validateCoverage` with the actual
changed paths and complete four-file contents for the local documentation
coverage preflight. Also check intended runtime paths against this work order
before implementation; label that result prospective coverage, not a runtime
diff. Follow normal hooks later, including gofmt and restaging hook changes.

## Inputs and likely files

- [Requirements](../../specs/integrations/requirements/jira-project-selection.md)
  and [system design](../../specs/integrations/system-design/jira-project-selection.md).
- `apps/backend/AGENTS.md`, `apps/backend/internal/integrations/AGENTS.md`.
- `cloud_client.go`, `client.go`, `models.go`, existing `cloud_client_test.go`;
  inspect handler/factory/consumer wiring without expanding production scope.
- `.github/workflows/backend-tests.yml` owns broader test/cohort policy;
  `scripts/lint-go-changed` owns local package/base lint selection.

## Risks

The private qualification proof is not reusable test source. Independently
write fixtures from the primary contracts. Maintain the existing Cloud page
and body-size bounds, and do not claim live Jira proof.

## Parallelism

`sequential`

## Results

ROOT reviewed the complete package and explicitly released implementation and
the sole local-heavy lease on 2026-10-18. Completed in the same primary session with no delegates or additional sessions.
The independently authored Server RED and passing controls preceded the
production correction. Complete Jira package race GREEN and canonical scoped
lint passed; actual exits/joins and logs are in the manifest. Normal delivery
hooks remain mandatory.
Design validation results are recorded in [the manifest](plan.md#verification-results).
