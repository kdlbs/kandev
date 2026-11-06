---
id: "01-open-hosted-workspace"
title: "Open hosted editors at the selected workspace"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-HOSTED-EDITOR-001
acceptance_criteria:
  - AC-WORKSPACES-HOSTED-EDITOR-001.1
  - AC-WORKSPACES-HOSTED-EDITOR-001.2
  - AC-WORKSPACES-HOSTED-EDITOR-001.3
  - AC-WORKSPACES-HOSTED-EDITOR-001.4
  - AC-WORKSPACES-HOSTED-EDITOR-001.5
system_design:
  - ../../specs/workspaces/system-design/hosted-editor-workspace.md
---

# Task 01: Open Hosted Workspace

## Summary

Independently prove the real service emits a file target for a selected root,
then correct custom hosted URL construction to emit the selected folder.
Preserve existing file navigation values, root selection, and error admission.

## In scope

- Follow the [plan's regression/control matrix](plan.md#tests), using a dedicated
  `hosted_workspace_test.go` and real `Service.OpenEditor`.
- Carry the resolved root into hosted construction and classify root equality
  after existing lexical admission. Update direct helper call sites as needed.
- Record RED, GREEN, package/lint results and synchronize work-order/plan status.

## Out of scope

All plan exclusions apply. No production or permanent test edits before the
later implementation request. Do not copy ROOT's private qualification test.
No shared path framework, nested folder inference, URL vendor-policy redesign,
frontend/API fields, browser suite, broad backend suite, or task SQLite scan.

## Acceptance

1. The new real-service root regression fails before the production correction
   for the expected decoded `folder`/`file` difference; hosted file coordinates
   and embedded root controls pass independently in that RED run.
2. After correction, the entire matrix passes with exact selected-root identity,
   ordinary URL value preservation, and existing failure/containment categories.
3. Owning-package race tests, canonical changed-Go lint, documentation catalog,
   specification and coverage gates pass. No native/external IDE was launched;
   recorded evidence states the actual service-to-URL boundary.

## Verification

Run from repository root after explicit implementation authorization. Execute
sequentially. Use Go 1.26 and the task-local Node 24.21.0 PATH; do not change
system toolchain trust/configuration. An environment/setup failure is not RED.

```bash
# RED: run after independently writing tests, before production changes.
(cd apps/backend && env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m /home/jcfs/.local/share/mise/installs/go/1.26.0/bin/go test -race -trimpath -tags fts5 -p 1 -count=1 -timeout 5m -v -run '^TestOpenEditor_(HostedWorkspaceRoot|HostedCompatibility|HostedWorkspaceEmbeddedControl|InternalVscodeWithoutWorkspace)$' ./internal/editors/service)

# GREEN: after the minimum correction; includes existing compatibility controls.
(cd apps/backend && env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m /home/jcfs/.local/share/mise/installs/go/1.26.0/bin/go test -race -trimpath -tags fts5 -p 1 -count=1 -timeout 5m ./internal/editors/service)

# Include any additional actual changed Go paths if implementation requires them.
env PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:$PATH" GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB GOFLAGS='-trimpath -tags=fts5 -p=1' timeout --kill-after=10s 6m bash scripts/lint-go-changed apps/backend/internal/editors/service/service.go apps/backend/internal/editors/service/service_test.go apps/backend/internal/editors/service/hosted_workspace_test.go

python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/hosted-editor-workspace apps/backend/internal/editors/service
git status --short -- docs/plans/hosted-editor-workspace
```

For the lightweight documentation-coverage preflight, run this from repository
root (the listed inputs are this work order's intended owned change set):

```bash
/home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node <<'JS'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const docs = [
  'docs/plans/hosted-editor-workspace/plan.md',
  'docs/plans/hosted-editor-workspace/task-01-open-hosted-workspace.md',
  'docs/specs/workspaces/requirements/hosted-editor-workspace.md',
  'docs/specs/workspaces/system-design/hosted-editor-workspace.md',
];
const source = [
  'apps/backend/internal/editors/service/service.go',
  'apps/backend/internal/editors/service/service_test.go',
  'apps/backend/internal/editors/service/hosted_workspace_test.go',
];
const result = validateCoverage({
  changedFiles: [...docs, ...source].map(filename => ({filename, status: 'modified', additions: 1, changes: 1})),
  fileContents: Object.fromEntries(docs.map(path => [path, fs.readFileSync(path, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
JS
```

At design time this preflight validates the intended package's references; it
is not evidence that production files changed or tests ran. At implementation
completion reconcile its input list with actual changed paths. Broad backend
and frontend cohorts remain CI-owned. Normal repository hooks apply at later
publication; hooks do not replace the checks above.

## Files likely touched

- `apps/backend/internal/editors/service/service.go`
- `apps/backend/internal/editors/service/hosted_workspace_test.go` (new)
- `apps/backend/internal/editors/service/service_test.go` (internal helper calls only)
- This work order and `plan.md` for execution status and results.

Keep `editor_file_containment_test.go`, selection/resolution helpers, frontend,
API schemas, public docs, dependencies, and runtime profiles unchanged.

## Dependencies

None. The existing service, minimal fakes, and containment tests are available.
This is the only pending work order.

## Risks

See [plan risks](plan.md#risks). Test fixture overrides must preserve ordinary
fake behavior for existing tests. Coordinate/path encoding assertions must be
independent of the implementation. Hosted file controls establish compatibility
only; no code-server file/rendering claim.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/workspaces/requirements/hosted-editor-workspace.md)
- [Design](../../specs/workspaces/system-design/hosted-editor-workspace.md)
- Existing `service.go`, `service_test.go`, `editor_file_containment_test.go`.
- [Containment contract](../../specs/workspaces/system-design/editor-file-containment.md)
  and [embedded capability ADR](../../decisions/2026-07-30-embedded-editor-executor-capabilities.md).

## Results

Implemented after ROOT's later explicit release on 2026-10-18.

- RED: exact command above exited 1 for the expected six decoded root target
  mismatches. Hosted file coordinate cases and both embedded controls passed.
- GREEN: exact owning-package race command passed on the final production
  revision (1.060s); all new and existing service tests included.
- Changed-Go lint: exact three-path command passed with zero issues after the
  required if/else-to-switch diagnostic correction.
- `list-docs.py validate`, all 36 `lint-spec-files.test.py` tests,
  `lint-spec-files.py --all`, Node documentation coverage (`covered`, no errors),
  and `git diff --check` passed. All listed documentation paths and AC links exist.
- Changed files: hosted builder/dispatch, existing helper call signature, new
  dedicated real-service test file, and the four accepted documentation artifacts.
- Service-to-returned-URL proof only. No real native/hosted editor launch, HTTP
  routing, or browser rendering claim. Shared opener/mobile data exception and
  unchanged public-doc assessment remain applicable.

Normal hooks and hosted CI/review publication receipts are recorded externally;
no second broad local verification pass is required.
