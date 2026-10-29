---
created: 2026-10-29
status: complete
requirements:
  - REQ-TASKS-PROJECT-REPOSITORIES-001
  - REQ-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001
  - REQ-PLUGINS-REPOSITORY-URL-DISCOVERY-001
  - REQ-PLUGINS-REPOSITORY-TASK-CREATION-001
system_design:
  - ../../specs/tasks/system-design/project-repository-context.md
  - ../../specs/workspaces/system-design/remote-repository-registration.md
  - ../../specs/plugins/system-design/repository-url-discovery.md
  - ../../specs/plugins/system-design/repository-provider-task-creation.md
legacy_specs: []
---

# Fix plan: Provider-neutral project repository URLs

## Overview

Make Office project URLs work with configured built-ins and active VCS plugins.
First add server-side plugin discovery, then use it in shared repository admission.
Finally prove the existing project-to-task flow on desktop and phone.

Issue: [kdlbs/kandev#4942](https://github.com/kdlbs/kandev/issues/4942).
Assigned user: `carlosflorencio`, verified through the connected GitHub account.
Investigated base: `012fb3c04069ba7154d9f5afdcddca49a8f3ba1c`, equal to remote `main` at investigation.
The issue was open with no comments or attachments.

The user extended the desired behavior to Bitbucket, Forgejo, and other compatible VCS plugins.
The directory retains the original issue slug so existing links remain valid.
All three work orders are complete. The implementation adds server-side plugin discovery,
shared provider admission, desktop and phone browser evidence, and public documentation.

## Diagnosis and evidence

The task system owns project-source admission because it selects attachments before task publication.
Office supplies the source strings. Workspaces owns repository identity, and Integrations owns connection metadata.

Confirmed path:

1. `projectRepositoryInputs` creates remote inputs without provider hints or trust markers.
2. `preflightRepositoryInputs` tries `inferEnterpriseGitHubProvider` for an unrecognized host.
3. A configured GitLab origin cannot satisfy that GitHub check, so creation returns an invalid-selection error.
4. An explicit `gitlab` hint passes parsing, but preflight does not call the GitLab origin verifier.
5. `resolveRepoInputRemote` then refuses the input through its `untrusted GitLab origin` guard.
6. `RegisterRemoteRepository` succeeds because it separately calls `verifyRemoteRepositoryOrigin` after preflight.

Three temporary service reproductions used the actual task service and SQLite fixtures.
The registration control used the existing verifier stub and persisted the correct GitLab origin.
No live GitLab service or credentials were required.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/service -run '^TestRepro4942' -count=1 -v)
```

Observed failures:

| Temporary test | Evidence |
| --- | --- |
| `TestRepro4942ConfiguredGitLabProjectSource` | Registration succeeded, but project task creation returned `The selected repository could not be verified.` The GitLab verifier received no call. |
| `TestRepro4942HintedGitLabPreflightMustVerifyOrigin` | Preflight returned success with `TrustedRemote=false` and zero verifier calls. |
| `TestRepro4942HintedGitLabTaskSource` | Creation returned `untrusted GitLab origin "https://gitlab.example.test"` with zero verifier calls. |

The temporary file `issue4942_repro_test.go` was removed after diagnosis.
Task 02 now owns the permanent expected-success regressions that replaced the temporary reproductions.

The frontend claim in the issue does not match this base.
`app/office/projects/project-repository-picker.tsx` already accepts custom URLs through `shouldShowCustomEntry`.
It does not use the separate `workspace-source-picker/remote-url.ts` host allowlist.
The package therefore needs no frontend production change or rendered UI preview.

## Specification reconciliation

Project criteria .10/.11 retain GitLab admission and refusal behavior. New .12/.13 define plugin sources and ambiguity.
The new plugin discovery requirement owns reusable server-side eligibility, inspection, and lifecycle rules.
The existing explicit plugin selection requirement remains active and compatible.
The new [discovery ADR](../../decisions/2026-10-29-provider-owned-repository-url-discovery.md) records provider-owned recognition and unique-candidate selection.
Workspace registration retains criteria .3, .4, and .6 while sharing the expanded preflight.

GitLab connection ownership and GitHub Enterprise identity remain governed by their integration requirements.
Ambiguous unhinted custom hosts now fail instead of taking GitHub-first precedence.
Public built-in URLs and explicit provider selections retain their existing routing.

Completed companion packages are [project context](../office-project-repository-context/plan.md),
[remote registration](../settings-remote-repository/plan.md),
[explicit plugin selection](../plugin-repository-task-resolution/plan.md), and
[URL cache refresh](../refresh-every-remote-url/plan.md).
Their completed results remain historical evidence. This package owns the additional discovery scenarios and verification.

The existing `TestCreateTaskRejectsUnresolvedPluginURLWithMissingOrBuiltinProviderBeforePersistence` proves current no-hint refusal.
Keep its no-resolver and explicit-wrong-provider cases. Add discovery-enabled success separately.
The inspected Bitbucket and Forgejo manifests both declare the existing inspection capability.
Provider manifests prove capability availability, not completed live-plugin verification.

## Scope

### In scope

- Bare project URLs resolved through verified workspace connections or active repository plugins.
- Bitbucket Cloud/Data Center, Forgejo, and future plugins through the same declared capability.
- Explicit selections, public providers, saved IDs, local inputs, and trusted Host descriptor compatibility.
- Exact origin/scope/immutable identity, source ordering, and canonical descriptor preservation.
- Unknown, ambiguous, canceled, stale-generation, and malformed-result refusal before writes.
- Desktop/phone Office proof and short Office/plugin-authoring documentation notes during implementation.

### Out of scope

- Hard-coded plugin hostnames or an unknown-host GitLab parser fallback.
- Relaxing the unrelated workspace-source picker or changing Office UI composition or copy.
- New wire actions, SDK APIs, external plugin edits, clone protocols, or authentication methods.
- Checkout, credential, executor, schema, project-storage, backfill, and explicit-selection precedence changes.
- Live vendor deployments and a new cross-table transaction.

## Technical approach

### Plugin discovery boundary

Add `DiscoverRepositoryProviders` beside the existing plugin inspection implementation.
Snapshot active manifest-owned repository providers with workspace `repositories.inspect` actions.
Ask eligible providers to inspect the URL under verified workspace context.
Keep normal no-match, typed candidate failure, and fatal operation failure distinct.
Return every validated successful descriptor, with bounded concurrency and the existing action limits.
Recheck candidate installation ownership before accepting the result. Never select by completion or registry order.
The [plugin design](../../specs/plugins/system-design/repository-url-discovery.md) defines limits and result semantics.

Extend the task resolver and backend adapter with `DiscoverRepositorySelections` and a discovery result type.
It carries authoritative candidates and ordinary candidate failure separately from fatal operation errors.
This adds no new public transport or plugin action.

### Shared admission boundary

Add `resolveWorkspaceRemoteProvider` in `internal/task/service/workspace_remote_provider.go`.
For unhinted non-public URLs, gather verified GitHub/GitLab candidates and plugin discovery candidates.
Accept exactly one successful candidate. Reject ambiguity across built-ins and plugins.
Caller cancellation, discovery deadline, and changed eligibility invalidate the attempt even when a candidate already succeeded.
An ordinary unrelated provider failure does not replace a unique, completed positive match.

Use a discovery-specific wrapper around the existing descriptor trust helper.
Keep explicit-provider equality and request pinning checks intact.
Persist only the server descriptor, preserving plugin context paths and clone URLs.
Apply built-in origin verification to explicit GitLab inputs and propagate verified inputs through the actual slice.
Remove registration's duplicate check only after shared preflight owns it.
Keep the strict parser, local/saved guards, explicit plugin path, external-ID ordering, and lower GitLab trust gate.

The [task design](../../specs/tasks/system-design/project-repository-context.md#workspace-verified-remote-admission)
defines arbitration and preflight ordering. The [registration design](../../specs/workspaces/system-design/remote-repository-registration.md#failure-and-recovery)
retains its origin and persistence contracts.

### Compatibility matrix

| Input | Provider evidence | Expected result | Verification | Unsupported result |
| --- | --- | --- | --- | --- |
| Public GitHub, GitLab HTTPS, Azure HTTPS/SCP | Existing parser | Existing identity and admission | Existing parser and project tests | Existing selection error |
| Unhinted self-managed GitLab HTTP(S), including custom port and subgroup | Exact configured workspace origin and authentication method | GitLab identity and internal trust marker if uniquely matched | New project and adapter tests | Invalid, unavailable, or ambiguous selection |
| Explicit GitLab hint | Parser plus the same origin verifier | Same admission as project inputs | New preflight tests and existing registration tests | No hint-based trust bypass |
| GitHub Enterprise HTTPS/SCP and GHE.com | Existing workspace GitHub verifier | Preserve unique host identity | Existing Enterprise tests and mixed-provider project case | Invalid or ambiguous selection |
| Bare plugin URL, including Bitbucket context path or Forgejo host | Active manifest owner and workspace inspection | Preserve exact validated descriptor without provider hint | New plugin/task discovery tables and packaged fixture E2E | Typed refusal |
| Two successful custom-host providers | Multiple verified candidates | Refuse before writes | Built-in/plugin and plugin/plugin ambiguity tests | Invalid selection |
| Unknown or foreign-workspace HTTP(S) host | No candidate authorizes it | Refuse before writes | Both mixed-source orders and workspace adapter tests | Invalid or typed provider failure |
| GitLab custom-host SSH/SCP | No current built-in GitLab parser support | No new built-in acceptance | Unsupported-transport case | Invalid unless an eligible plugin supports the URL and returns a valid HTTPS descriptor |
| Saved ID or local path | Existing workspace/local admission | No remote origin parse | Existing project-selection tests | Existing ownership or path error |
| Plugin selection or trusted host descriptor | Existing plugin resolver/host authority | Preserve exact provider descriptor | Existing repository-selection tests | Existing typed plugin error |

An explicit provider hint never falls back to another provider.
Multiple successful inspectors always produce ambiguity for an unhinted custom URL.
All selected entries must pass preflight. One authorized source cannot authorize another source.
An origin refusal produces no new rows or creation event.
Later non-origin resolution errors retain the existing row lifecycle.

### Existing desktop and phone flow

Task 03 extends the existing Office New Task suites.
Use the actual project picker to save a custom GitLab or plugin URL before selecting that project in New Task.
Assert the saved source after reload and the returned and persisted task attachment.
Configure the connection in `dialogSeed.workspaceId` on desktop and `officeSeed.workspaceId` on phone.
Use the existing mock GitLab connection and real packaged repository-plugin fixture with disposable data.
Leave the task unassigned so this admission test does not start an external clone.

The existing project dialog and New Task phone surface remain the mobile exemplars.
Use `.tap()` for phone actions and preserve existing fixtures and cleanup.
No layout, navigation, touch target, copy, or breakpoint change is planned.

## Tests

All project AC suffixes below use `AC-TASKS-PROJECT-REPOSITORIES-001`.
The test names below are the implemented regressions; the results sections record their outcomes.

| Criteria | File and test |
| --- | --- |
| Plugin discovery .1-.7, .9 | New `plugins/repository_provider_discovery_test.go`: `TestDiscoverRepositoryProviders`, with Bitbucket, Forgejo, unknown future provider, no-match, typed failures, invalid descriptors, scope, ambiguity, cancellation, bounds, and changed ownership |
| Plugin discovery .3, .8 | `backendapp/repository_selection_test.go`: `TestPluginRepositorySelectionDiscoveryUsesAuthoritativeDescriptors` and existing explicit adapter tests |
| .1-.3, .10, .12 | New `task/service/project_remote_provider_test.go`: `TestCreateTaskProjectRemoteProviders`, with self-managed GitLab (custom port and subgroup), a bare plugin descriptor, registered/unregistered identities, and mixed sources |
| .6, .11, .13 | Same file: `TestProjectRemoteProviderFailuresBeforeWrites`, including multiple verified providers, disconnected/foreign workspace, invalid clone origins, cancellation, and both source orders |
| .10, .12, .13 and registration .3, .6 | New `task/service/workspace_remote_provider_test.go`: `TestRepositoryPreflightRemoteProviderDiscovery`, with pinned-provider isolation, explicit GitLab trust propagation, unique-match/error arbitration, and bare registration |
| .6, .10-.13 | New `backendapp/repository_origin_task_test.go`: `TestCreateTaskProjectRemoteProviderAdapters`, using real origin and plugin-selection adapters with verified workspace context |
| Registration .3, .4, .6 | Existing `TestRegisterRemoteRepositorySelfManagedGitLab` and registration suite, including exactly one verification for an explicitly hinted registration |
| Compatibility | Existing parser, Enterprise, project precedence, explicit plugin, idempotency, saved IDs, and direct untrusted-origin regressions |

The adapter test must prove that the selected workspace reaches the connection reader.
Refusal assertions cover repository/task/attachment rows and absence of `task.created`.
Use `@covers` comments for the affected ACs.

## E2E tests

Add `self-managed GitLab project sources attach before launch` and `installed plugin project URL resolves without a provider hint` to:

- `apps/web/e2e/tests/office/new-task-dialog.spec.ts`, project `chromium`.
- `apps/web/e2e/tests/office/mobile-new-task-dialog.spec.ts`, project `mobile-chrome`.

Map them to .1, .3, .9, .10, and .12.
Save the source with the existing custom-entry picker, reload the project, and create an unassigned task through New Task.
Verify the GitLab provider, exact origin, subgroup identity, and persisted attachment.
Add a disconnected or mismatched-origin submission through the same form and assert visible error with no created task.
For the plugin case, install the real packaged fixture and preserve its exact clone URL, provider ID, and repository ID.
Disable it and retry a new project source. Verify visible refusal and no new task/repository rows.
Map that failure to .6 and .11, plus plugin discovery .7.
Retain the existing explicit-plugin desktop/phone suites as compatibility evidence.
Service tests provide the exhaustive ambiguity and refusal matrix.
These tests prove admission and persistence. They do not claim live cloning or first-launch authentication evidence.

## Work orders

- [x] [Task 01: Discover active plugin repository providers](task-01-discover-plugin-providers.md)
- [x] [Task 02: Resolve project URLs through shared provider admission](task-02-resolve-project-providers.md)
- [x] [Task 03: Prove provider-neutral Office flows](task-03-prove-office-provider-flows.md)

Run Task 01, Task 02, then Task 03. All are sequential. No delegation is authorized.
Each work order lists its exact commands and owned files.

## Documentation coverage preflight

Run from the repository root. The runtime path below models the planned production change.

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const packageDir = 'docs/plans/office-project-self-managed-gitlab';
const documents = fs.readdirSync(packageDir).filter(p => p.endsWith('.md')).map(p => `${packageDir}/${p}`);
documents.push('docs/specs/tasks/requirements/project-repository-context.md', 'docs/specs/tasks/system-design/project-repository-context.md', 'docs/specs/workspaces/requirements/remote-repository-registration.md', 'docs/specs/workspaces/system-design/remote-repository-registration.md');
documents.push('docs/specs/plugins/requirements/repository-url-discovery.md', 'docs/specs/plugins/system-design/repository-url-discovery.md', 'docs/specs/plugins/requirements/repository-provider-task-creation.md', 'docs/specs/plugins/system-design/repository-provider-task-creation.md');
const fileContents = Object.fromEntries(documents.map(p => [p, fs.readFileSync(p, 'utf8')]));
const changedFiles = [...documents.map(filename => ({ filename, status: 'modified' })), { filename: 'apps/backend/internal/task/service/repository_selection.go', status: 'modified' }];
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({ status: result.status, workOrders: result.workOrders, errors: result.errors }, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
```

## Verification results

### Implementation and final verification

- `(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/service -count=1)`: passed, 368.895s.
- `(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/plugins -run '^Test(DiscoverRepositoryProviders|InspectRepositoryProvider)' -count=1)`: passed.
- `(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/backendapp -run '^Test(PluginRepositorySelection|CreateTaskProjectRemoteProviderAdapters|GitLabRepositoryOriginVerifier|GitHubRepositoryOriginVerifier)' -count=1)`: passed.
- `bash scripts/lint-go-changed` over the thirteen changed and new Go files: 0 issues.
- `go build ./...` in `apps/backend`: passed.
- `(cd apps/web && pnpm exec vitest run app/office/projects/repo-entry.test.ts)`: passed, six tests.
- `(cd apps/web && pnpm exec prettier --check ...)` and `eslint` on both Office specs and the shared helper: passed.
- `(cd apps/web && pnpm e2e:run --project chromium --no-build -- tests/office/new-task-dialog.spec.ts)`: passed, seven tests, including all four new provider-neutral cases.
- `(cd apps/web && pnpm e2e:run --project chromium --no-build -- tests/task/plugin-repository-task-create.spec.ts)`: passed, two tests.
- `(cd apps/web && pnpm e2e:run --project mobile-chrome --no-build -- tests/office/mobile-new-task-dialog.spec.ts tests/task/mobile-plugin-repository-task-create.spec.ts)`: passed, six tests, including all three new mobile cases.
- `node --test scripts/validate-public-docs.test.mjs`: passed, 63 tests. `node scripts/validate-public-docs.mjs`: validated 54 published pages.
- `python3 scripts/list-docs.py validate`: passed, 417 decisions and 1,841 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed. `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- Documentation coverage preflight: `covered`, all three work orders, no errors.
- `git diff --check`: passed.

### Code-review remediation

Three review findings were fixed after the first verification pass:

- Saved repository IDs now short-circuit remote URL and provider admission in `verifyRemoteRepositoryOrigin`, so a valid saved plugin or self-managed GitLab repository still resolves when its provider or connection is unavailable. Foreign-workspace IDs remain refused by `resolveRepoInputID`.
- Dispatch read leases are context-aware (`internal/plugins/dispatch_lock.go`). `InvokeAction` and the other context-bearing dispatch paths use `beginPluginDispatchContext`, so a discovery pass waiting behind a config restart, disable, or upgrade write lease returns on caller cancellation or its own deadline. The write side keeps its exclusion, every started inspection is joined, and a canceled pass with no eligible provider returns the cancellation instead of an empty result.
- Discovery now emits exactly one sanitized completion event for every exit, including invalid requests, deadline, cancellation, and a changed eligible snapshot, while successful and failed candidate inspections keep their own per-inspection entries. The explicit inspector's invalid-request and missing-owner failures log again.

New regressions: `TestCreateTaskSavedRepositoryIDSkipsRemoteProviderAdmission`, `TestRepositoryPreflightRejectsCanceledCandidateSet`, `TestDiscoverRepositoryProvidersLogsEveryExit`, `TestDiscoverRepositoryProvidersHonorsCancellationDuringDispatchLease`, `TestDiscoverRepositoryProvidersRejectsCanceledEmptySnapshot`, and `TestInspectRepositoryProviderLogsRejectedRequests`.

Remediation verification:

- `(cd apps/backend && go test -trimpath -tags fts5 ./internal/plugins -count=1)`: passed, 24.482s.
- `(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/plugins -run 'Test(DiscoverRepositoryProviders|InspectRepositoryProvider|InvokeAgentTool|ManagedDeletion|ExecutorProvider|ServiceRejectsActive|UpdateConfig|Install|Disable|Uninstall)' -count=1)`: passed.
- `(cd apps/backend && go test -trimpath -tags fts5 ./internal/backendapp -run '^Test(PluginRepositorySelection|CreateTaskProjectRemoteProviderAdapters|GitLabRepositoryOriginVerifier|GitHubRepositoryOriginVerifier)' -count=1)`: passed.
- `golangci-lint run ./internal/plugins ./internal/task/service ./internal/backendapp --new-from-rev=012fb3c04069ba7154d9f5afdcddca49a8f3ba1c`: 0 issues. The `lint-go-changed` wrapper was blocked by an unrelated concurrent lint run in the shared environment, so the same scoped invocation ran with `--allow-parallel-runners`.
- Desktop E2E re-run: nine tests across the Office and explicit-plugin suites. Mobile E2E re-run: six tests across the Office and explicit-plugin suites.
- Docs, catalog, specification lint, public docs validation, and `git diff --check`: passed.

### Initial GitLab investigation

The following evidence predates the plugin scope extension.

- Temporary service reproduction: all three expected-success assertions failed for the diagnosed reasons.
- Registration control: passed with the existing verifier stub and expected origin.
- Temporary reproduction cleanup: complete.
- `go test -trimpath -tags fts5 ./internal/task/service -run 'Test(ParseRemoteRepositoryURL|CreateTaskEnterpriseRepository|EnterpriseRepositoryRejects|CreateTaskProjectRepositorySourceFailures|RegisterRemoteRepositorySelfManagedGitLab)' -count=1`: passed.
- `go test -trimpath -tags fts5 ./internal/backendapp -run '^Test(GitLabRepositoryOriginVerifier|GitHubRepositoryOriginVerifier)$' -count=1`: passed.
- `pnpm exec vitest run app/office/projects/repo-entry.test.ts`: passed, six tests.
- `python3 scripts/list-docs.py validate`: passed, 416 decisions and 1,839 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Documentation coverage preflight: `covered`, both work orders, no errors.
- Package Markdown file targets: all exist.
- `git diff --check -- docs/specs docs/plans/office-project-self-managed-gitlab`: passed.
- `git status --short`: three modified specification files and three new package files, all unstaged.
- Issue assignment: confirmed as `carlosflorencio`.
- Implementation, permanent regression tests, and browser runs: not started.

### Provider-neutral design extension

- `(cd apps/backend && go test -trimpath -tags fts5 ./internal/plugins -run '^TestInspectRepositoryProvider' -count=1)`: passed.
- `(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/service ./internal/backendapp -run '^Test(CreateTask.*Plugin|UpdateTaskPreflightsPlugin|PluginRepositorySelectionResolver)' -count=1)`: passed.
- `python3 scripts/list-docs.py validate`: passed, 417 decisions and 1,841 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Documentation coverage preflight: `covered`, all three work orders, no errors.
- Package Markdown file targets and new-file whitespace checks: passed.
- `git diff --check -- docs/specs docs/decisions docs/plans/office-project-self-managed-gitlab`: passed.
- `git status --short --untracked-files=all`: five modified specs and seven new documentation files, all unstaged.
- New discovery, project success, ambiguity, and packaged-fixture E2E tests: planned, not implemented.
- All revised artifacts remain unstaged and uncommitted.

## Risks

- Unknown-host guessing can redirect plugin or Enterprise inputs into GitLab. Use provider-owned verification.
- Local, saved-ID, and plugin inputs do not all have built-in remote URLs. Guard them before origin parsing.
- Verification must update the actual input slice, not a loop-local copy.
- Removing registration's extra verification before shared preflight owns it creates a trust gap. Change them in one work order.
- A first-match return hides ambiguity. Finish bounded discovery before choosing a descriptor.
- Treating an interrupted discovery as a normal no-match can accept an incomplete candidate set.
- Active plugins can be disconnected in one workspace. The plugin must decline unrelated or unconfigured connections safely.
- The desktop dialog fixture and mobile Office fixture use different workspace IDs from the base seed. Configure and clean the correct connection.
- Packaged fixtures prove the host contract, not live vendor authentication or every third-party plugin release.
