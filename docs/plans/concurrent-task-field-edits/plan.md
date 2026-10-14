---
created: 2026-10-04
status: in_progress
requirements:
  - REQ-TASKS-FIELD-UPDATES-001
system_design:
  - ../../specs/tasks/system-design/task-field-updates.md
legacy_specs: []
---

# Implementation plan: Preserve concurrent task field edits

## Overview and checkpoint

Preserve omitted fields by carrying request presence to a locked current-row mutation, retaining
existing hierarchy, completion, title/metadata ownership, association and publication contracts.
One sequential work order owns the complete interface/service/storage/protocol evidence slice.
ROOT reviewed the four-file package and explicitly released implementation in this same primary
on 2026-10-04. One sequential work order is in progress; delivery remains subject to its
joined test/resource/lint/review/merge gates.

Tasks owns the missing ordinary-update persistence contract. Existing sidebar editing describes
an entry point rather than concurrent request omission; existing hierarchy and association
specifications stay authoritative for their own outcomes. The new
[requirement](../../specs/tasks/requirements/task-field-updates.md) is that smallest missing
contract. The [design](../../specs/tasks/system-design/task-field-updates.md) names every request
field, interface/dependency, writer category, and directional exclusion before implementation.

## Confirmed cause and retained evidence

Base is parent24's actual squash merge `5db79135cb67cb21a0747b2e96444a555343d972`.
`Service.UpdateTask` applies request fields to an early full snapshot. The final hierarchy writer
reads current parent/workspace state but writes other stale snapshot values. Serialization alone
therefore loses the first disjoint edit. ROOT's retained proof used actual Service and private
SQLite, a construction-supplied delegated real writer barrier, and explicit commit ordering.
Both APIs succeeded in both orders; title-first lost title to `Test Task`, description-first lost
description to empty. Sequential description then title and unrelated fields passed.

Read-only parent-owned evidence: `/tmp/kandev-concurrent-task-fields-repro_test.go` and
`/tmp/kandev-root25-task-field-proof-receipt.json`. Handle 59875 actually joined expected exit 1;
test .240s/package .307s; archive SHA256
`7204eae466fa0fcba46fd3bed33c009900151868b15f5af1bb9a624e863af7a9`.
No replay, modification or child cleanup of that proof. Permanent RED must assert the same real
behavior through existing APIs before introducing the proposed repository method.

## Scope and technical approach

Introduce a small typed task-field patch/result and required repository method; start its
candidate from the writer-transaction current task after workspace/step/task locks. Reuse
`updateTaskTx`, hierarchy validators and normalization; move the existing metadata protection
calculation to models with a service wrapper. Service preserves normalized assignee presence,
uses actual prior state/step from the mutation, retains priority-only scalar writes, and fixes
the shortcut's missing assignee guard. Keep separate repository preparation/finalize/replacement
and postcommit rereads. Registered REST/WS shapes and normalization stay unchanged.

The request API must not fall back to an early full snapshot. Legacy full-row, exact, workflow,
runtime metadata-merge, Office scheduling and capacity/promotion methods keep intentional
semantics; no symmetric arbitrary-field guarantee is asserted for them. No generic callback
framework, global mutex/revision/per-key merge, schema, UI, runner, launch-policy, flag, or cascade
redesign. Newly found material coupling beyond the design requires a concrete ROOT scope amendment.

## Tests and acceptance mapping

Exact permanent tests and files are in the work order. All nine new names and the anchored
compatibility selectors now exist. An empty selection or missing-API compile error cannot
count as RED/GREEN.

| Criteria | Real behavior evidence |
| --- | --- |
| `.1`, `.2` | Two independent Services/connections on private SQLite, delegated initial-read barrier, both disjoint orders, omission/empty/same-field and sequential controls |
| `.3` | Actual mixed priority/assignee request, current-state completion validation, supplied service-level workflow step with transition ledger |
| `.4`, `.5` | Current metadata and generated-title CAS owner races, explicit metadata replacement controls, native PG workspace and task-row physical waits against scalar changes |
| `.6` | Current-row hierarchy/completion rejection, encode/store rollback, cancellation and `errors.Is`, unchanged persisted row/ledger/entries and no successful service events |
| `.7` | Registered REST PATCH and WS dispatch, actual DB/DTO/event payloads, omitted/null/explicit empty fields, existing detach/error mapping and association failure suppression |

All criteria above are `AC-TASKS-FIELD-UPDATES-001`. Existing exact parent/ABA/cycle, association
replacement/F19 and generated-title/deferred-launch controls remain in the narrow verification
commands. The PostgreSQL test must actually execute using its environment gate; SKIP is recorded
separately and is not physical-lock proof.

## Mobile and public documentation audit

Backend data only. No rendered UI/copy/layout/navigation/touch/breakpoint or frontend state
change is proposed. The same unchanged event/DTO path serves desktop and mobile; mobile-parity
requires no browser, web build or E2E expansion here. Actual registered protocol tests provide
the causal end-to-end evidence. UI or policy expansion requires ROOT's explicit scope amendment.

The existing WebSocket reference now clarifies ordinary omission/disjoint updates, metadata
replacement and full-snapshot exclusions. No new payload field, global event order, arbitrary
metadata merge or full-snapshot safety is advertised. The reference owns API wording; no new
page, screenshots, navigation or tasks-and-workflows guide change is needed.

## Work orders

- [ ] [Task 01: Preserve request intent through the task-row write](task-01-preserve-field-intent.md)

## Verification results

Design package passed catalog, 36 specification-linter unit tests, all-spec lint, local links,
acceptance/reference/whitespace checks and owning Tasks discovery before ROOT's release.
No product tests or installs ran in the design turn; permanent tests below ran after release.

Implementation verification is recorded below. Delivery remains pending until required lint,
normal hooks, current-head hosted gates/full review and independently verified actual merge.
Exact operation handles, command logs and resource identities are retained in the external
Kandev task plan; parent-owned proof remains read-only.

## Execution and delivery constraints

The sole primary owns integration, communication and artifacts; no delegation or new session.
One heavy local command at a time, every returned handle retained and actually joined before
the next heavy command, dependent edit or publication. Scoped race commands use GOMAXPROCS=2,
GOMEMLIMIT=512MiB, trimpath, fts5 and p=1. PG fixture uses exact owned identity and private schemas;
never touch foreign resources. Retain dependencies/worktree for parent archive.

Full changed-revision original-binary backend lint is mandatory before push, independently of
scoped lint/hook. ROOT's task-specific exception is CLI10m/GNU11m/kill-after10s with GOMAX2,
GOMEM1GiB, concurrency2/allowserial and actual immutable PR base. It does not change repository
policy. Timeout/nonzero with zero reported issues is still FAILED. Resource timeout or failed
transport ends new heavy operations and creates a durable WAITING checkpoint for ROOT; no
automatic resource retry, cache deletion or foreign kill.

Normal hooks/conventional commit/push/ready PR, then one joined 90-minute `scripts/pr-await`
all-terminal JSON monitor. Current full substantive authenticated CodeRabbit App347564 review
must cover all actual changed files at current head; inspect auto report before at most one
needed full request. Optional assurance/style suggestions do not churn published SHA. All actual
inline/grouped findings require concrete disposition. Hosted failure gets exact leaf/log/artifact
classification and ROOT amendment or one explicitly authorized same-head job rerun after terminal
workflow, never a blind retry. Merge only normal expected-head squash after terminal policy gates
and current full review, then independently verify merge identity/parent/tree/all owned blobs and
authoritative main. Completion is actual merge plus independent verification plus joined exact
owned cleanup. Parent owns archive/proof cleanup/next discovery.

## Risks

Missing task-row locking would still lose scalar CAS commits on PostgreSQL. Reusing an early
state/step would create transitions or misleading events. Moving metadata protection must retain
pending-title and ordinary replacement semantics. Interface fake adaptation must preserve the
meaningful existing tests. Expanding guarantees to legacy full snapshots would be false.

## Implementation verification checkpoint

Existing-API independent SQLite RED reproduced both lost-field orders before production edits;
a second RED reproduced mixed priority/assignee dropping its assignment. Two further meaningful
REDs found omitted pending-title metadata deleting a current JSON-null value, and private presence
leaking through postcommit dispatch into a real independent legacy writer. Corrections preserve
omitted locked metadata and scope presence to the transaction, retaining supplied-map and legacy
pending merge semantics. No missing-API compile error is counted as behavioral evidence.

Joined affected race checks:

- Eighteen service names: PASS, package 2.054s. After the final context scoping correction,
  the exact concurrent-update and protected-owner names passed again (1.406s).
- Eight registered protocol/compatibility names: PASS, package 3.567s. Actual registered REST/WS
  updates verify database, DTO and event payloads; legacy mapper fakes retain their narrow purpose.
- Seven SQLite storage names: PASS, package 2.441s. Real entry/runner allocation, committed
  ledger/marker identities, late storage rollback and dispatcher-to-legacy writer are included.
- Three PostgreSQL names: actually executed PASS, package 13.863s. All eight new subcases observed
  independent workspace/task-row `Lock/transactionid` waits and real commits/cancellation; previous
  hierarchy wait/cancellation controls also passed. No skipped case is claimed as execution.

Both owned tmpfs PostgreSQL fixtures had all schemas/clients drained and exact container IDs
and private credential files removed after joined tests. No volumes or foreign resources were
created/touched; ROOT proof is unchanged. The single pinned pnpm 9.15.9 frozen install passed
and its dependencies remain for normal hooks and parent archive.

Catalog, all-spec/all-harness lint, targeted backend harness hook and public-reference validation
passed. Scoped original-binary lint passed with zero issues after correcting two concrete
test-code findings (unchecked rows close and nested PG assertion). Mandatory full changed-revision
original-binary `./...` lint passed with zero issues, exit 0, elapsed 340.28s against immutable base
`5db79135cb67cb21a0747b2e96444a555343d972`, using the task-local CLI10m/GNU11m bound. No timeout
is claimed as a pass. Normal hooks, publication, current-head full review, hosted gates and actual
merge remain delivery steps; authoritative receipts continue in the external task plan.
