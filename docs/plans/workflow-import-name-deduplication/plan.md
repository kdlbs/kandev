---
created: 2026-10-26
status: implemented
requirements:
  - REQ-TASKS-IMPORT-PROFILES-001
system_design:
  - ../../specs/tasks/system-design/workflow-import-profile-selection.md
legacy_specs: []
---

# Implementation plan: Workflow import name deduplication

## Overview

Make one-time workflow import select the first new exact name in document order and skip later occurrences before profile resolution or persistence.
Deliver one backend work order covering preview, browser submission, raw-YAML HTTP, and MCP.
Amend the existing tasks-owned [requirement](../../specs/tasks/requirements/workflow-import-profile-selection.md) and [design](../../specs/tasks/system-design/workflow-import-profile-selection.md), rather than creating a parallel repair specification.

Tasks owns this capability because it owns durable workflow definitions and their import selection; workspaces supplies the destination and agents supplies profile eligibility.
The owning pair remains draft. This repair implements only AC-TASKS-IMPORT-PROFILES-001.7, .10, and .11, with compatibility checks for .2, .4, and .5; it does not certify or promote the broader profile-selection package.

## Evidence and reconciliation

ROOT independently qualified baseline `6a1a2ae10003c6bccc754ab144c6035ac38158d1` using real task SQLite and workflow repositories/service, foreign keys enabled, normal UUID-producing domain creation, and no production mocks.
Two exact `Release A` entries created two durable workflows and lanes through both public service entry points; results were `Created=[Release A, Release A]`, `Skipped=[]`.
Distinct-name and pre-existing-name controls passed for both paths. This is supplied qualification evidence, not a reproduction run performed for this design package.

Source inspection at that baseline confirms `ImportWorkflows` builds `existingNames` once and never records names created within the document.
`collectImportProfileSteps`, `resolveImportProfileBindings`, and `prepareImportedWorkflows` likewise filter only against destination names.
Consequently, merely fixing the persistence loop would leave preview and profile resolution inconsistent.
`WorkflowExport.Validate` deliberately accepts repeated workflow names and validates all entries; rejecting duplicate documents would change the supported contract.

The existing requirement's .7 owns profile-free name skips, but did not define within-document order or the shared noninteractive rule.
The amendment makes that selection precise and adds .10 for cross-entry-point results and .11 for whole-envelope/binding validation preservation.
The public import reference already describes name deduplication and create-or-skip; implementation will clarify duplicate occurrences in the same document.
No new architectural choice requires an ADR. Existing profile-model strictness remains separate from import matching.

## Scope

### In scope

- One selection rule shared across raw import, preview, binding resolution, and prepared persistence.
- Existing visible/hidden names, repeated new names, original binding indexes, and exact case/whitespace distinctions.
- No workflow/step writes for skipped occurrences; first retained content survives unchanged.
- A later duplicate's missing profile cannot block import; malformed skipped entries and malformed supplied bindings still fail existing structural validation.
- Independent TDD regressions and a small clarification to `docs/public/workflow-import-export.md`.

### Out of scope

- Concurrent-request uniqueness, global name constraints, schema/migrations, sync behavior, and batch transactions.
- Existing-definition updates/merges or falling through to a later duplicate after a retained occurrence fails.
- Workflow-default/review-action matching, profile creation, runtime routing, or session lifecycle changes.
- Frontend files, UI controls/copy, parsing/formatting changes, settings-save behavior, or unrelated repairs.

## Technical approach

Add a small request-local selector in `apps/backend/internal/workflow/service/import_name_selection.go`.
Given the original export and existing exact names, return retained original indexes and skipped names in document order; copy the name set so callers' state cannot be consumed across passes.
Wire `ImportWorkflows` in `service.go` to import only those indexes, retaining its existing error timing and appending `Created` only after successful import.

In `import_profile_selection.go`, preview consumes the same selection; browser submission computes it once and shares retained indexes with profile resolution and preparation.
Preserve original indexes throughout, especially when an existing workflow or duplicate appears before a later retained workflow.
Leave whole-document normalization/validation and `expectedImportProfileSteps`/`validateImportProfileBindings` structural checks ahead of selected-occurrence processing.
Only retained items resolve eligible profile IDs, prepare steps, and reach persistence.
Do not move the selection rule into `importSingleWorkflow`, which is also used by sync.

No HTTP/MCP production contract or frontend change is expected.
The browser already consumes original-index keys and server preview steps; backend response selection corrects the affected flow on both viewports.
If later evidence requires frontend changes, revise scope and apply mobile-parity/E2E before implementation of that extension.

## Tests

Every AC in the table has prefix `AC-TASKS-IMPORT-PROFILES-`.
The named tests below were independently authored after the design review and implementation release.

| Criteria | Test file and proposed test | Observable evidence |
| --- | --- | --- |
| 001.7, .10 | `service/import_name_selection_test.go`: `TestWorkflowImportDocumentDuplicates` | Both public import methods, supported v1/v2 envelopes, repeated new name: one real UUID workflow with only the first occurrence's lane/content; ordered created/skipped lists. |
| 001.7, .10 | Same file: `TestWorkflowImportMixedNameSelection` | Existing visible/hidden names, repeated new A/B, a name present only in another workspace, case and whitespace variants. Count all persisted rows and verify pre-existing definitions/steps unchanged. |
| 001.7, .2 | Same file: `TestWorkflowImportPreviewSelection` | Preview returns no writes, skips each duplicate occurrence, resolves only retained explicit profiles, preserves original noncontiguous workflow indexes and step positions; a skipped-only missing profile does not require a catalog. |
| 001.7, .4, .5, .10 | Same file: `TestWorkflowImportBindingSelection` | Bind only retained indexes, retain exact selected profile IDs and earlier-step session targets after reload; later duplicate missing bindings/unavailable profiles do not block; a retained missing binding still blocks even if a later duplicate is fully bound. |
| 001.11, .5 | Same file: `TestWorkflowImportSelectionValidation` | Invalid step/reference in a skipped occurrence rejects all paths before writes. Unknown/duplicate binding keys and descriptor mismatches still reject skipped bindings; structurally valid skipped bindings need no eligible profile resolution. |
| 001.7, .10 | Same file: `TestWorkflowImportSelectionRechecksDestination` | A name created after preview is skipped on submit, with no workflow/step writes for it and stable indexes for other retained items. |
| 001.7, .10, .11 | `handlers/import_name_selection_test.go`: `TestWorkflowImportNameSelectionHTTP` | Separate raw-YAML HTTP, preview, and browser JSON submission requests agree on the selection/results; later duplicate missing-profile path succeeds; malformed skipped occurrence is rejected. |
| 001.10, .11 | `mcp/handlers/config_workflow_import_name_selection_test.go`: `TestHandleImportWorkflow_DocumentNameSelection` | MCP document request reaches the raw service path, reports one created and one skipped occurrence, and persists only first content; structural error remains a validation response with zero writes. |

Core service regressions must use real task SQLite and workflow repositories and task-service creation through a thin forwarding adapter, with `foreign_keys` asserted enabled and UUID IDs asserted valid.
Do not use the existing name-derived-ID providers as causal persistence proof: their second insert can collide on an ID instead of reproducing two real workflows.
HTTP/MCP boundary fixtures may reuse local routing/message helpers, but use per-creation UUIDs and observe provider create calls plus persisted steps so duplicate creation cannot be hidden by fixture identity.
For skipped-entry and validation-failure cases, also observe workflow/step writes with test-local SQL audit triggers or narrow call counters; final row counts alone must not hide create-then-delete effects.
Use a local database per test and close service goroutines/connections with existing cleanup patterns.

## E2E evidence and mobile assessment

The end-to-end boundary for this backend-owned repair is HTTP/MCP request through parsing, controller/service selection, and persistence, paired with real-domain service regressions.
No Playwright additions or UI previews are planned: no rendered layout, navigation, touch interaction, copy, or client state changes are needed.
Desktop and phone consume the same preview/submission endpoints. This does not claim a newly run browser test or revalidate the existing mobile UI.

## Public documentation

Update the existing reference page `docs/public/workflow-import-export.md` in Task 01.
Clarify first-wins same-document skips and original preview indexes in the import-reconciliation section, and align the workflow-name field description.
Keep duplicate workflow names accepted by structural validation; distinguish acceptance from selection rather than claiming the validator requires unique names.
Preserve browser profile-selection versus raw-YAML/MCP matching and partial-storage-failure distinctions.
No navigation, screenshot, root README, or locale change is needed; searches found no duplicate-name promise there requiring an amendment.

## Related delivery records and overlap

The prior [profile-selection plan](../workflow-import-profile-selection/plan.md) and [work order](../workflow-import-profile-selection/task-01-import-profile-selection.md) record completed browser feature work and its historical results.
They are inputs, not tasks to reopen or results to reuse as proof of this defect's correction.
This package is the only new manifest/work order for the bounded repair.

ROOT's focused overlap assessment reports PR 4714 changes `service.go` imports/Service fields but not its import function; preserve those independent prompt-source additions if they land.
PR 4718 owns task/MCP creation boundaries, and PR 4736 owns workflow deletion. Neither owns this document-selection repair.
These are supplied overlap snapshots, not a claim that every open PR was audited. Reconcile actual affected current diffs before implementation/publication when necessary.

## Work orders

- [x] [Task 01: Apply shared workflow import name selection](task-01-name-selection.md)

One sequential work order; no implementation dependencies or delegated work.
Exact test, lint, public-doc, and specification commands are in its [verification section](task-01-name-selection.md#verification).

## Verification results

Design-only validation on 2026-10-26:

- `python3 scripts/list-docs.py validate`: passed, 403 decisions and 1740 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Scoped `git diff --check`: passed; direct checks also passed for links, AC definitions, whitespace, one manifest/one work order, and unchanged draft lifecycle, including the two untracked plan-package files.
- `.github/scripts/pr-docs.cjs` exported `validateCoverage`, supplied the four local documents and anticipated `service.go` runtime path: `covered`, zero errors, one work order accepted. This tests the planned artifact chain, not a current runtime diff or hosted PR status.
- The first Node invocation was unavailable in the login Zsh PATH; the same preflight passed in the configured non-login Bash environment without installation.

The design turn changed only documents; the results above are design evidence.

Implementation validation on 2026-10-26:

- The focused regression command in Task 01 first exited 1 before production edits: both raw/bound service methods created two real UUID workflows for v1/v2, preview requested the skipped duplicate's profile, browser submission blocked on it, and raw HTTP/MCP reported duplicate creation. This is independent Red evidence.
- The identical focused command then passed all three packages after the selector change (service 9.634s, HTTP handlers 1.288s, MCP handlers 1.285s).
- Full affected workflow models/service/controller/handlers race tests passed (1.036s / 13.122s / 1.255s / 5.162s).
- Focused MCP import/export race tests passed (1.592s).
- Canonical `bash scripts/lint-go-changed` over the seven changed Go files passed with zero issues.
- `node --test scripts/validate-public-docs.test.mjs`: 63 passed; `node scripts/validate-public-docs.mjs`: 54 pages validated.
- `python3 scripts/list-docs.py validate`: 403 decisions and 1740 specifications validated; `python3 scripts/lint-spec-files.test.py`: 36 passed; `python3 scripts/lint-spec-files.py --all`: passed.
- Actual-runtime-diff documentation-coverage preflight: `covered`, zero errors, one work order accepted. `git diff --check` passed and both plan-package files were present in status.

All Go test commands used Go 1.26.0, `-trimpath -race -tags fts5 -p 1`, a 120s test timeout, and the documented outer timeout. No frontend, sync, migration, or production handler/controller changes were needed. The public reference now documents shared first-wins selection and original binding indexes. The owning requirement/design remain draft.
Local implementation is complete. Hosted CI/review evidence is recorded separately and is not certified by these local results.

## Risks

- Filtering into a compact workflow slice would silently bind a selection to the wrong occurrence; preserve original indexes.
- Mutating one name map across multiple browser passes would suppress retained workflows; compute selection once and share indexes.
- Moving validation after filtering would weaken the whole-envelope contract; keep skipped-entry structural regressions.
- Fixing only final persistence would leave unnecessary profile prompts/conflicts in preview and submission.
- Imports remain susceptible to separate concurrent requests selecting the same absent name; this package makes no concurrency guarantee.
