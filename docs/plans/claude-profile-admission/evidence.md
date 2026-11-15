# Claude profile admission: planning evidence

## Scope and source baseline

Investigation date: 2026-11-08. All repository work stayed in this task's existing Worktree workspace.
Local HEAD and repeated direct remote reads both returned `9aba7b2edd020f6ba0f92c2bfe318429723ea2ed`.
The parent's earlier `02b1267f` revision and the reports' `813a741b` revision were treated as historical evidence only.

```bash
git log -1 --format='%H %s'
git ls-remote origin refs/heads/main
gh api user --jq .login
gh issue view 5180 --repo kdlbs/kandev --json number,title,url,state,assignees,updatedAt,body,comments
gh issue view 5183 --repo kdlbs/kandev --json number,title,url,state,assignees,updatedAt,body,comments
```

Viewer: `carlosflorencio`. Both reports were OPEN and unassigned before assignment.
#5180 had no comments. #5183 had one comment confirming the explicit full-ID workaround.
Neither body nor its comments contained an attachment to fetch.
No live deployment, real credential, credential store, journal, or shared worktree was used for reproduction.

## Canonical reports

- [#5180](https://github.com/kdlbs/kandev/issues/5180): Stock 0.99.1 successfully ran a secret-backed Claude API-key profile on SSH.
  Guarded workflow preview rejected a destination Claude profile with `workflow_agent_incompatible` despite its authorized environment binding.
  This is the reporter's deployment evidence, not a deployment reproduced here.
- [#5183](https://github.com/kdlbs/kandev/issues/5183): Exact `sonnet` was absent from the advertised catalog while `claude-sonnet-5-5` was present.
  Startup refused before the prompt.
- [Verified workaround comment](https://github.com/kdlbs/kandev/issues/5183#issuecomment-6720279723): Selecting the full advertised ID allowed the same SSH automation to start.
  Its guarded command later completed. The comment did not enable model/provider fallback.

## GitHub overlap

Canonical issue timelines were fetched with `gh api --paginate repos/kdlbs/kandev/issues/<number>/timeline`.
They contained no fix-PR cross-reference at the investigation snapshot.
All 146 open PR titles and bodies were fetched with `gh pr list --state open --limit 250 --json ...`.
No open PR explicitly referenced #5180 or #5183.
Semantic matches and their changed-file lists were inspected.
The 100 most recent merged PRs were also searched, alongside local history for the affected files.
This is a bounded recent-merge check, not a claim to have inspected all historical PRs.

| PR | Current relevant head | Disposition |
| --- | --- | --- |
| [#5184](https://github.com/kdlbs/kandev/pull/5184) | `29989fc62d8f55d18ab254a80433960b9d4ced1d` | Draft/open. Shared profile owner grants and prepared actors. Not a dependency. |
| [#5176](https://github.com/kdlbs/kandev/pull/5176) | `1249b5d8d565b8e4836f06b95b3f9cc55458a1d2` | Draft/open. Cold-resume executor admission. Separate gap. |
| [#5179](https://github.com/kdlbs/kandev/pull/5179) | `92c8f001a15f4ce2330998da625f94126e4be6e9` | Draft/open. Retained executor bindings and content edits. Separate gap. |
| [#4995](https://github.com/kdlbs/kandev/pull/4995) | `405423974a95380bf36809f460455150c64c000e` | OpenCode credential bootstrap. Shared catalog edit overlap only. |
| [#4764](https://github.com/kdlbs/kandev/pull/4764) | `2789d904894cef436ba23a5bf9b6b1ce470c108a` | Exact assigned task profiles. Separate from exact model identity. |
| [#5152](https://github.com/kdlbs/kandev/pull/5152) | `a56f1f9419987c1b88c512b1f1e0b973aeb66d31` | Dynamic routing. No dependency for concrete Claude auth. |

#5184's API response reported base `52309e613fc9a76d38a9f891b74b4d8b32bff265` and `merged=false`.
Its body and changed files were read, including its owner-grant, secret resolver, and prepared-actor boundaries.
No branch from it was checked out, cherry-picked, or imported.

Recent relevant merged work includes #5037 (profile-aware discovery) and #5055 (grouped catalog presentation).
Their behavior is present on this baseline.
Local affected-file history also includes typed startup transport handling in #4626.
Dependency/main heads must be refreshed before implementation.

## Read-only source trace

### Authentication

1. `task/service/workflow_change.go:normalizeWorkflowChangeAgentOverrides` resolves the task-selected executor and validates defaults and replacements.
2. `backendapp/task_agent_executor_compatibility.go:ValidateAgentProfileForExecutor` receives the profile but omits it from `validateRemoteAgentCredentials`.
3. That checker builds a remote-auth catalog and tests only executor `remote_credentials` and `remote_auth_secrets`.
   A nonempty secret ID passes without a store read.
4. `agents/claude_acp.go:RemoteAuth` declares only the OAuth env method.
5. `lifecycle/environment_resolution.go:resolveStrictEnvironment` resolves profile definitions through `environment.Resolve`.
   `resolveEnvironmentDefinition` uses scoped Global secret reveal for profile entries.
6. `lifecycle/executor_ssh_credentials.go:resolveAuthSecrets` maps method IDs to their declared env variables.
   `runAuthSetupScripts` uses the method's own env var and script.
7. `executor_ssh_operations.go:sshRemoteAgentEnv` forwards `ANTHROPIC_API_KEY` from the resolved request env.
   It does not fall back to the control-plane process environment.

Settings already validate Global profile references through `secrets.ValidateGlobalReference`.
The existing `UserVisibleStore` contains internal IDs and non-Global access.
The raw store's missing-identity path is unscoped, which makes actor preparation essential before the new admission reads.
Existing `executorCaller` checks durable/current actor status. `AuthorizeExecutorConfiguration` checks parent/profile ownership.
The plan reuses those boundaries rather than borrowing a profile owner's identity.

### Model identity

`start_model.go:applyStartModelPolicy` requires exact advertised membership for `RequireExactModel`.
An unadvertised ID returns `model_unavailable/requested_not_advertised` before `SetModel`.
`CurrentModelID` becomes diagnostic evidence, not permission to substitute it.
An exact advertised ID is applied through `SetModel`.

Active no-silent-fallback AC-001.1 and .10 require this refusal.
AC-001.8 shares the policy across fresh session boundaries. AC-002.1 forbids strict variation inference.
Profile discovery is host-scoped and advisory.
Host-CLI AC-002.3 permits exact custom-ID saving even when a host list omits the ID.
Those contracts make an unconditional host-catalog save gate an incompatible repair.

The editor's `ModelPicker` retains custom Claude IDs, including unadvertised saved selections.
The recovery model already has typed requested/effective-model details and a generic unavailable-model explanation.
No existing Kandev catalog type inspected here supplied a runtime-bound alias-equivalence map.

### Pinned upstream bridge inspection

Main pins `@agentclientprotocol/claude-agent-acp` to `0.100.0` in `managed_npm_runtime_versions.json`.
Public npm metadata reports git head `981bf9a28ac3afef26e7877a2a638814c34f1679` and Claude Agent SDK dependency `0.3.316`.
The package tarball was downloaded into disposable scratch space and its published SHA-512 integrity was verified.
No package code or installer was executed.
The GitHub commit was independently resolved through `gh api`.

Primary sources:

- [Pinned bridge source](https://github.com/agentclientprotocol/claude-agent-acp/tree/981bf9a28ac3afef26e7877a2a638814c34f1679)
- [Model preference implementation](https://github.com/agentclientprotocol/claude-agent-acp/blob/981bf9a28ac3afef26e7877a2a638814c34f1679/src/session-model.ts)

The published `dist/session-model.js:resolveModelPreference` first tries direct/resolved matches, then substring and token-scoring matches.
`getAvailableModels` consumes `ANTHROPIC_MODEL` and returns `model.value` as `modelId`, plus a current model.
It does not return a qualified alias-equivalence mapping in that models response.
Allowlist overrides and resumed-model handling can change which ID is surfaced.

Inference: reproducing those heuristics in Kandev would not establish strict alias identity.
This inspection strengthens the conservative refusal. It does not prove which bridge/account configuration ran in the reported deployment.
Task 04 remains a policy/repair-surface prerequisite, not a guessed alias patch.

## Diagnostic results

Scratch path used during investigation: `/tmp/claude-admission-evidence-ejhxmino`.
Go overlays mapped temporary test sources onto package test paths without changing repository files.
All SQLite/master-key material lived under test-owned temporary directories and used synthetic values.

### Auth probe: expected red result

The first scratch compile referenced a nonexistent `ClaudeACP.SetEnabled` helper and provided no behavioral evidence.
That scratch-only mistake was corrected before collecting these results.

```bash
cd apps/backend
go test -trimpath -overlay=/tmp/claude-admission-evidence-ejhxmino/overlay.json ./internal/backendapp -run '^TestPlanningProbe' -count=1 -v
```

Exit 1 with two intended regression failures:

- `TestPlanningProbeClaudeAPIKeyAdmission`: Created a caller-owned Global key in temporary SQLite and successfully revealed it under that caller.
  The real compatibility validator still returned `no credentials are configured for agent family "claude-acp"` for the profile binding and empty executor config.
- `TestPlanningProbeInvalidDeclaredSecretAdmission`: A nonexistent synthetic OAuth secret ID was accepted by `validateRemoteAgentCredentials`.
  The probe expected rejection and failed.

This establishes the validator defect at unit/integration level.
It does not prove HTTP preview/apply or real SSH provider authentication.

### Model and SSH controls: pass

```bash
cd apps/backend
go test -trimpath -overlay=/tmp/claude-admission-evidence-ejhxmino/overlay.json ./internal/agent/runtime/lifecycle -run '^(TestPlanningProbeClaudeExactAliasAndFullID|TestApplyStartModelPolicyExecutorAuthority|TestStrictStartModelFailureCarriesBootstrapCause|TestStartModelCancellationAndDeadlineKeepTheirMeaning|TestStartModelTransportFailureIsNotReportedAsModelApplicationFailure|TestStartModelProviderRejectionStaysModelApplicationFailure|TestSSHRemoteAgentEnv|TestRemoteExecutorsResolveExplicitGitHubTokenSecret)' -count=1 -v
```

Exit 0. Eleven top-level tests ran, including subtests, in the lifecycle package (reported package time `0.028s`).
The scratch model probe used a catalog containing only `claude-sonnet-5-5`, also set as the current model.
Strict `sonnet` failed with `prompt_not_sent=true` and zero `SetModel` calls.
Strict full-ID selection called `SetModel` once with that exact ID and returned it as effective.
Existing controls covered typed transport, cancellation, provider rejection, executor authority, and request-only SSH credential forwarding.
No real ACP process, reset/resume deployment, or remote shell was exercised by this run.

### Existing baseline controls: pass

```bash
(cd apps/backend && go test -trimpath ./internal/backendapp -run '^TestTaskAgentExecutorCompatibility' -count=1)
(cd apps/backend && go test -trimpath ./internal/agent/remoteauth -run '^TestBuildCatalog' -count=1)
```

Both exited 0 (`0.041s` and `0.015s`). Four existing compatibility tests and two catalog tests matched.
They do not cover the reported profile API-key case, as shown by the scratch regression.
These diagnostic runs did not use `-race`. Work orders require targeted race tests after implementation.

## Requirements and related package reconciliation

Ownership discovery used `python3 scripts/list-docs.py specs` for Agents, Executors, Tasks, Workspaces, and Auth.
Relevant ADRs were found through `python3 scripts/list-docs.py decisions --format paths`.
The root/scoped engineering guides, specification guide, requirement/design templates, and planning workflows were read.

Existing packages were inventoried without changing their delivery status:

- `change-workflow`: plan/work orders done. Reuse its atomic transition and guard ACs.
- `exact-profile-model-identity`: implemented. Reuse active strictness ACs and existing controls.
- `profile-capability-discovery`: implemented. Reuse profile-context discovery.
- `host-cli-model-discovery`: implemented. Preserve custom-ID saving and host-advisory scope.
- `first-draft-profile-discovery`: in progress, with Devin Fusion qualification still pending.
  That unrelated qualification does not block ordinary Claude full-ID choices or this auth fix.

Only the missing authentication-admission contract gets a new draft requirement/design pair.
The host-CLI design's older alias language is reconciled to active strictness through a documentation correction.
No model strictness AC is weakened, and no duplicate workflow or repair requirement is created.

## Issue assignment

A fresh check before assignment still found both issues open/unassigned and 146 open PRs with no explicit reference to either report.
Only #5180 met the requesting user's certainty rule for a corrective package.

```bash
gh issue edit 5180 --repo kdlbs/kandev --add-assignee carlosflorencio
gh issue view 5180 --repo kdlbs/kandev --json number,state,assignees,updatedAt
gh issue view 5183 --repo kdlbs/kandev --json number,state,assignees,updatedAt
```

Verified at `2026-11-08T21:09:30Z`: #5180 remains OPEN and is assigned to `carlosflorencio`.
#5183 remains OPEN and unassigned. Its corrective model policy/UX outcome is not yet established.
No issue/PR comment was posted, and no issue was closed.

## Document validation

These checks passed on the package files:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
python3 scripts/lint-spec-files.test.py
git diff --check -- docs/specs docs/decisions docs/plans/claude-profile-admission
git status --short -- docs/plans/claude-profile-admission
```

Catalog result: `Validated 440 decisions and 1948 specifications.`
Specification lint result: `All specification files passed.`
Linter self-tests: 36 passed. Relative Markdown file targets and trailing whitespace passed for all 11 changed/new package files.
`git status` confirmed the untracked plan/work orders, alongside the two modified existing internal documents.

The repository PR-documentation preflight was called through its exported `validateCoverage` function in `.github/scripts/pr-docs.cjs`.
Inputs included actual changed paths and the full referenced plan, work orders, requirement documents, and designs.
The actual docs-only diff returned `ok=true, status=exempt`.
A second local call added the planned backend validator path as a prospective trigger and returned `ok=true, status=covered`.
All four work orders passed with zero reference errors. This was a local reference audit, not a GitHub PR check or a production diff.

No pnpm install, frontend build, browser suite, public-doc publication, or broad backend audit was needed for this internal planning change.

## Remaining evidence limits

There is no live deployment, real Anthropic credential, HTTP workflow-change reproduction, browser run, or SSH end-to-end reproduction in this turn.
The planned integration/E2E regressions have not been created.
No claim of real provider validity, universal remote-executor parity, or alias equivalence follows from the probes.
Production code, permanent test code, generated contracts, and lockfiles remain unchanged.
