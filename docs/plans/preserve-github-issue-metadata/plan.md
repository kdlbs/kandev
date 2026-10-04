---
created: 2026-10-04
status: in_progress
requirements:
  - REQ-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001
system_design:
  - ../../specs/tasks/system-design/link-existing-task-github-issue.md
legacy_specs: []
---

# Implementation plan: Preserve metadata during GitHub issue linking

## Overview and checkpoint

Repair existing issue-only preservation through one canonical typed set/remove
operation, from real GitHub service through the actual backendapp adapter and
task service into serialized current database metadata. One sequential work order
owns permanent existing-API RED, the coherent implementation and scoped protocol,
storage, compatibility and delivery evidence.

ROOT reviewed the ended DESIGN package and explicitly released implementation
to this same primary on 2026-10-04. Task 01 is in progress. Only ROOT's later explicit reviewed implementation message via
interrupt to this same primary session releases production/permanent test edits,
install, DB, build, commit or PR work. No agents/tasks/tabs/model switch.

Tasks owns the existing
[external-link requirement](../../specs/tasks/requirements/link-existing-task-github-issue.md).
Criteria .2 to .5 make its existing preservation/coherence/failure scenarios
explicit; criterion .1 and unrelated provider/PR/watch outcomes remain active.
The new [focused design](../../specs/tasks/system-design/link-existing-task-github-issue.md)
references the shared
[field/merge contract](../../specs/tasks/system-design/task-field-updates.md).
Do not reopen or rewrite historical external-link, issue-indicator, ordinary-field
or metadata-merge delivery results.

## Confirmed cause and accepted evidence

Initial actual HEAD and recorded main are both
`ca1492fafa5bd86bdc02a5fff476b4428c558de8`, with a clean initial worktree. No
intervening causal change beyond ROOT's compatibility receipt. Inventory of
`5a51992f17325ee42fecb615a2f1c328871e9844..ca1492f` is unrelated runtime recovery;
the relevant adapter block and entire Task model remain statically compatible.
The rest of `adapters.go`/`models.go` changed for that unrelated work.

GitHub copies early task metadata before a delayed external fetch or unlink
write. `githubTaskIssueStoreAdapter.UpdateTaskMetadata` intentionally invokes
ordinary `Service.UpdateTask` metadata replacement. Serializing a stale snapshot
does not preserve independently accepted unrelated keys. Simply routing to the
ordinary merge would fail to delete unlink keys.

Read-only accepted evidence, never replay/alter/remove:

- `/tmp/kandev-github-issue-metadata-repro_test.go`, SHA256
  `624dcb57f7b3ee87b015da0882f5d581cd8f616f110059a7f65303d472b1de23`.
- `/tmp/kandev-root27-issue-metadata-proof-receipt.json`: actual handle 57722
  joined exit 1; `TestRootGitHubIssueMetadataProof` .49s/package .600s at
  `5a51992f17325ee42fecb615a2f1c328871e9844`. Both delayed link/unlink revert
  accepted `alpha` and `port_forwarding_enabled`; both sequential controls PASS.
  Actual production GitHub service/adapter/task service/SQLite; construction-time
  forwarding GetTask gate and only external transport mocked. All workers/cleanup
  joined and temporary repository diagnostic removed. Current-base compatibility
  is static, not a replay or fresh RED verdict.

Permanent meaningful RED after release is a separate existing-API test. Record
actual immutable PR base before delivery; do not rebase onto moving main.

## Scope and technical approach

Add internal `models.TaskGitHubIssueLink`, required repository and service
`UpdateTaskGitHubIssue`, and change only GitHub's issue-write interface/adapter
callers. Nonnil typed identity sets all five keys; nil explicitly removes all
five. Both operations use current raw metadata under native writer/row locks and
write only metadata/timestamp. Return the transaction's committed candidate for
the existing ordinary postcommit fallback, reload/list task repositories, and
normal `task.updated`. The design inventories all consumers and writer families
before choosing this seam. No schema, wire, auth, routing or frontend changes.

| Compatibility boundary | Preserved behavior and evidence |
| --- | --- |
| GitHub legacy/internal client and personal-read REST path | Same fetched identity, task/workspace/provider/repository validation, WithoutCancel write boundary; real adapter/registered-route tests. |
| Legacy issue-watch metadata | Canonical URL lookup still accepts slug repo shape; explicit unlink removes five keys and retains watch/author/profile data. |
| Ordinary merge/scalar/title/server-owned writers | Current-row serialization; preserve omitted raw values and scalar/effect state in both orders, independent connections. |
| Same issue domain | One complete link or no link by commit order; never torn keys. |
| Ordinary replacement/full snapshot | Existing directional exclusions, pending value dialects and owner protections; explicit compatibility controls. |
| Other providers/PR watches/associations | Existing reads/validation and unsupported provider behavior; compile existing adapters and selected PR repository controls. No issue-watch intake rewrite. |

Out of scope: revision or global event order, public schema/API addition, generic
set/remove framework, runner/Office/general lifecycle redesign, new routing/auth,
watch reservation/deduplication, UI/layout/copy/browser/E2E/build/broad suites.

## Tests and acceptance mapping

All new names below are planned, not executed. Companion files keep existing
oversized source/test files within lint limits. Full matrix/commands belong to
[Task 01](task-01-atomic-issue-mutation.md).

| Criteria | Required evidence |
| --- | --- |
| `.1`, `.2` | `TestGitHubIssueMutationConcurrentSQLite`: delayed link/unlink vs accepted merge in both orders, sequential controls, two real services/handles/actual adapter, independent task/workspace isolation. |
| `.2`, `.4` | `TestGitHubIssueMutationWriterCompatibility`: ordinary omission/scalar/title owner/human-title interactions in both orders, sanctioned server-owned/deferred/handoff/workspace writes and removal. |
| `.3` | `TestGitHubIssueMutationCoherence`: relink all five, link/link and link/unlink ordered competing intents, legacy-watch unlink without collateral changes. |
| `.4`, `.5` | `TestTaskGitHubIssueSQLiteValues`, `TestTaskGitHubIssueSQLiteAtomicity`, `TestTaskGitHubIssueSQLiteCancellation`: real raw metadata/number/null/nested/pending values; rejected native storage/decode rollback, actual busy cancellation and typed errors. |
| `.5` | `TestGitHubIssueMutationRegisteredRoutes`, `TestGitHubIssueMutationPostcommitObservation`, `TestGitHubIssueMutationFailures`: actual routes/DB/DTO/task.updated, existing fallback/later observation, access/provider/repo/fetch/WithoutCancel errors and no failed success evidence. |
| `.2` to `.5`, PG | `TestTaskGitHubIssuePostgresPhysicalWait`, `TestTaskGitHubIssuePostgresValues`: independent physical row blockers, lock/wait/PID evidence, committed current metadata/title/owner value, cancellation/rollback and all-five coherence. |

Prefix for criteria: `AC-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001`. Do not use
helper call counts, mocked business logic, transport-only assertions, elapsed
time, an unjoined handle or PG SKIP as executed proof. Registered protocol tests
are the causal end-to-end boundary; no browser work is planned.

## Public documentation and mobile audit

Skills `docs-maintainer` and `mobile-parity` assessed: state/data-only correction
with identical desktop/phone DTO/event consumption and controls. No visual,
touch/scroll/navigation/copy or viewport-dependent change. Public reference
audit: integrations, tasks/workflows, WebSocket/API reference, root README and
screenshots. After implementation add only a short accurate issue PUT/DELETE
preservation clarification beside existing partial-update semantics in
`docs/public/websocket-api.md`; retain ordinary replacement/snapshot exclusions.
No new page/screenshots or browser/build/E2E absent causal ROOT amendment.
The existing owning requirement and design preserve enough rationale; no ADR.

## Work orders

- [ ] [Task 01: Commit issue-only mutations atomically](task-01-atomic-issue-mutation.md)

One work order, sequential in this same primary; no dependencies beyond reviewed
main's canonical ordinary field/merge contract. No delegated execution.

## Verification results

DESIGN gates passed on 2026-10-04:

- `python3 scripts/list-docs.py validate`: exit 0, 348 decisions and 1337 specifications.
- `python3 scripts/lint-spec-files.test.py`: exit 0, 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: exit 0, all specification files passed.
- Documentation gate handle 86475 actually joined exit 0 before next mutation.
- Local document-link check: exit 0, 17 links across all four artifacts.
- `.github/scripts/pr-docs.cjs` exported `validateCoverage`: exit 0, covered,
  no errors, using the four actual unstaged documents and an explicitly planned
  runtime trigger. This is design-reference preflight, not implementation or CI proof.
  The login-zsh attempt could not find Node and is no verdict; actual preflight
  used existing Node PATH through bash loginfalse, with no install/setup change.
- `git diff --check`: exit 0. Status: one modified owning requirement and three
  new documents in the focused design/plan paths, all unstaged/uncommitted.

ROOT reviewed the full package and released implementation in this same primary
session on 2026-10-04. Implementation and targeted acceptance are verified:

- Separate permanent existing-API RED: delayed link and unlink lost accepted
  metadata before correction; joined exit 1. Both sequential controls passed.
- Corrected real GitHub/backendapp/task-service/SQLite path: joined exit 0.
  Controlled merge and registered REST preference overlaps in both orders,
  distinct complete relinks, unlink, independent tasks/workspaces, protected
  writers/title CAS and ordinary replacement/snapshot exclusions passed.
- Real registered routes used actual workspace PAT connection, credential
  resolver and encrypted secret store. Only remote GitHub HTTP responses were
  supplied. DB/DTO/task.updated, auth, provider/fetch/repository/storage failure,
  cancellation/WithoutCancel and postcommit current/candidate/list fallback passed.
- SQLite raw value/NULL/pending owner, native busy cancellation, rejecting trigger,
  candidate-read rollback and populated session/transition/entry/runner controls
  passed. Existing selected field/title/merge/service/GitHub/registered preference
  regressions passed. Executor adapter package compiled; no-tests is compile only.
- Actual isolated PostgreSQL 16: joined exit 0, 26.004s package, plus affected
  coherence/isolation/value extensions 9.653s. Independent physical row blocker
  and advisory waits were observed for issue/merge/link/unlink commit orders;
  current raw large-number/null/title-owner values and cancellation/rollback
  passed. An independent connection mutated sibling and other-workspace tasks
  while the subject was locked. No skipped PG test is claimed as execution.
- SQLguard and real SQLite/PostgreSQL storeconformance passed; latter package
  101.453s. Every command/client handle joined. Only the proven owned tmpfs PG
  container and generated credential file were removed, after zero client
  backends were observed. Foreign resources and accepted ROOT archives retained.
- Public reference clarification is limited to issue-only preservation beside
  existing partial-update semantics. State/data-only mobile exception applies;
  no browser, E2E, build or visual change.

- Original installed golangci-lint 2.9.0 full `./...` changed-revision gate at
  immutable `ca1492fafa5bd86bdc02a5fff476b4428c558de8` passed with zero issues,
  joined exit 0, 6.076s, CPU 2/1GiB/CLI10m/GNU11m/allow-serial bounds. The first
  run joined exit 1 after 576.514s on one test-only ineffassign; the minimal
  binding correction compiled, with assertions and executed PG behavior unchanged.
  Both original receipts were retained; no resource retry or cache change.
- Documentation gates passed: 348 decisions/1337 specifications, 36 linter tests,
  all specs, 47 public pages/62 validator tests, actual 34-path coverage with one
  work order and no errors, 18 local artifact links, and diff whitespace check.

Active hooks,
ready PR/current-head full review/all-terminal statuses and actual verified normal
merge remain delivery gates. Work order remains in_progress until delivery.
Exact commands, PID/duration/exit receipts and resource ownership are retained in
the external task plan; `/tmp` receipt paths are local evidence, not public APIs.

## Risks

- Returning a stale fallback would break observation compatibility. The new
  repository must return its locked current candidate and retain ordinary reload
  fallback, unlike the explicit merge's different postcommit failure contract.
- Pending-title generic patch cannot express deletion safely; use direct current
  raw document update and preserve unrelated null/number/owner records.
- PG advisory locking alone misses other physical row writers; observe actual
  row-only wait and current result, with no later workspace/step lock inversion.
- Full replacements/snapshots still have permitted later overwrite behavior.
- Resource failure or unrelated hosted failure requires exact durable evidence,
  WAITING and ROOT's bounded direction, not retry/weakening or scope growth.
