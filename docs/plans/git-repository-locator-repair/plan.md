---
created: 2026-11-08
status: implemented
requirements:
  - REQ-WORKSPACES-REPOSITORY-LOCATORS-001
  - REQ-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-002
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
system_design:
  - ../../specs/workspaces/system-design/repository-locators.md
  - ../../specs/workspaces/system-design/remote-repository-registration.md
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
legacy_specs: []
---

# Implementation Plan: Git Repository Locator Repair

## Overview

Repair SCP admission for remote workspace materialization first.
Then add validated, identity-preserving clone-URL repair through HTTP, WebSocket, and the existing compact MCP settings tools.
Task 01 is independently deliverable. Tasks 02 and 03 deliver the separate public-surface feature.

This is the implementation record for [#5182](https://github.com/kdlbs/kandev/issues/5182) and [#5188](https://github.com/kdlbs/kandev/issues/5188).
The three work orders are implemented and their local verification is recorded below. Files remain unstaged and uncommitted.
The container smoke and physical PostgreSQL gates remain unexecuted because their infrastructure is unavailable.

## Investigation and confidence

Evidence was refreshed on 2026-11-08 against checkout and remote main `9aba7b2edd020f6ba0f92c2bfe318429723ea2ed`.
The earlier parent revision was `02b1267f7f279565cd2a9496caa429fd701607f0`.
The worktree was initially clean. No parent checkout, live deployment, user database, credential store, or journal was changed.

### Observed facts

1. Both canonical issues were open and unassigned, with zero comments and no attachments.
   `gh api user --jq .login` returned `carlosflorencio`.
2. A refreshed inventory contained 146 open PRs, without explicit issue references or a semantically covering fix.
   Canonical timeline queries returned no linked fix PRs.
3. `workspace_materialize.go:validateRepositoryLocator` calls `url.Parse` before its SCP branch.
   `url.Parse` rejects `git@host:path`. The later condition also uses a character set containing space, backslash, and `t`.
4. A disposable Go program copied the exact validator and helper functions from this checkout.
   GitHub, GitLab, and self-managed SCP inputs failed both materialize and removal validation.
   HTTPS passed. Credentialed HTTPS and `ssh://git@...` failed.
   Local/file locators failed materialization but passed removal's existing cleanup-only policy.
   The character probe reported `t=true`, `tab=false`. The program exited 0 and its temporary directory was removed.
5. `reconcileLaunchWorkspace` invokes agentctl materialization for secondary repositories on clone-based executors.
   Prepare scripts can establish the primary without invoking that validator.
   `validateRemovalRepositoryLocator` delegates remote inputs to the same defective validator.
6. `httpUpdateRepositoryRequest` and `wsUpdateRepositoryRequest` omit `remote_url`.
   `UpdateRepositoryRequest.RemoteURL` exists, and `applyRepositoryUpdates` only trims/stores it.
   `FindOrCreateRepository` fills an empty URL and preserves an existing nonempty one.
7. Contrary to #5188's historical MCP premise, current `settingscatalog.repositoryDomain` declares `remote_url` writable.
   Compact `get_settings_kandev` and `update_settings_kandev` already reach the repository service through guarded adapters.
   The task-level `update_repository_base_branch_kandev` tool is a different operation.
8. `repository_checkout_defaults.go:repositoryUpdateQuery` writes `remote_url` from the service snapshot even when omitted.
   Its special omission protection currently covers only default branch and pull policy.
   `publishRepositoryEvent` currently omits the clone URL.
9. Managed credential identity already converts supported SSH origins through `repoclone.CanonicalHTTPSCloneURL`.
   The targeted existing identity tests passed. The package must preserve #2673's fixes rather than reinstate an HTTPS-only credential policy.

The source-derived probe can be repeated outside the repository: copy the span from `func validateRepositoryLocator` through the removal validator into a temporary Go main.
Use only its standard-library imports, run `go run -trimpath <temporary-main.go>`, and remove the directory afterward.
Do not edit or install a permanent regression test during Analysis.

### Conclusions and limits

- **#5182: high confidence.** Both deterministic faults are confirmed on current main.
  The source trace establishes the secondary-repository failure path. No live Kubernetes or remote bootstrap reproduction ran in this turn.
- **#5188: high confidence in the remaining feature gap.** HTTP/WS omission, unvalidated service writes, and absence of a domain preview are confirmed.
  MCP raw update already exists. A generic managed-GitHub SSH failure is not reproduced on current main and is not a feature premise.
- The issue's proposed SCP helper is insufficient as a complete grammar: it admits paths such as `/absolute`, `group//repo`, and `group/../repo`.
  The design defines the narrow supported grammar instead.
- Clone reachability, deployment-specific credentials, and retained-checkout behavior require the planned integration evidence.
  Static admission and credential unit tests cannot prove them.

## Assumption check and ownership

The user already settled registration identity preservation, authorization, credential rejection, supported surfaces, and the planning-only boundary.
No material user question blocks this package.
The narrower API/MCP workflow satisfies the repair request without a speculative settings editor.

Workspaces owns the package because it owns registrations, stored locators, and source preparation.
The runtime locator grammar lacked an explicit requirement. The new locator pair fills that gap.
The repair feature extends the existing remote-registration pair instead of creating a duplicate repair spec.
Active checkout-default AC .20 through .23 remain compatibility requirements, not newly invented repair criteria.

The [validated-update ADR](../../decisions/2026-11-08-validated-repository-locator-updates.md) records saved-identity pinning, advisory preview, and database omission preservation.
It is accepted for implementation.

## Scope

### In scope

- Strict conventional `git@host:path` admission before URL parsing, shared by materialize and remote removal.
- Credential, malformed-input, local-path, control, and delimiter rejection without fixture exceptions.
- Real handler/client/lifecycle regression evidence and a separate disposable remote smoke.
- Authorized inspect, validate, update, and readback of one registration's clone URL across HTTP/WS/MCP.
- Built-in identity checks and active plugin descriptor validation pinned to the saved provider.
- Ordinary URL omission preservation, identity fencing, committed response/event projection, and atomic secret-binding compatibility.
- Public documentation for the supported API/MCP repair procedure and syntax limits.

### Out of scope

- Git-origin edits, checkout replacement, session restart, automatic re-registration, schema/data migrations, or provider-identity repair.
- Rewriting managed SSH credential rules or building another general Git URL parser.
- New web controls, screenshots, localization strings, or viewport-dependent behavior.
- Production local/file URL exceptions, plugin executor SDK work, or unrelated refactors.
- Other approved groups and deferred issues #5191, #5186, #5203, and #5169.
- Commit, push, PR, publication, delegation, or persistent follow-up work.

## Technical approach

### Locator admission

Task 01 adds a small neutral `internal/common/gitlocator` parser for the design's conventional SCP subset.
Agentctl validates that form before `url.Parse`, with a final refusal for malformed `git@` forms.
Existing URL schemes remain available without URL user information. Raw queries, fragments, whitespace, and controls fail.
No stored value or requested origin is rewritten.

Keep direct-child destination containment, classified Git subprocess admission, managed runtime environment, exact origin checks, and quarantine cleanup intact.
Keep the removal-only local-origin recognition distinct from clone permission.
Add new tests beside the large existing test file rather than extending it past backend limits.

### Validated registration repair

Task 02 adds one domain validator and a read-only preview result to the repository service.
It pins preflight to the saved identity and compares the full result afterward.
It extends ordinary repository write intent, plus its binding companion, with a validated URL and observed identity fence.
The SQL statement preserves omitted URLs and returns the committed URL, checkout pair, and timestamp.
Existing exact and deliberate full-write paths retain their contracts.

Task 03 exposes that domain path through the registered HTTP/WS handlers and compact settings adapter.
The existing MCP update uses `options.validate_only` for a URL-only preview. Update always validates again.
Add safe `remote_url` projection to repository update events, which currently omit it.
Preview and error responses redact unsafe old or proposed locators.

### Provider and transport compatibility

| Provider / transport | Identity / behavior | Evidence | Unsupported-shape outcome |
| --- | --- | --- | --- |
| GitHub/GitLab/self-managed SCP at agentctl | Strict `git@` grammar; nested relative paths; exact origin retained | Pure parser, real agentctl handlers, lifecycle integration, Kubernetes smoke | Malformed host/path/user, SCP port/IPv6, controls, local path: reject |
| Existing agentctl network URLs | Existing `https/http/ssh/git` schemes; no URL user information | Handler rejection/acceptance controls | User information, query/fragment, file/path: reject |
| GitHub HTTPS/SCP registration repair | Complete saved provider origin and owner/name; preserve managed SSH normalization | Service + guarded route tests; existing credential tests | Unknown saved identity or changed repository: reject |
| Public/self-managed GitLab repair | Exact configured origin/port and full namespace/name; SCP repair requires matching default-port HTTPS identity | Origin-verifier service tests + route tests | Missing/mismatched connection or unprovable SCP scheme/port: reject |
| Azure DevOps repair | HTTPS clone URL; saved evidence must prove organization/project/repository | Positive complete-identity and negative unknown-identity tests | Legacy incomplete organization/host: reject without guessing |
| Plugin provider repair | Pinned active manifest owner, immutable ID/scope, host/namespace/name, exact HTTPS descriptor path | Real fixture inspection + guarded MCP/HTTP/WS | Inactive owner, mismatched scope/context/descriptor: reject |
| Local registration with provider identity | Same provider validation; source type, local path, and Git origin stay stable | Real temporary checkout and registration tests | Unlinked local or empty saved host: reject |

Shared implementation does not make every legacy provider shape repairable.
No production hostname, credential, or local-URL exception is needed for fixtures.

## Tests

All names below are proposed permanent tests for a later implementation turn.
Work-order verification blocks contain the exact selectors and working directories.

| Requirement / criteria | Test evidence |
| --- | --- |
| LOCATORS-001.1, .2 | `common/gitlocator/scp_test.go:TestParseConventionalSCP`; GitHub, GitLab, DNS single-label, IPv4, nested paths, letter `t` |
| LOCATORS-001.2, .3 | Same table plus `workspace_materialize_locator_test.go:TestWorkspaceRepositoryLocatorRejections`; empty host/path, extra separators, alternate users/passwords, whitespace/control, query/fragment, path/drive/file/UNC |
| LOCATORS-001.3, .5 | Existing local/credential refusal and quarantine tests; new registered `TestWorkspaceSCPLocatorMaterializeAndRemove` with real temporary Git origins |
| LOCATORS-001.4, .5 | `workspace_materialization_scp_test.go:TestRemoteWorkspaceSCPLocators`; primary prepared separately, secondary clone, failure after new+reused entries, reverse rollback, rescan, single-repository control |
| REGISTRATION-002.1-.4, .8-.10 | `service_repository_remote_url_test.go:TestRepositoryRemoteURLValidation`, `TestRepositoryRemoteURLPreview`, `TestRepositoryRemoteURLAccess`; identity pinning, plugin changes, saved local source, empty legacy host, no Git calls |
| REGISTRATION-002.6-.8 | `TestRepositoryRemoteURLConcurrentSQLite`, `TestRepositoryRemoteURLPhysicalPostgres`, `TestRepositoryRemoteURLAtomicity`; independent service/store instances, both write orders, disjoint branch/pull/URL, same-field commits, identity race, binding rollback, exact conflicts |
| Active BASE-REFRESH-001.20-.23 | Existing `TestRepositoryCheckoutDefaults*`, registered route controls and exact Host controls; no weakening of two-choice guarantees |
| REGISTRATION-002.1-.10 | Registered `TestRepositoryRemoteURLHTTP`, `TestRepositoryRemoteURLWS`, `TestRepositoryRemoteURLGuardedMCP`; inspect/preview/update/readback, scope denial, read-only, missing/null/blank, typed failures, event assertions |

Hostile URL cases include `https://token@host/repo`, `ssh://user@host/repo`, non-git SCP users, empty query/fragment suffixes, and encoded authority/separator tricks.
Do not accept an issue/PR/browser resource URL by silently truncating it to owner/name.
Plugin tests retain exact context paths instead of reconstructing URLs from the display identity.

## E2E evidence

The API repair flow uses registered backend routes, real persistence, real event publication, and the actual guarded compact MCP dispatcher.
This is the required end-to-end evidence for the data-only feature. It does not claim browser rendering or live-provider access.
Desktop and phone use the same persisted registration and runtime path. No composition changes require a mobile preview or new mobile test.

Task 01 adds `apps/web/e2e/tests/kubernetes/kubernetes-scp-multi-repository.spec.ts` in the `containers` project.
Use existing Kind and worker-image fixtures, with a disposable SSH Git server derived from the existing SSH test image.
Bind a `git` fixture user, isolated keys, known-host data, and seeded bare repositories inside disposable infrastructure.
Do not alter a developer SSH configuration or disable checks in production.

The smoke covers LOCATORS-001.4/.5: a primary prepare script plus a secondary SCP checkout, both files visible before the mock agent runs.
Assert persisted source inventory, repeat/reuse, a single-repository control, and cleanup after a later injected preparation failure.
The API and filesystem assertions can run without browser interaction. They exercise the actual remote boundary.
Keep deterministic tests and the real remote smoke separately reported.

## Dependencies and overlap

- [#4934](https://github.com/kdlbs/kandev/pull/4934) is open at `b8671b993d48dc819fb48fd698b9c296c182155c`, with PR base `56af766dd9e9571e00e3c948426d75bf1c88b5a9`.
  Its actual file patch adds `isE2ELocalFileLocator` after network parsing and leaves both SCP faults untouched.
  It is not a dependency or a covering fix. Re-resolve its head and re-read the validator if it lands.
  This package does not absorb its fixture exception, native-worker contracts, or runtime work.
- [#2673](https://github.com/kdlbs/kandev/issues/2673) closed through #2678; #2728 and #2869 add SSH/hostile-authority coverage.
  Preserve those guarantees. The current credential tests are evidence against repeating the historical HTTPS-only premise.
- [#4031](https://github.com/kdlbs/kandev/issues/4031) and the [registration package](../settings-remote-repository/plan.md) own create/find-or-create behavior.
  Do not change their existing implemented work order or claim that registration replaces a stored nonempty URL.
- [Provider-owned discovery](../office-project-self-managed-gitlab/plan.md) is already present on main.
  Reuse its configured-origin and plugin-authority seams, while repair remains explicitly pinned rather than unhinted discovery.
- [Checkout-default preservation](../preserve-repository-checkout-defaults/plan.md) records the active omission contract now present in source.
  Extend its ordinary SQL path without reopening or changing that package's historical results/status.

The five task groups share no required new dependency for this package.
Resolve main and overlap heads again before implementation. Concurrent source or contract changes require reconciliation first.

## Documentation impact

Internal specifications, this package, and the accepted ADR are updated now.
Public docs remain unchanged during planning.
Task 03 updates the repair procedure in `docs/public/git-operations.md` (explanation with a focused recovery section).
It adds the HTTP/WS contract to `docs/public/websocket-api.md` (reference) and compact settings examples to `docs/public/automation-and-mcp.md` (reference section).
Examples show inspect, validate, update, and readback for one registration ID without a database workaround or Git-origin edit.
State static versus provider inspection, unknown-identity refusal, and existing-checkout limits.
Root README and `docs/screenshots.md` need no change because navigation, controls, and screenshots do not change.

## Work orders

- [x] [Task 01: Admit conventional SCP workspace locators](task-01-scp-workspace-locators.md)
- [x] [Task 02: Validate and persist registration clone-URL changes](task-02-validated-registration-repair.md)
- [x] [Task 03: Expose inspect, preview, update, and readback](task-03-supported-repair-surfaces.md)

Order: 01, then 02, then 03. Task 01 does not depend on the public feature, PostgreSQL feature tests, or Task 03 documentation.
Execution is sequential in the primary session. No delegation is authorized.

## Verification results

Implementation evidence on 2026-11-08:

- Task 01: `go test -trimpath ./internal/common/gitlocator ./internal/agentctl/server/api -run '...(locator cases)...' -count=1` PASS.
  `go test -trimpath -race ./internal/agent/runtime/lifecycle -run 'Test(RemoteWorkspaceSCPLocators|MaterializeWorkspaceRepositories|RemoteWorkspaceProjection)' -count=1` PASS.
  `go test -trimpath ./internal/gitcredentials -run '^TestResolveRepositoryIdentity' -count=1` PASS.
  The real route test materializes and removes an SCP checkout through a temporary `insteadOf` map while retaining the SCP origin.
- Task 02: `go test -trimpath -race ./internal/task/service ./internal/task/repository/sqlite -run 'Test(RepositoryRemoteURL|RepositoryCheckoutDefaults|Service_FindOrCreateRepository.*SSH)' -count=1` PASS.
  `go test -trimpath ./internal/gitcredentials ./internal/repoclone -run 'Test(ResolveRepositoryIdentity|CanonicalHTTPSCloneURL|ValidateHTTPSCloneOrigin)' -count=1` PASS.
  `go run ./cmd/sqlguard ./internal` PASS. `go test -trimpath -race ./internal/persistence/storeconformance -count=1` PASS.
  `TestRepositoryRemoteURLPhysicalPostgres` reports the documented skip because `KANDEV_TEST_POSTGRES_DSN` is absent.
- Task 03: registered HTTP/WS route tests, the guarded compact MCP dispatcher, settings catalog metadata tests, and the checkout-default controls PASS.
  `node --test scripts/validate-public-docs.test.mjs` passed 63 tests, and `node scripts/validate-public-docs.mjs` validated 55 pages.
- The Kubernetes multi-repository SCP smoke is not present because the fixture cannot register an arbitrary SCP host as a provider identity.
  Registering such a host would require a new supported provider connection or a direct database seed, which this package does not authorize.
  A raw agentctl smoke remains possible only with separate fixture work; record it as an unexecuted container gate.
- No permanent browser/mobile interaction was added, so no browser run is claimed.
- Full affected Go packages passed: `./internal/task/handlers`, `./internal/task/service`, `./internal/task/repository/sqlite`, `./internal/backendapp`, `./internal/mcp/handlers`, `./internal/mcp/server`, and `./internal/settingscatalog`.
  Scoped Go lint reported zero issues, and `go build ./...` passed.
- Review follow-up fixes are covered by targeted service, handler, and guarded compact MCP tests: strict boolean and unknown-option parsing at both boundaries, missing/conflicting saved origins, exact identity preservation, `validate_only: false` ordinary updates, and encoded/Unicode HTTPS paths with hostile escape controls.
- Fixup review also added exact proposed-origin and port comparison before retaining encoded bytes, safe display of encoded paths in previews and events, missing-row identity-fence conflicts, independent conflict-test deadlines, changed-plugin-descriptor coverage across HTTP/WS/MCP, a refreshed settings discovery snapshot, and aligned locator admission documentation.

Planning evidence remains in the investigation section. Validation of generated documentation, links, anchors, and coverage is repeated below.

### Repeat document validation

Run from the repository root:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/decisions docs/plans/git-repository-locator-repair
git status --short -- docs/plans/git-repository-locator-repair
node <<'JS'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const changed = [...new Set([
  ...execFileSync('git', ['diff', '--name-only'], {encoding: 'utf8'}).trim().split('\n'),
  ...execFileSync('git', ['ls-files', '--others', '--exclude-standard'], {encoding: 'utf8'}).trim().split('\n'),
].filter(Boolean))];
const refs = [
  'docs/specs/workspaces/requirements/worktree-base-refresh.md',
  'docs/specs/workspaces/system-design/worktree-base-refresh.md',
];
const fileContents = Object.fromEntries([...new Set([...changed, ...refs])]
  .filter(p => p.endsWith('.md') && fs.existsSync(p))
  .map(p => [p, fs.readFileSync(p, 'utf8')]));
const runtime = [
  'apps/backend/internal/agentctl/server/api/workspace_materialize.go',
  'apps/backend/internal/task/service/service_resources.go',
];
const result = validateCoverage({
  changedFiles: [...new Set([...changed, ...runtime])].map(filename => ({filename, status: 'modified'})),
  fileContents,
});
console.log(JSON.stringify({status: result.status, workOrders: result.workOrders, errors: result.errors}));
process.exit(result.ok ? 0 : 1);
JS
```

## Issue assignment

After confidence and fresh ownership/PR checks, both issues were assigned to `carlosflorencio`.
Post-write canonical API reads confirmed that sole assignee on both still-open issues.
No issue or PR comments were posted, and neither issue was closed.

## Risks

- The strict conventional SCP subset excludes IPv6, explicit ports, unusual path characters, and server absolute paths.
  Do not widen it without a separate contract and security assessment.
- Existing complete-model writers remain deliberate writes. Ordinary omission protection is not universal writer coordination.
- Unknown provider identity and Azure organization ambiguity fail closed. This package cannot recover metadata it cannot prove.
- Plugin descriptor or installation changes between preview and update can refuse a previously valid proposal.
- A repaired registration does not automatically reconcile existing checkout origins or discard unpublished work.
- Real remote smoke requires Docker/Kind resources and isolated SSH fixtures. Physical PostgreSQL requires an explicit test DSN.
  Missing infrastructure is an unexecuted verification gate, not a passing result.
