---
created: 2026-10-04
status: implemented
requirements:
  - REQ-PLUGINS-MANAGED-COORDINATION-002
system_design:
  - ../../specs/plugins/system-design/managed-coordination.md
legacy_specs: []
---

# Implementation plan: Atomic managed conversation settings

## Overview

Restore revision and idle admission for retained HOST managed conversations across
independent services and runtime row writers. One coherent sequential work order
contains existing-API RED, mandatory native admission, writer integration, and
meaningful SQLite/PG16/Host evidence. ROOT reviewed this four-artifact package after the design turn ended and explicitly
released implementation and delivery in the same primary session. No delegation,
new tasks, tabs or sessions are authorized.

Ownership: the existing [requirement](../../specs/plugins/requirements/managed-coordination.md)
and [design](../../specs/plugins/system-design/managed-coordination.md#atomic-managed-settings-admission)
own installation-scoped host managed lifecycle. This is not a production plugin.
Preserve both broad documents' draft statuses and the completed
[original package](../plugin-coordinator-platform/plan.md) and its history.
Existing [coordination ownership decision](../../decisions/2026-09-25-plugin-coordination-platform.md)
remains applicable. This narrow native admission has sufficient rationale here
and in the owning design; no new ADR or fifth artifact is needed.

## Evidence and assumption check

Authoritative main/proof base and initial clean HEAD:
`1a2c60cf81e9da95d3572aa689c1b5c9e3a2271b`.
Read-only `/tmp/kandev-managed-config-admission-repro_test.go` SHA256
`0761da29d313cc2de7f94492ff6400543b2feefb9a943f51851692a535d21a35`
matches `/tmp/kandev-root28-managed-config-proof-receipt.json`.
Accept ROOT's joined handle 20001, exit 1, test 0.27s/package 0.336s without replay.
The delayed service overwrites independently accepted RUNNING with CREATED and
accepts stale revision 1 after another service commits revision 2. Already-running
and idle-update controls pass. No race/setup/compile failure; cleanup joined.

Confirmed intent: atomic current admission, one configuration winner, typed current
errors, coherent task/session commit, retained identity and existing replay/nochange.
Verified: actual service-local locks and separate full writes violate that contract.
Settled limits: no universal snapshot/launch engine, schema, wire, UI, auth, profile
registry, or deletion lifecycle redesign. No material question remains. Do not
reinterpret a ROOT release of this package as authorization for scope expansion.

## Scope and technical approach

- Mandatory typed task repository seam, native SQLite writer reservation and
  PostgreSQL READ COMMITTED workspace -> task -> primary session -> executor locks.
- Current ownership, replay-before-revision, idle and execution predicates, narrow
  primary launch-column writes, current task metadata overlays, atomic create/repair.
- All canonical configuration/revision/pause/invalidation/detach writers participate.
  Per-conversation lifecycle commits precede stop/wake callbacks.
- Required backendapp adapter/provider wiring and real registered Host v2 evidence.
- Preserve full legacy/bulk APIs; their later deliberate snapshot replacement and
  launch preparation predating a config commit keep existing contracts. DeleteManaged
  cleanup/revision preflight stays unchanged; admission obeys its barriers/absence.

No generic callback/coordinator/version engine, optional unsafe fallback, preflight-only
reread fix, runtime-instance/browser work, feature flag, generated contract or new
permissions. No model/profile/executor switch or delegated execution.

| Boundary | Identity and compatibility | Evidence / unavailable handling |
| --- | --- | --- |
| SQLite native task provider | Installation/workspace/instance; independent handles use writer reservation | Existing API races plus atomic rollback and current metadata controls |
| PostgreSQL native task provider in sqlite package | Same contract; READ COMMITTED and physical row locks | PG16 independent pools; task/session/executor wait and cancellation; missing DSN is not evidence |
| backendapp task/session adapters | Compile-time required typed admission forwarding | Real provider/service wiring; legacy-only adapter fails closed, no unsafe managed fallback |
| Host v2 and SDK gRPC adapter | Current authorization/digest/operation receipt, unchanged schema | Registered ExactHost manager with real service/store; CONFLICT and post-commit receipt failure |
| Legacy v1 and generic runtime writers | Existing lifecycle and state/attempt identities | Existing controls; no new universal snapshot/launch guarantee |

## Tests and acceptance mapping

The following permanent test targets map to the acceptance criteria; receipts below distinguish design and implementation evidence.
Use `@covers` beside the actual tests.

| Criteria | Planned meaningful evidence |
| --- | --- |
| 002.1, .4, .5 | `TestManagedConversationAdmissionInterleavings` in service/managed_conversation_admission_test.go: launch_after_idle_read and competing_revision_after_read, independent services, handles, real state/attempt/executor writes and accepted-winner readback |
| 002.2, .3, .5 | `TestManagedConversationAdmissionLifecycle`: exact pause, installation pause, invalidation, detach race accepted settings; installation/workspace/instance isolation and retained transcript |
| 002.6, .8 | `TestManagedConversationAdmissionRollback`: update/create/repair failure between writes, cancellation, disposal; task/session bytes, unrelated metadata/scalars, events and callbacks |
| 002.7 | `TestManagedConversationAdmissionControls`: current/stale/latest/busy/execution-only, empty/nochange, replay-before-revision, missing-primary replay using stored config and ordinary repair |
| 002.4-.8 | `TestManagedConversationAdmissionPostgresWaits`, `TestManagedConversationAdmissionPostgresRegistrationFence`, `TestManagedConversationAdmissionPostgresRollback` in repository/sqlite/managed_conversation_admission_postgres_test.go: independent physical task/session/executor waits, READ COMMITTED current results, absent-registration fence, cancellation and rollback |
| 002.6-.8 | `TestManagedConversationHostAdmissionReceipts` in plugins/host_managed_conversation_admission_test.go: real registered manager + service + command store, APPLIED/NO_CHANGE/replay/CONFLICT and receipt completion failure after commit |

Keep existing `managed_conversations_test.go`, `agent_conversations_real_repo_test.go`,
host lifetime and managed-input controls. Replace only the obsolete partial-new-create
expectation. Race gates forward real reads or admission requests; no behavioral store
mocks. After correction a pre-admission gate may forward to the real transaction;
never pause a transaction while requiring its blocked rival to commit. Use existing
createTestService/seedConversationWorkspace factories and actual independent file-backed
SQLite pools. PG patterns: newHierarchyPostgresRepoPair, OpenIsolatedPostgres,
NewWithInitializedDB and actual pg_locks/pg_stat_activity; do not invent factories.

## Public guide and mobile impact audit

`docs/public/plugins-authoring.md`, Exact managed conversations, already promises
stale-revision and active-turn rejection. No public copy/API change is needed.
Internal docs are these four artifacts. No rendered UI, touch, scroll, navigation,
viewport or localization changes: mobile parity pure state/data exception, supported
by targeted integration tests. No ASCII preview/browser E2E or running instance is
required for this backend correction. Host end-to-end API evidence owns the outcome.

## Work orders

- [x] [Task 01: Admit current managed settings atomically](task-01-atomic-admission.md)

One work order, wave 1, no dependencies, sequential. Full task-level checks are in
the work order. Design-only gates: catalog discovery/validation, spec lint and its
36 regression tests, actual repository coverage evaluator for docs and planned
source triggers, whitespace, exactly four unstaged document artifacts.

## Verification results

Design checks passed on 2026-10-04:

- Catalog: 348 decisions and 1337 specifications validated; owner pair discovered.
- `lint-spec-files.test.py`: all 36 tests passed.
- `lint-spec-files.py --all`: all specification files passed.
- Actual `.github/scripts/pr-docs.cjs` evaluator: actual four documentation paths
  exempt; all six planned production triggers covered by the single work order,
  REQ-PLUGINS-MANAGED-COORDINATION-002 and owning design, zero errors.
- Whitespace and exact four-document/empty-staging assertions passed.

At the design checkpoint, no implementation/PG/lint/install/publication/merge had run. ROOT proof accepted,
read-only hash verified, not replayed. At that checkpoint no heavy handles or owned DB/container
resources were opened. Apps dependencies were absent; existing Node binary is
`/home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node`, available for lightweight
doc evaluation without install. Primary session/profile/executor unchanged.
Implementation phase/release wording is current; record actual checks as they settle. Implementation
checks passing can mark work order done/plan implemented, but the persistent child's
completion gate remains actual verified merge plus joined owned cleanup.

Implementation checkpoint on 2026-10-04: permanent existing-API RED preceded correction;
precise service compatibility matched88 names and passed. Independent native PG16 physical
waits/cancellation/registration/rollback, real Host concurrent-loser and lost acknowledgement
receipts, provider wiring, transcript/effect controls, SQLguard and two-engine conformance
passed with joined handles. See Task01 results and the external task plan for exact receipts.
Owned tmpfs PG was removed after all clients joined; immutable ROOT proof remains intact.
Original full lint identified required handler test wiring and nine concrete size/
complexity/style issues. Minimal fail-closed fixture wiring and focused managed settings
extraction resolved them. Final88conversation/Host/adapter/6handler regression and
independent finalPG16 waits/rollback passed; all owned clients joined and containers
removed. The final same-method QF1008 selector correction passed affected admission
tests (1.562s). Original installed full ./... changed-revision lint passed exit0 in
240.893s, joined handle17930, binary SHA256
67342f8c4ce658ed63692675a906a43293597ab526f97e1c2b60a2900355f06b.
Local implementation is complete. Normal hooks, publication, current-head review/CI
and actual verified merge remain external delivery gates, not completed outcomes.
Current main snapshot99f509743b29d3d021d76167f920304790485d42 is fetched for changed-revision
lint and static compatibility; the original proof base/history remain unchanged.

## Risks

- Wrong PG lock order or a LEFT JOIN lock can deadlock/fail; separate physical
  lock queries and independent wait proofs are required.
- Missing executor rows cannot be row-locked; canonical registration task/session
  barriers must fence creation. Existing executor rows need their physical lock.
- Failing to migrate a pause/invalidation/detach writer permits stale resurrection.
- A receipt or runtime callback can fail after commit; do not claim rollback.
- Legacy full snapshots remain deliberately bounded exclusions, not universal safety.
- Resource timeout/lost handle ends at a durable WAITING checkpoint for ROOT;
  no automatic retry, foreign cleanup, or unrelated CI remediation.

## Documentation coverage preflight

From repository root, use existing Node PATH (or the absolute binary above), with
bash login=false. This invokes the actual evaluator; planned paths are data only.

```bash
node <<'NODE'
const fs = require('node:fs');
const cp = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const tracked = cp.execFileSync('git', ['diff', '--name-only', '-z', 'HEAD']).toString().split('\0').filter(Boolean);
const untracked = cp.execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z']).toString().split('\0').filter(Boolean);
const changed = [...new Set([...tracked, ...untracked])];
const documents = [
  ...fs.readdirSync('docs/plans/managed-conversation-admission').map(p => `docs/plans/managed-conversation-admission/${p}`),
  'docs/specs/plugins/requirements/managed-coordination.md',
  'docs/specs/plugins/system-design/managed-coordination.md',
];
const fileContents = Object.fromEntries(documents.map(p => [p, fs.readFileSync(p, 'utf8')]));
const planned = [
  'apps/backend/internal/task/service/agent_conversations.go',
  'apps/backend/internal/task/repository/interface.go',
  'apps/backend/internal/task/repository/managedconversation/admission.go',
  'apps/backend/internal/task/repository/sqlite/managed_conversation_admission.go',
  'apps/backend/internal/backendapp/adapters_agent_conversations.go',
  'apps/backend/internal/plugins/host_managed_conversations.go',
];
for (const [label, paths] of [['actual', changed], ['planned', [...changed, ...planned]]]) {
  const result = validateCoverage({ changedFiles: paths, fileContents });
  console.log(JSON.stringify({ label, status: result.status, workOrders: result.workOrders, errors: result.errors }));
  if (!result.ok) process.exitCode = 1;
}
NODE
```
