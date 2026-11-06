---
created: 2026-10-23
status: implemented
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-008
system_design:
  - ../../specs/tasks/system-design/current-managed-attachment-recovery.md
legacy_specs: []
---

# Implementation Plan: Managed attachment Git compatibility

## Overview

Remove a false managed-repository ownership rejection when adding a GitHub URL
on Git 2.30. One sequential work order adds regression evidence and replaces
the incompatible path queries without weakening ownership validation.

The task system owns successful source/branch attachment and compensation.
The existing [managed source requirement](../../specs/tasks/requirements/managed-source-attachment.md)
already defines the intended outcome. This is conformance work for
`REQ-TASKS-ATTACH-WORKSPACE-SOURCES-008`, not a new product requirement or
ownership decision. The existing
[recovery ADR](../../decisions/2026-10-21-current-managed-attachment-recovery.md)
remains applicable.

## Confirmed defect and evidence

The report came from other users on other machines. Their URLs, task IDs,
Git versions and filesystem layouts are unavailable. Local installation logs
cannot establish the cause of those incidents. This package fixes an independently
reproduced defect with the identical error and URL-versus-local behavior; it
does not claim Git 2.30 explains every report.

`Cloner.verifyAttachmentCheckout` calls `attachmentGitPath` with
`--show-toplevel`, `--absolute-git-dir` and `--git-common-dir`. That helper
unconditionally prepends `--path-format=absolute`. Git 2.30 does not recognize
the option, echoes it to stdout, prints the requested path, and returns zero.
The validator compares both output lines against one expected path and returns
`ErrRepositoryOwnershipMismatch`. This can happen after a successful clone,
before verified path persistence or worktree creation.

A temporary `TestReproGitHubURLAttachmentGitCompatibility` used the registered
`mcp.add_branch_to_task` handler, real task service, SQLite, executor and cloner,
with the existing persisting test worktree manager. A Git command wrapper
injected a command-local URL rewrite so the public-shaped
`https://github.com/acme/added` input cloned disposable local Git content.
Fixture credentials replaced remote credential resolution; no GitHub access
or user data was needed. An actual Git 2.30.0 binary was built in temporary
storage from the official source archive; the host binary was Git 2.43.0.

Observed results:

- Git 2.43, GitHub URL: response with materialized worktree paths.
- Git 2.30, identical GitHub URL: `INTERNAL_ERROR`, with exactly
  `Failed to add branch: workspace source materialization failed: materialize branch: managed repository ownership mismatch`.
- Git 2.30, explicit local checkout: response with materialized worktree paths.

The temporary regression asserted success for every valid input: two controls
passed and the old-Git URL case failed. It was removed at the design checkpoint.
This is not native full-lifecycle acceptance with the production worktree manager.

Independent command evidence with Git 2.30:

```text
git rev-parse --path-format=absolute --show-toplevel
--path-format=absolute
<absolute checkout path>
```

The command exits zero. The
[official Git 2.32 release notes](https://github.com/git/git/blob/v2.32.0/Documentation/RelNotes/2.32.0.txt)
confirm that `--path-format` originated during the 2.31 series.

## Scope

### In scope

- Compatible absolute top-level and Git-directory inspection in `repoclone`.
- Relative common-directory resolution solely for comparison with an admitted
  checkout's inspected physical `.git` directory.
- Deterministic old-Git protocol regression and registered MCP URL consumer proof.
- Preservation of whitespace, trusted parent aliases, origin checks, unsafe-path
  refusals, exact persistence and attachment compensation.

### Out of scope

- Identifying the original reporters' environments without evidence.
- Changing clone destinations, ownership admission, credentials or repository identity.
- Changes to unrelated worktree inspectors, a global Git minimum-version policy,
  new configuration, migrations, flags, UI, public API arguments or error envelopes.
- Production deployment, release claims, PR creation or issue closure.

## Technical approach

Change only `apps/backend/internal/repoclone/managed_attachment_path.go`:
keep separate classified, sanitized Git queries, omit `--path-format`, use the
already absolute top-level and Git-directory results, and resolve relative
common-directory output against the checkout before physical comparison.
Never trim legitimate path bytes or accept a path because it merely resolves
somewhere under managed storage. Preserve all existing preinspection and
mutation-boundary guards.

Add focused tests in `managed_attachment_git_compatibility_test.go` and an MCP
consumer test in `managed_branch_git_compatibility_test.go`. A test-only Git
wrapper should reproduce Git 2.30's unsupported-option behavior while delegating
actual Git operations to the available binary. It must not return canned
successful paths or bypass the production ownership inspector. Reuse the
URL-to-disposable-local-transport wrapper and existing service fixtures for the
consumer test. A version-only stub is insufficient.

Compatibility boundaries:

- GitHub URL on current Git: maintain authenticated managed clone behavior.
- GitHub URL with pre-2.31 query behavior: admit a valid clone without this option.
- Explicit local checkout: retain its existing local route.
- Foreign origin, redirected control data or symlink at/below the clone base:
  continue to reject and preserve content.
- Other providers sharing inspection benefit from compatible queries, but these
  fixtures do not prove their external authentication or native platform support.

## Tests

- `TestManagedAttachmentGitPathCompatibility` in
  `apps/backend/internal/repoclone/managed_attachment_git_compatibility_test.go`:
  valid current checkout with old-Git output behavior; relative common dir,
  space/newline names and trusted outside-base alias. Maps to AC 008.1 and 008.2.
- `TestManagedBranchGitHubURLGitCompatibility` in
  `apps/backend/internal/backendapp/managed_branch_git_compatibility_test.go`:
  registered MCP GitHub URL with modern/old output behavior and explicit-local
  control, readable result, exact request scope and canonical inventory. Maps
  to AC 008.1, 008.3, 008.4 and 008.6.
- Existing managed-path security and branch/source consumer suites cover refusal,
  stale identity, compensation and retry under AC 008.2 through 008.6.

Use full `AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.N` annotations in permanent tests.

## End-to-end evidence

The registered MCP handler through SQLite and real Git is the affected consumer
boundary. No rendered UI changes are planned, so no Playwright suite or preview
is required. State the persisting test worktree-manager limitation in results.

## Work orders

- [x] [Task 01: Use compatible attachment path queries](task-01-compatible-path-queries.md)

## Verification results

Diagnostic evidence before implementation:

- `(cd apps/backend && go test -trimpath ./internal/repoclone -run 'TestManagedAttachment' -count=1)`: passed.
- `(cd apps/backend && go test -trimpath ./internal/backendapp -run '^TestManagedBranchAttachmentConsumers$' -count=1)`: passed.
- Temporary `TestReproGitHubURLAttachmentGitCompatibility`: expected RED;
  current URL and old-Git local controls passed, actual Git 2.30 URL failed
  with the reported exact error. Temporary test and downloaded build removed.

The user subsequently requested implementation. The production correction and
both permanent regressions are implemented. The exact work-order race commands
passed (`repoclone`: 1.536s; `backendapp`: 56.494s), changed-package lint passed
with zero issues, and specification catalog/lint, reference and diff checks
passed. Full command receipts and platform/test-double limits are recorded in
the [completed work order](task-01-compatible-path-queries.md#results).

Design-package checks:

- `python3 scripts/list-docs.py validate`: passed (399 decisions, 1718 specifications).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- Work-order requirement/AC existence, design requirement mapping, linked design
  paths and plan/work-order linkage: passed by focused read-only reference check.
  The repository's Node PR-documentation helper could not run because `node`
  was unavailable on PATH at the design checkpoint. During delivery, the Mise
  Node runtime was located and the repository's `validateCoverage` preflight
  passed with `status: covered` for this work order and its complete references.
- `git diff --check -- docs/specs docs/plans/managed-attachment-git-compatibility`:
  passed. Package files remain unstaged and uncommitted.
- Removed the owned downloaded Git source/build/logs and local diagnostic ZIP.

## Risks

- Physical resolution must remain comparison-only after ownership inspection.
- Using `TrimSpace`, splitting path output on newlines, or joining against the
  process CWD would introduce path regressions.
- A simulated old-Git protocol is deterministic coverage, not acceptance on
  every old Git release or native Windows/macOS filesystem.
- Other independent ownership refusals remain possible for the external reports.

## Delivery checkpoint

Requirements remain active and unchanged; the owning design includes the
compatibility section. Implementation was explicitly requested in the later
user turn and completed through the linked work order with TDD. The user then
authorized delivery through merge. Remote CI/review and merge receipts belong
to the PR and session handoff. No live-instance changes are part of delivery.
