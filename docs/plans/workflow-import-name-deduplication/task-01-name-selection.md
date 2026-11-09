---
id: "01-name-selection"
title: "Apply shared workflow import name selection"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-IMPORT-PROFILES-001
acceptance_criteria:
  - AC-TASKS-IMPORT-PROFILES-001.2
  - AC-TASKS-IMPORT-PROFILES-001.4
  - AC-TASKS-IMPORT-PROFILES-001.5
  - AC-TASKS-IMPORT-PROFILES-001.7
  - AC-TASKS-IMPORT-PROFILES-001.10
  - AC-TASKS-IMPORT-PROFILES-001.11
system_design:
  - ../../specs/tasks/system-design/workflow-import-profile-selection.md
---

# Task 01: Apply shared workflow import name selection

## Summary

Apply the [shared workflow-name selection design](../../specs/tasks/system-design/workflow-import-profile-selection.md#shared-workflow-name-selection) across preview and both import entry points.
Independently author failing regressions against the public boundaries, implement the smallest shared correction, and clarify the public import reference.

## In scope

- Request-local exact-name selection, preserving original indexes and ordered occurrence-level skipped results.
- Backend preview, browser binding submission, raw-YAML HTTP and MCP import.
- All tests in the [manifest's AC matrix](plan.md#tests), real-domain persistence evidence, and no workflow/step writes for skips.
- Public reference clarification and accurate results/status for this work order and its manifest.

## Out of scope

The [plan exclusions](plan.md#out-of-scope) apply. Keep the patch small and preserve foreign edits.
Do not alter sync/conversion helpers, migrations, uniqueness constraints, existing workflow definitions, frontend controls/state/copy, or unrelated completed packages.

## Acceptance

1. AC-TASKS-IMPORT-PROFILES-001.7 and .10 pass across preview, both public service methods, raw-YAML HTTP, browser JSON, and MCP. Mixed existing/hidden/new/repeated/exactly distinct names retain original indexes and first-occurrence content; skipped occurrences cause no workflow/step writes.
2. AC-TASKS-IMPORT-PROFILES-001.11 and compatibility coverage for .2, .4, and .5 pass: skipped missing-profile entries do not block import, retained missing bindings still do, structural errors in skipped entries or supplied bindings still reject before writes, and selected profile IDs/session targets survive reload.
3. TDD shows the causal duplicate regressions failing before production edits and passing afterward with real UUID-producing domain persistence; all listed checks pass and the public reference agrees with the final behavior. Keep the broader owning requirement/design draft rather than promoting them based on this bounded repair.

## Implementation sequence

1. Read the plan, paired contracts, scoped backend guide, `/tdd`, and `/docs-maintainer`. Mark this work order `in_progress` only when implementation starts.
2. Write `import_name_selection_test.go` and a small real task-service/repository fixture in `import_name_selection_fixture_test.go`. Add HTTP and MCP boundary tests in the files below. Use the exact test names from the manifest.
3. Run the focused regression command. Require semantic failure for both duplicate-import paths and browser preview/profile selection; compiler/setup failures are not Red evidence. Assert real workflow and step rows, first content, foreign keys, and UUIDs.
4. Add the shared selector and wire raw import, preview, resolution, and preparation. Preserve full-document/binding validation and original-index identity. Keep `importSingleWorkflow` and sync untouched.
5. Run Green and the existing affected package tests, then canonical changed-Go lint. Update only the public reference's name-selection wording. Run documentation/specification/diff checks and record actual outcomes.
6. Mark only this work order `done` and this manifest `implemented` once its checks pass. Preserve unrelated historical records and the owning pair's draft statuses.

## Verification

Run from the repository root in Bash with the configured Node/Go toolchain. Commands are underlying checks; execution resource controls and log placement are session-owned.
The first command is the focused Red/Green command for newly authored tests. The next commands exercise existing affected behavior, including sync and profile selection.

```bash
(cd apps/backend && timeout --kill-after=10s 15m go test -trimpath -race -tags fts5 -p 1 -timeout 120s -count=1 -run 'TestWorkflowImport(DocumentDuplicates|MixedNameSelection|PreviewSelection|BindingSelection|SelectionValidation|SelectionRechecksDestination|NameSelectionHTTP)$|TestHandleImportWorkflow_DocumentNameSelection$' ./internal/workflow/service ./internal/workflow/handlers ./internal/mcp/handlers)
(cd apps/backend && timeout --kill-after=10s 15m go test -trimpath -race -tags fts5 -p 1 -timeout 120s -count=1 ./internal/workflow/models ./internal/workflow/service ./internal/workflow/controller ./internal/workflow/handlers)
(cd apps/backend && timeout --kill-after=10s 15m go test -trimpath -race -tags fts5 -p 1 -timeout 120s -count=1 -run 'TestHandleImportWorkflow|TestHandleExportWorkflow' ./internal/mcp/handlers)
timeout --kill-after=10s 6m bash scripts/lint-go-changed apps/backend/internal/workflow/service/service.go apps/backend/internal/workflow/service/import_profile_selection.go apps/backend/internal/workflow/service/import_name_selection.go apps/backend/internal/workflow/service/import_name_selection_test.go apps/backend/internal/workflow/service/import_name_selection_fixture_test.go apps/backend/internal/workflow/handlers/import_name_selection_test.go apps/backend/internal/mcp/handlers/config_workflow_import_name_selection_test.go
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/workflow-import-name-deduplication
```

If the actual changed Go file list differs, update the lint command to those actual files before running it; never broaden to full-backend lint.
Confirm every changed test suite is covered and save the actual command/exit/result, including Red's expected semantic assertions.
Use `.github/scripts/pr-docs.cjs`'s exported `validateCoverage` with local document contents, this changed work order and the actual runtime changed paths to check the REQ -> AC -> design -> plan chain before completion; a documentation-only exempt result does not exercise that chain.
The plan's test matrix is the requirements-to-work-order coverage list.

## Files likely touched

- `apps/backend/internal/workflow/service/service.go`: raw import selection only.
- `apps/backend/internal/workflow/service/import_profile_selection.go`: preview, binding resolution and preparation selection.
- New `apps/backend/internal/workflow/service/import_name_selection.go`.
- New `apps/backend/internal/workflow/service/import_name_selection_test.go` and `import_name_selection_fixture_test.go`.
- New `apps/backend/internal/workflow/handlers/import_name_selection_test.go`.
- New `apps/backend/internal/mcp/handlers/config_workflow_import_name_selection_test.go`.
- `docs/public/workflow-import-export.md`: name field and reconciliation wording only.
- This manifest/work order and the paired requirements/design if implementation reveals a necessary reconciliation.

No production handler/controller, shared schema, dependency, generated file, or frontend change is expected.

## Dependencies

None. The existing profile-selection implementation is present at the baseline.

## Risks

Deterministic name-derived IDs in existing fixtures can mask the defect. The service fixture must create via the real task domain; boundary fixtures must not collide on duplicate names.
Filtering or shared-map mutation can corrupt original binding indexes or suppress selected entries.
Whole-envelope validation and sync must keep their existing boundaries.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/workflow-import-profile-selection.md): .7, .10, .11; .2, .4, .5 compatibility.
- [System design](../../specs/tasks/system-design/workflow-import-profile-selection.md): shared selection, validation and persistence.
- [Prior plan](../workflow-import-profile-selection/plan.md) and [prior work order](../workflow-import-profile-selection/task-01-import-profile-selection.md): historical scope/results only.
- Existing `service/service_test.go`, `service/import_profile_selection_test.go`, `handlers/http_handlers_test.go`, `handlers/step_events_test.go`, and `mcp/handlers/config_workflow_handlers_test.go` for surrounding APIs/test patterns.
- `internal/backendapp/services.go`'s `workflowProviderAdapter` for a test-local forwarding adapter to real task-service calls; do not edit the production adapter.
- `apps/backend/AGENTS.md`, `/tdd`, and `/docs-maintainer`.

## Results

Completed on 2026-10-26 with the seven Go files listed above and the public-reference update.
The real-domain service fixture asserts SQLite foreign keys are enabled, validates generated workflow UUIDs, and records insert/update/delete effects through test-local SQL audit triggers.
HTTP and MCP tests use per-creation UUID fixtures to exercise their distinct wire paths without name-derived ID collisions.

Verification commands above were run with these results:

- Focused Red/Green command: Red exit 1 on the expected duplicate-creation/profile-selection failures before production edits; Green exit 0 across all three packages afterward (service 9.634s, HTTP handlers 1.288s, MCP handlers 1.285s).
- Full workflow models/service/controller/handlers command: exit 0, respectively 1.036s, 13.122s, 1.255s, 5.162s. Existing sync, profile selection, references, and HTTP contracts passed.
- Focused MCP import/export command: exit 0, 1.592s.
- Canonical changed-Go lint command: exit 0, zero issues, all seven changed Go files included.
- `node --test scripts/validate-public-docs.test.mjs`: exit 0, 63 passed.
- `node scripts/validate-public-docs.mjs`: exit 0, 54 published pages validated.
- `python3 scripts/list-docs.py validate`: exit 0, 403 decisions and 1740 specifications.
- `python3 scripts/lint-spec-files.test.py`: exit 0, 36 passed.
- `python3 scripts/lint-spec-files.py --all`: exit 0.
- `git diff --check`: exit 0; scoped status confirmed the manifest and work order were present.
- Exported `validateCoverage` preflight using actual runtime changed paths and local documents: `covered`, zero errors, one accepted work order.

The documented Go arguments ran under Go 1.26.0 with local toolchain selection, GOMAXPROCS=2 and a 512MiB memory target; changed-Go lint ran separately with a 3GiB target.
No production/permanent tests were changed during design. Tests were authored only after the separate reviewed implementation release.
No frontend or rendered UI change was required, so no browser capture or new Playwright run is claimed. The broader owning requirement/design remain draft.
Hosted CI, review and publication evidence are maintained separately from this completed local work order.
