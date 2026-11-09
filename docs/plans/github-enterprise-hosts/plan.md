---
created: 2026-10-14
status: complete
requirements:
  - REQ-INTEGRATIONS-GITHUB-ENTERPRISE-001
  - REQ-INTEGRATIONS-GITHUB-ENTERPRISE-002
  - REQ-INTEGRATIONS-GITHUB-ENTERPRISE-003
  - REQ-INTEGRATIONS-GITHUB-ENTERPRISE-004
  - REQ-INTEGRATIONS-GITHUB-ENTERPRISE-005
system_design:
  - ../../specs/integrations/system-design/github-enterprise-hosts.md
legacy_specs: []
---

# Implementation plan: GitHub Enterprise hosts

## Overview

Make the existing Enterprise CLI account selection save and operate successfully,
then carry the same host identity through all GitHub authentication, workflows,
and task execution. The user confirmed GHES and GHE.com, full authentication
parity, and confidential-client OAuth for GHES without PKCE.

The original CLI picker already listed Enterprise accounts. Its Save request
included host/login, but `resolveGHAccountToken` rejected the host, and token
validation/API clients targeted GitHub.com. The implementation preserves the
picker and tests the complete select/save/read path.

Read the [requirements](../../specs/integrations/requirements/github-enterprise-hosts.md),
[design](../../specs/integrations/system-design/github-enterprise-hosts.md), and
[ADR](../../decisions/2026-10-14-github-enterprise-host-boundaries.md) first.
Implementation and native gpt-6.1-sol subagents were explicitly authorized on
2026-10-14. Root owns integration and verification. Delivery acceptance remains
in dependency order; independent implementation slices may proceed as below.

## Scope

### In scope

- GitHub.com, GHES, and GHE.com endpoint routing and exact credential destinations.
- PAT, named CLI, App import/manifest/install, personal OAuth, and webhook lifecycle.
- Host-bound repository/issue/PR workflows, watches, and persisted associations.
- Managed and executor-inherited task Git/CLI behavior across launch and resume.
- Desktop/phone setup, translations, targeted E2E evidence, and public guidance.

### Out of scope

- Multiple automation hosts within one workspace and cross-host forks.
- Enterprise-owned/enterprise-level Apps beyond existing user/organization ownership.
- Arbitrary API overrides, HTTP/TLS bypass, and path-prefix proxy deployments.
- Automatic CA distribution and unverified SSH aliases or separate SSH-port mappings.
- Changes to Kandev's GitHub release infrastructure or unrelated plugins.

## Technical approach

1. **Host-aware transport:** introduce `internal/common/githubhost`; add explicit host
   constructors to `TokenClient`, `AppClient`, OAuth, and manifest conversion.
   Keep public compatibility wrappers, central URL validation, separate GHES
   REST/GraphQL endpoints, redirect restrictions, and TLS/network rules.
2. **Connection lifecycle:** fix CLI token selection and validation; thread host
   through PAT and App setup; persist flow host/OAuth policy; fence connection
   replacement and webhook effects. This work order delivers the reported
   backend Save regression before broader workflow/UI additions.
3. **Provider workflows:** persist original resource hosts, isolate caches and
   watches, validate URL/repository bindings, and remove public-host assumptions
   from issue, PR, fork, and comparison operations. Reconcile legacy schema
   safely and prevent host changes from reinterpreting historical records.
4. **Task execution:** extend the GitHub broker adapter, clone protocol routing,
   host CLI bridge, and agentctl shim. Scope environment variables and explicit
   CLI targets to the redeemed host; preserve renewal and generation fencing.
5. **Setup and documentation:** keep the CLI picker, add PAT/App host input,
   implement stale-draft protection and host status, update links and host-aware
   frontend identities, localize, and prove desktop/phone workflows.

### Compatibility matrix

| Provider/transport | Identity | Intended behavior | Evidence | Unsupported shape |
| --- | --- | --- | --- | --- |
| GitHub.com HTTPS/CLI | Public host + workspace/principal | Existing behavior and defaults | Existing suites plus regression fixtures | Existing permission/auth errors |
| GHES HTTPS/CLI | Explicit web authority; REST `/api/v3`; GraphQL `/api/graphql` | PAT/CLI/App and confidential OAuth; version capabilities | GHES 3.20 fixture profile, TLS routing tests, provider smoke when available | Specific capability/version error; no public fallback |
| GHE.com HTTPS/CLI | Tenant web host and derived API subdomain | Same authentication methods; public-family CLI token variable | Tenant fixtures, OAuth/manifest tests, provider smoke when available | Invalid tenant/API-host input rejected |
| Managed Git | Verified repository origin + task/session/generation lease | HTTPS checkout and renewable scoped credentials | Real Git subprocess + fake CLI + broker tests | Host/path/generation mismatch fails |
| Executor inheritance | Persisted attached provider host | Current per-host protocol and optional local CLI bridge | Local/Worktree launch/resume tests | Remote runtime credentials remain executor-owned |

GHES 3.20 is the initial planned fixture baseline, not a claim of live validation.
Optional capabilities such as merge queues must not make core queries fail.

## ASCII UI preview

Drawings specify grouping, action order, host provenance, and scroll ownership.
Exact copy/spacing is illustrative and must use localized existing UI primitives.
Entry point: workspace Settings > Integrations > GitHub > Change connection.

### UI-01: Existing CLI account selection, successful save

Desktop dialog, CLI method selected:

```text
+-------------------------------------------------------+
| Change GitHub connection                         [X]  |
| Method: ( ) PAT  (*) GitHub CLI  ( ) GitHub App        |
| GitHub CLI account                                    |
| [ alice (git.company.example)                     v ] |
| Host: git.company.example                             |
| Task Git access: [Inherit executor credentials    v ] |
|                                                       |
|                                     [Save changes]    |
+-------------------------------------------------------+
```

The dropdown is existing behavior. Its host is authoritative; no duplicate
host field appears. Save now validates on that host and returns a connected
summary. Error state stays in the form and retains the selected account.
Maps to AC-001.1, AC-005.1, and AC-005.2 of the feature requirements.

### UI-02: PAT/App host selection and phone composition

Desktop dialog, App method selected:

```text
+-------------------------------------------------------+
| Change GitHub connection                         [X]  |
| Method: ( ) PAT  ( ) GitHub CLI  (*) GitHub App        |
| GitHub host [ git.company.example                   ] |
| App [ Select an App on this host                  v ] |
| [Create App]  [Import App]                             |
| ... existing owner, visibility and callback guide ... |
| [Install selected App]                                |
| Task Git access: [Managed                        v ] |
+-------------------------------------------------------+
```

PAT mode uses the same host field followed by its token field and Save action.
App mode keeps the existing creation/import/installation actions and callback
completion semantics; it does not imply that Save alone installs an App.

Phone, full-height inset connection Drawer, PAT example:

```text
+-----------------------------------+
| Change GitHub connection     [X]  |  fixed header
|-----------------------------------|
| Method [Personal access token v]  |
| GitHub host                       |
| [git.company.example          ]   |
| Token                             |
| [*****************************]   |  one scroll body
| Task Git access                   |
| [Managed                      v]  |
|                                   |
| Current host: github.com          |
| New host: git.company.example     |
| Existing work keeps its old host. |
| Choose repository scope next.     |
|-----------------------------------|
| [         Save changes          ] |  fixed safe-area footer
+-----------------------------------+
```

Phone CLI mode replaces host/token fields with the same host-labelled account
picker; App mode exposes its existing create/import/install actions in the
scroll body. Use the existing method selection primitive, not a new selector
solely because of this sketch. Phone targets are >=44px; ordinary desktop
controls remain 28px. The shared draft survives desktop/phone transitions.

Changed states: loading disables dependent App choices; invalid host associates
an error with the host field; failed authentication retains non-secret draft;
TLS failure explains trust setup; stale-host responses are ignored. A successful
host replacement displays an empty selected-repository scope until configured.
The existing connection remains active until a replacement validates and commits.
Maps to AC-001.2–001.4 and AC-005.1–005.4. Rendered evidence belongs to task 05.

## Tests

New test names below are planned, not existing evidence. Use TDD for changed
logic, and extend neighboring suites rather than mirroring helper implementations.

| Acceptance criteria (prefix `AC-INTEGRATIONS-GITHUB-ENTERPRISE-`) | Planned test location and test |
| --- | --- |
| 001.2, 001.3, 001.4, 001.5 | `internal/common/githubhost/host_test.go`: `TestHostNormalization`, `TestEndpointFamilies`; `internal/github/enterprise_transport_test.go`: `TestEnterpriseHostTransport` (TLS, redirects, pagination, endpoint paths, no foreign-server credential receipt) |
| 001.1, 002.1, 001.5 | `internal/github/enterprise_connections_test.go`: `TestEnterpriseCLIConnectionSaveAndRead`, `TestEnterprisePATConnection`, with a fake CLI runner and real service/handler/HTTP client path |
| 002.2, 002.3, 002.4 | `internal/github/enterprise_app_auth_test.go`: `TestEnterpriseAppLifecycle`, `TestEnterpriseOAuthPolicy`, `TestEnterpriseWebhookIsolation` (same IDs across hosts; replay; replacement; refresh and non-expiring tokens) |
| 002.5 | `internal/github/enterprise_capabilities_test.go`: `TestEnterpriseOptionalCapabilities` (basic reads survive absent optional fields; auth errors remain auth errors) |
| 003.1, 003.2, 003.3 | `internal/github/enterprise_workflows_test.go`: `TestEnterpriseResourceIsolation`, `TestEnterpriseHostReplacement`, `TestEnterpriseWorkflowURLs` |
| 003.4 | `internal/github/enterprise_store_test.go`: `TestEnterpriseHostMigration`; extend `store_postgres_test.go` for schema replay and host round trips |
| 004.1, 004.2 | `internal/github/credential_broker_test.go`: `TestEnterpriseBrokerScope`; `cmd/agentctl/github_cli_shim_test.go`: `TestEnterpriseCLIShimRouting` (public/GHE/GHES variables, explicit targets, reissue) |
| 004.3, 004.4 | `internal/repoclone/enterprise_test.go`: `TestEnterpriseManagedCheckout`; `internal/orchestrator/executor/executor_host_gh_bridge_test.go`: `TestEnterpriseHostBridgeLifecycle`; lifecycle executor tests for remote prepared workspace |
| 003.1, 005.1, 005.2 | `components/github/github-connection-settings-form.test.tsx`, `lib/github/parse-url.test.ts`, `lib/utils/github-repo-url.test.ts`, and existing remote-repository/branch/association hook tests: host in payloads/keys, stale lookup rejection, host-aware links |
| 005.3, 005.4 | Desktop/mobile E2E below, i18n checks, and public-document validation |

Every newly changed test suite must be covered by its work order's final command
block. PostgreSQL tests require `KANDEV_TEST_POSTGRES_DSN`; a skipped test is not
PostgreSQL evidence. Use the existing isolated database fixture.

## E2E tests

| File/project | Flows and criteria |
| --- | --- |
| `tests/integrations/github-enterprise-hosts.spec.ts`, chromium | Select an already-listed Enterprise CLI account, save, reload status, browse/import a repository; PAT host errors/recovery; host replacement and stale response; equal repo names across workspaces; App select/create/import/install and personal OAuth return. AC-001.1, 001.2, 003.1–003.3, 005.1–005.2 |
| `tests/integrations/mobile-github-enterprise-hosts.spec.ts`, mobile-chrome | CLI save and PAT/App setup from the phone opener; failed-save correction; keyboard/touch, footer reachability, overflow, draft preservation at responsive boundaries. AC-005.1–005.4 |
| Existing `github-authentication.spec.ts`, `mobile-github-auth-settings.spec.ts`, `settings/github-app-registration.spec.ts`, `settings/mobile-github-app-registration.spec.ts` | Public-host authentication, App callbacks, and layout regressions after shared components change |

Extend the existing mock controller/API fixture to represent multiple hosts and
capture submitted host values. UI mocks do not prove the backend unsupported-host
guard is fixed: task 02's service/handler regression must traverse real validation
with a fake CLI and TLS API server. Never make UI E2E depend on a real corporate
token or external App installation.

## Work orders

- [x] [Task 01: Host-aware clients and transport](task-01-host-transport.md)
- [x] [Task 02: Enterprise connection and App lifecycle](task-02-connection-lifecycle.md)
- [x] [Task 03: Host-bound provider workflows](task-03-provider-workflows.md)
- [x] [Task 04: Enterprise task Git and CLI access](task-04-task-git-access.md)
- [x] [Task 05: Setup surfaces and public guidance](task-05-setup-and-guidance.md)

Acceptance order: 01 -> 02 -> 03 -> 04 -> 05. No independent release is
claimed before all five pass; task 02 provides the reported regression milestone.

### Authorized implementation lanes

- `host_transport` owns new githubhost and direct HTTP transport/client files.
- `connection_lifecycle` owns auth/App services, connection/flow persistence and
  their model fields. It consumes the pinned host constructors; oauth_flow.go
  client edits finish before its manager edits begin.
- `frontend` owns apps/web UI, API types, helpers, catalogs and tests; PAT/CLI
  request `host`, App request `github_host`, and optional resource `expected_host`
  are pinned. The App catalog remains workspace-scoped and is filtered by host.
- `host_transport` continues with resource schema, PR pollers/actions, host routing,
  contribution models and optional GraphQL after transport tests pass. Auth and
  resource column migrations use separate helpers and coordinated shared hooks.
- `connection_lifecycle` takes review/issue service and poller host fences plus
  Enterprise mock fixtures after authentication invariants pass.
- Root owns runtime Git/CLI access, backend clone/broker composition, public docs
  and integration.

These are parallel-safe sub-scopes with explicit file ownership, not parallel
schema work. Subagents do not edit shared plan status or spawn children. The
full test/build acceptance waits for dependent interfaces and fixtures to land.

## Verification results

Implementation verification on 2026-10-14:

- Complete GitHub package with actual PostgreSQL: passed in 142.070 seconds on
  the final cleanup/mock snapshot. Enterprise race suite: passed in 19.565 seconds;
  final cleanup/replacement race regressions also passed in 2.910 seconds.
- Full persistence conformance with actual PostgreSQL and race detection: passed
  in 273.558 seconds. Workflow-sync PostgreSQL/race checks and SQL portability passed.
- Runtime Git/CLI, clone, broker, lifecycle and affected backend composition checks
  passed. Real Git clone/fetch/push/revocation fixtures cover all three host families.
- Frontend: 584 tests in 44 suites, typecheck, ESLint and translations passed.
  Browser checks passed: 12 desktop and 8 mobile cases, including Enterprise App
  installation and personal OAuth. See task 05 for commands and rendered evidence.
- Combined changed-Go lint passed with zero issues; final cleanup/mock changes
  also passed coordinated narrow lint with zero issues. Public documentation: 63 tests and 48 published
  pages validated. Specification catalog (382 decisions, 1605 specifications),
  full specification lint and five-work-order documentation preflight passed.

No live GHES or GHE.com environments were supplied. TLS/provider fixtures and
mock browser flows are not live-provider certification. These results describe
the original implementation snapshot, before PR #4517 was opened.

Design-package checks from repository root:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/decisions docs/plans/github-enterprise-hosts
git status --short -- docs/plans/github-enterprise-hosts
```

Repeat the repository PR-documentation preflight with this read-only command.
The representative production path only activates reference checking; it does
not claim that production code changed during planning.

```bash
node <<'JS'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const dir = 'docs/plans/github-enterprise-hosts';
const paths = fs.readdirSync(dir).filter(n => n.endsWith('.md')).map(n => `${dir}/${n}`);
paths.push('docs/specs/integrations/requirements/github-enterprise-hosts.md');
paths.push('docs/specs/integrations/system-design/github-enterprise-hosts.md');
const fileContents = Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')]));
const changedFiles = paths.map(filename => ({ filename, status: 'added' }));
changedFiles.push({ filename: 'apps/backend/internal/github/gh_accounts.go', status: 'modified' });
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({ ok: result.ok, status: result.status, errors: result.errors }, null, 2));
process.exitCode = result.ok ? 0 : 1;
JS
```

## Integration repair on 2026-10-25

The user authorized preparing PR #4517 against current `main`. The repair merges
base `62af870aea3556e34ae3878041d2b99bf1c522bc` into original PR head
`0e009fd3d695f0dc6bb288635efe7faddfaa1010`.

- Preserve authenticated watcher creators alongside persisted Enterprise hosts.
- Combine host replacement guards with the current workflow-sync scheduler,
  retry handling, and quota coordination.
- Pass the resource host through merge-permission requests and cache identity.
  Reject responses from an earlier host after a context change.
- Classify absolute pagination URLs by API resource, including the GHES
  `/api/v3` prefix, so paginated search obeys search-quota admission.

Regression tests reproduced both the missing merge-permission host and the
pagination admission bypass before their fixes. The full GitHub race run exposed
one new upstream test that constructed a client without a host. That fixture now
uses the public-host constructor; its focused rerun passes.

Verification so far:

- Workflow-sync race suite passed. Focused GitHub regression and PostgreSQL
  suites passed, including watcher creator/host round trips.
- Full SQLite/PostgreSQL persistence conformance with race detection passed
  in 322.322 seconds. SQL portability checks passed.
- Frontend: 62 tests in eight affected suites, type checking, lint, and
  translations passed.
- Documentation: 63 tests and 54 public pages passed validation. The catalog
  validated 404 decisions and 1738 specifications; specification lint passed.
- Enterprise repository selection race tests passed after replacing the SSH
  username literal with a dedicated constant. This avoids coupling the parser
  to an unrelated retirement predicate introduced on `main`.
- Desktop browser checks: five passed. Mobile browser checks: four passed.
  Both runs cover Enterprise CLI/PAT/App setup and merge-permission behavior.
  The production backend/web build passed. The first browser attempt could not
  reach its test backend because it inherited the running instance configuration;
  the successful rerun used isolated configuration and an explicit loopback bind.
- Go lint passed with the CI-pinned golangci-lint 2.9.0. The earlier 2.12.2
  attempt timed out and reported additional constant checks; it is not the
  authoritative lint result for this repair.

Public guidance already covers these contracts; this repair does not add a new
setup step or supported host. Live GHES/GHE.com testing remains unverified.

### CI fixture repairs

The first integration head, `b4d1e800`, exposed three test-fixture defects:

- A legacy PAT rate-tracker test sent its fake token to the live GitHub API.
  The resulting idle HTTP/2 connection failed the package leak check in three
  CI jobs. The test now provides a controlled 401 response and checks that the
  identity probe ran. The full GitHub race suite, including leak detection,
  passes in 270.035 seconds.
- Azure saved-view seeding raced the configuration save. A 500 ms injected
  save delay reproduced the failure. The test now waits for the successful
  configuration response before seeding, and retains the delay as a regression
  stimulus. Three runs pass with retries disabled.
- Mobile statistics recovered automatically while the test was measuring the
  Retry button. Advancing the browser clock reproduced the missing-button
  timeout. The fixture now keeps the outage active through automatic retries
  until the test starts the manual retry. Three runs pass with retries disabled.

The analytics aggregate deadline also failed once in CI. Its exact test passed
three local race-enabled runs without changes. The walkthrough generator hit
its external ten-minute deadline. Both require confirmation in the next CI run.
All 118 browser reports were audited: only the Azure and mobile statistics
tests had failed attempts; both passed on their CI retry.

Focused backend rerun commands from `apps/backend` (PostgreSQL checks use
`KANDEV_TEST_POSTGRES_DSN` pointing at the disposable test database):

```bash
GOMAXPROCS=2 go test -race -trimpath -p 1 ./internal/github ./internal/workflowsync -run 'TestCoordinateLegacyClientGatesIdentityProbe|TestEnterprisePaginationUsesSearchAdmission|TestExecutorOwnership|Test.*Postgres' -count=1
GOMAXPROCS=2 go test -race -trimpath -p 1 ./internal/task/service -run 'Test.*Enterprise.*Repository' -count=1
GOMAXPROCS=2 go test -race -trimpath -p 1 ./internal/persistence/storeconformance -count=1
GOMAXPROCS=2 go run -trimpath ./cmd/sqlguard ./internal
```

Frontend regression command from `apps/web`:

```bash
pnpm exec vitest run components/github/github-connection-dialog.test.tsx components/github/github-connection-draft.test.tsx components/github/github-app-onboarding-model.test.ts components/github/github-status.test.tsx components/github/pr-merge-button.test.tsx hooks/domains/github/use-repo-merge-access.test.ts hooks/domains/github/use-repo-merge-methods.test.ts lib/api/domains/github-pr-api.test.ts
```

Managed browser commands from the repository root:

```bash
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --project chromium e2e/tests/integrations/github-enterprise-hosts.spec.ts e2e/tests/pr/pr-merge-permissions.spec.ts
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome e2e/tests/integrations/mobile-github-enterprise-hosts.spec.ts e2e/tests/pr/mobile-pr-merge-permissions.spec.ts
```

### Adversarial review follow-up (2026-10-26)

A secondary GPT-6.1-Sol Max session reviewed the complete PR at `680f19cb`.
It confirmed four defects with independent temporary probes. The primary
session added permanent regressions and repaired each boundary:

- Credential resolution now compares the request's captured host with the
  returned credential before a provider operation can start.
- Pasted issue and PR URLs retain their parsed host through credential
  selection. PR watch checks, branch detection, status refresh, batch reads,
  and missing-repository cache checks also retain the original resource host.
  The follow-up audit reproduced the watch and batch defects separately.
- Managed CLI validation checks relative API endpoints, including commands
  with flags before `api`. Repository selection follows GitHub CLI's final
  `--repo` value. Rejected targets fail before lease redemption or child launch.
- Connection method controls are disabled during saving, like the other draft
  fields. Dismissal and reopening still discard stale results. Shared desktop
  and phone browser scenarios cover deferred failure, restored controls, and
  successful retry without changing either overlay's composition.

The reviewer also checked the repair diff. Its remaining flag-order finding
was reproduced and fixed in the primary session. No source changes came from
the reviewer. These repairs enforce the existing requirements; public setup
instructions and supported host families are unchanged.

Validation: all 13 connection-dialog tests pass. Frontend type checking,
affected ESLint checks, translations, and the document catalog/specification
checks pass. The full GitHub race suite passed in 402.255 seconds; the final
host/watch/batch/task-link race run passed in 133.061 seconds. The complete
agentctl race suite passed in 6.780 seconds after the flag-order fix. The rebuilt
app passed four desktop and four phone Enterprise browser scenarios with
retries disabled. Live GHES/GHE.com testing remains unverified.

## Risks

- GHES version/policy differences can block App manifests or optional GraphQL
  fields. Keep failures explicit; baseline fixtures are not live certification.
- Host replacement touches saved watches/settings and asynchronous callbacks.
  Migrations and generation guards must land before full workflow support.
- Private GHES needs network and certificate trust from backend and executor;
  this work does not distribute trust roots or create network routes.
- The existing large GitHub package has URL assumptions beyond the initial
  inventory. Each work order must search its owned paths and classify remaining
  public-host literals rather than replace unrelated constants indiscriminately.
- Live GHES and GHE.com test environments have not been supplied. Record any
  real smoke evidence separately, including version and auth method; do not
  represent fixture-only testing as live-provider validation.


### Greptile follow-up: retained watches, draft ownership, and shared package

- Workspace batches skip retained watches from other hosts. Fallback groups include
  the host so an old watch cannot consume the current watch's result or prevent its update.
  Host-replacement and mixed-workspace guards remain active.
- Connection dialogs retain the selected method during status refreshes. Each opening
  initializes the method from current status. Save completion and failure recovery use
  the existing draft guard, including dismissal/reopen protection.
- The pure host package and its tests now live under `internal/common/githubhost`.
  Backend, agentctl, clone, executor, task, and workflow-sync imports use that shared path.
  No compatibility copy remains at the old location.

The mixed-host regression covers batch, poller fallback, and passive fallback through
persisted task PRs. All three cases fail on the previous production source and pass
with the repairs. Four deferred-save cases reproduce status refresh during successful
and failed saves on both desktop and phone. The full dialog suite passes (17 tests),
as do frontend type checking and affected ESLint. Full race checks passed for
`internal/common/githubhost` (1.012s), `internal/github` (314.109s), and
`cmd/agentctl` (8.523s). Both backend and agentctl builds pass. Documentation catalog
and specification checks pass. Final CI and thread disposition are recorded on the PR.

This UI change only changes draft state ownership. It does not change layout, touch
handling, navigation, scrolling, or breakpoint behavior. The existing desktop dialog
and phone drawer share the fix. Component coverage exercises both compositions;
new mobile E2E coverage is not required for this state-only correction. Public setup
instructions remain accurate. Live GHES/GHE.com smoke testing remains unverified.


Follow-up commands:

```bash
# apps/backend
GOMAXPROCS=2 GOMEMLIMIT=3GiB go test -race -p 1 ./internal/common/githubhost ./internal/github ./cmd/agentctl
GOMAXPROCS=2 GOMEMLIMIT=3GiB go build -p 1 ./cmd/kandev ./cmd/agentctl
# apps/web
pnpm exec vitest run components/github/github-connection-dialog.test.tsx
pnpm run typecheck
pnpm exec eslint components/github/github-connection-dialog.tsx components/github/github-connection-dialog.test.tsx
# repository root
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```


### Follow-up CI fixture correction

The sidebar color-rule reload case observed its optimistic color before the last
settings PATCH reached the server. The CI trace records that PATCH with status -1
at navigation, and the reloaded task retained the previous red rule. Delaying the
last PATCH by 500 ms reproduced the same red-versus-blue failure locally. Moving
the existing persisted-settings assertion before navigation made all three delayed
runs pass with retries disabled. The temporary latency probe was removed. The
permanent change only reorders the existing assertion; product behavior is unchanged.
The final test passed three repetitions without retries (43.9s), using
`pnpm --dir apps/web e2e:run --host --no-build --project chromium e2e/tests/task/sidebar-automatic-colors.spec.ts --grep "reorders overlapping rules through More" --repeat-each=3 --retries=0`
after rebuilding the application for the prior reproduction.

The mobile plugin diagnostic case also failed once with `Route is already handled`
in its response mock. Its CI retry and three exact local repetitions passed. No
plugin source or test change was made. The next head must confirm both browser
checks through CI before this follow-up is complete.


### Browser CI synchronization repairs

The next CI run exposed four test synchronization gaps. The mobile Quick Chat
case checked a transient disabled button after cancellation had already settled.
It now uses the existing cancellation observation helper to hold settlement until
the pending-state assertions finish, with release in `finally`. The Settings
recovery case now observes the second failed chunk request before checking the
error screen. Kotlin completion sends the shortcut to Monaco's actual input,
instead of relying on page focus after a click. Attachment reloads use the existing
chat-panel readiness helper before checking persisted previews.

Controlled probes reproduced the original Settings and Kotlin failures by delaying
the second chunk failure and removing editor focus. Both passed with the fixes.
Delaying the desktop panel chunk reproduced the attachment failure; waiting for
session HTTP data alone did not fix it, while chat-panel readiness did. All
latency and focus probes were removed. These are test-only corrections with no
product, translation, public documentation, or screenshot changes.

Baseline resource-bounded runs passed 24 desktop cases and two mobile cancellation
cases with retries disabled, CPU quota 200%, and memory limit 4 GiB. The final eight
changed desktop/mobile cases passed (1.8m), and the shared attachment helper passed
its phone case (27.2s) under the same limits. ESLint passed with zero warnings.
Normal commit hooks and exact-head CI remain delivery gates.
