---
created: 2026-10-28
status: implemented
requirements:
  - REQ-TASKS-RELATED-DISCOVERY-001
system_design:
  - ../../specs/tasks/system-design/related-task-discovery.md
legacy_specs: []
---

# Fix plan: Related task access contract

## Overview

Correct the advertised access contract for [issue #4921](https://github.com/kdlbs/kandev/issues/4921).
The implementation deliberately rejects unrelated targets, including targets in the same workspace.
The tool description and public coordination guide omit that relation restriction.

Use one work order for the registered metadata, focused regression tests, and public guidance.
Leave the authorization predicate unchanged.
The package is ready for a later explicit implementation request.

## Investigation and confidence

Investigation used repository commit `a33ff4139ada169271a8177fa181c45b0c0176e7` on 2026-10-28.
The issue and its proposal comment were read in full. Neither contains image attachments.
The authenticated GitHub account is `carlosflorencio`. Issue assignment to that account is verified.

**Bug class:** Backend API discovery-contract mismatch.

**Confirmed root cause:** `registerRelatedTasksTool` advertises inspection of another same-workspace task without stating the relation gate.
`listRelatedTasksHandler` supplies the bound caller to the backend.
`ListRelatedForCaller` applies `canReadDocuments` before `ListRelated` builds any response.
The `verbose` filter runs only after that backend request returns.
Thus `verbose: false` cannot admit an unrelated target.

**Smallest service reproduction:** Create independent root tasks A and B in one synthetic workspace.
`ListRelatedForCaller(ctx, A, B)` returns `ErrAccessDenied`.
The issue reports this result through the task-bound tool with `verbose: false`.
No protected live task was queried during this investigation.

**Executed evidence:**

- `TestListRelatedForCaller_GatesUnrelatedAndCrossWorkspace` passed.
- `TestListRelatedForCaller_ReturnsSiblingDescription` passed.
- Temporary `TestRepro4921IndependentRootTargetDenied` passed for the issue's exact independent-root setup.
  Ten existing `TestCanReadDocuments_*` tests also passed. The temporary test was removed.
- Temporary registered-tool test `TestRepro4921RelatedTaskContractStatesRelationGate` failed as expected.
  The description omitted `ancestor`, `descendant`, `non-empty parent`, and `unrelated`.
  The temporary test was removed after the command completed.

The existing service tests prove intentional denial, rather than a missing check introduced by this issue.
The temporary test proves that the discoverable contract lacks the restriction.
Confidence is high for a contract correction.
There is no equivalent confidence or settled authorization design for a workspace-wide metadata grant.

## Requirement conformance

No focused acceptance criterion currently owns the relation-tool access wording.
This is `/fix` classification 2: add the smallest missing requirement while preserving existing eligibility.
The [requirement](../../specs/tasks/requirements/related-task-discovery.md) records that contract.
The [design](../../specs/tasks/system-design/related-task-discovery.md) covers the adapter, service gate, and public wording.

The assumption check separates the following facts and decisions:

- **Confirmed:** The user requested investigation, issue assignment for a reliable fix, and a fix package.
- **Verified:** Source and existing tests intentionally deny unrelated targets.
- **Repair scope:** Correct the misleading promise and protect the current boundary with regression coverage.
- **Unresolved feature:** The comment proposes explicit delegated metadata access.
  Grant authority, revocation, target scope, pagination, and completeness need a separate design.
  This repair neither settles nor implements those choices.

## Scope

### In scope

- Registered tool description and argument help.
- Tests for the advertised restriction, identity binding, verbose behavior, and independent-root denial.
- Public coordination, communication, and MCP guidance.
- Preservation of all existing authorized relation data and forbidden responses.

### Out of scope

- A new relation-summary tool, permission grant, control surface, runtime flag, or database schema.
- Changes to `canReadDocuments`, trusted internal callers, or write and document access rules.
- Changes to projection completeness, pagination, enrichment, conversation access, or task relationships.
- Implementation, permanent test edits, commit, push, or PR creation during this design turn.

This package repairs the inaccurate contract. It does not unblock independent-task coordination.
Do not describe that use case as resolved or silently close it through this package.

## Technical approach

Change only the registration text in `apps/backend/internal/mcp/server/handoff_handlers.go`.
Keep the description within the existing 500-character gate in `tool_metadata_test.go`.
State permitted relations in the main description and the restriction in `task_id` help.
State that `verbose` only changes descriptions.

Add registered-tool tests to `apps/backend/internal/mcp/server/handoff_handlers_test.go`.
Use `testBackend.lastPayload` to inspect bound identity and `testBackend.err` for forbidden propagation.
Use the existing `callTool` helper so argument validation remains part of the exercised path.
Add the exact independent-root case to `apps/backend/internal/task/service/handoff_related_test.go`.

Update `docs/public/coordination.md`, `docs/public/agent-communication.md`, and `docs/public/automation-and-mcp.md` in the same implementation change.
These pages serve users who coordinate agents. Preserve their existing how-to and reference structure.
The README and screenshot catalog contain no conflicting access promise and need no change.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| `.001.1`, `.001.5` | New `TestListRelatedTasks_ToolDescriptionStatesAccessBoundary`; existing `TestCoreToolDescriptionsStayFocused`; public-doc source review and validators |
| `.001.2` | New `TestListRelatedForCaller_RootTasksAreUnrelated`; existing `TestListRelatedForCaller_GatesUnrelatedAndCrossWorkspace` and `TestCanReadDocuments_*` |
| `.001.3` | New `TestListRelatedTasks_VerboseDoesNotBypassForbidden`; existing omit-description and verbose-description tests |
| `.001.4` | New `TestListRelatedTasks_CallerBinding` and `TestListRelatedTasks_RejectsCallerOverride` |
| `.001.5` | Existing `TestListRelatedTasks_ToolSchemaExposesVerbose`, projection tests, and task/Office registration controls |

All abbreviated IDs use the prefix `AC-TASKS-RELATED-DISCOVERY`.
The new description test must fail before the registration text changes.

## Agent-facing verification

The registered-tool tests exercise the agent-visible metadata and call entry point.
The service tests exercise the real eligibility predicate with synthetic tasks.
Together they cover the discovery, request, and rejection boundaries without a browser or a live workspace.
No rendered UI change requires a Playwright test or UI preview.

## Work orders

- [x] [Task 01: Align relation discovery with existing access rules](task-01-align-discovery-contract.md)

Task 01 has no dependencies. It was executed in the primary session after the
explicit implementation request, and its Results section records RED, GREEN,
lint, public-doc, specification, coverage, and diff-check evidence.

## Verification results

Investigation command, from `apps/backend`:

```bash
go test -trimpath -tags fts5 -p 1 ./internal/task/service -run '^(TestListRelatedForCaller_.*|TestReadAccess_.*)$' -count=1 -v
```

Result: PASS, two matching `TestListRelatedForCaller_*` tests. No `TestReadAccess_*` test matched that alternative.
The work order uses the actual `TestCanReadDocuments_*` names for broader policy controls.
The temporary contract reproduction returned FAIL for the expected missing restriction text.
The exact-root diagnostic and access controls ran from `apps/backend`:

```bash
go test -trimpath -tags fts5 -p 1 ./internal/task/service -run '^(TestRepro4921IndependentRootTargetDenied|TestCanReadDocuments_.*)$' -count=1 -v
```

Result: PASS, eleven top-level tests, including the temporary root-task reproduction.
Both temporary Go files are removed. No diagnostic instance or browser was started.

Package checks passed:

- `python3 scripts/list-docs.py validate`: 415 decisions and 1,826 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- Package coverage preflight using `validateCoverage`: `covered`, no reference errors.

Final whitespace and workspace checks cover all four package files.

Implementation checks also passed after the explicit implementation request.
The work order records the RED test, the GREEN registration change, and these
final results:

- Focused MCP server tests: PASS, including the new description contract test.
- Focused task-service tests: PASS, including the independent-root denial.
- Full `./internal/mcp/server` and `./internal/task/service` packages: PASS.
- `go build -trimpath ./...` from `apps/backend`: PASS.
- `bash scripts/lint-go-changed` on the three Go files: `0 issues.`
- Public-doc validators: 63 unit tests and 54 pages.
- `git diff --check`: clean.

## Risks

- Compact output still constructs rich relation records. A guard bypass for verbose mode expands access and is outside this repair.
- The issue also requests a delegated metadata reader. Contract accuracy alone does not satisfy that separate use case.
- Agents can retain old tool descriptions until their client refreshes the session catalog.
- Existing graph lookup and completeness behavior remains outside this package. Do not promise a complete blocker inventory.
