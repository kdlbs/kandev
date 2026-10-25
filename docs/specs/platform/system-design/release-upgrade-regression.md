---
status: current
system: platform
requirements:
  - REQ-PLATFORM-UPGRADE-REGRESSION-001
  - REQ-PLATFORM-UPGRADE-REGRESSION-002
  - REQ-PLATFORM-UPGRADE-REGRESSION-003
---

# Release upgrade regression system design

## Purpose and boundaries

Exercise the published baseline application and candidate application in order
against the same installation. This is test infrastructure owned by Platform;
it changes neither migration algorithms nor task lifecycle contracts.

The frozen `apps/backend/internal/persistence/storeconformance/upgrade_test.go`
suite remains independent. Its `TestPreviousStableUpgrade` currently loads
v0.93.0 SQL, initializes store adapters, replays initialization, and runs store
scenarios. That does not exercise a release process or orchestrator lifecycle.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-PLATFORM-UPGRADE-REGRESSION-001 | Artifact identity; Isolated fixture; Execution and evidence |
| REQ-PLATFORM-UPGRADE-REGRESSION-002 | Seed and preservation; Database checks |
| REQ-PLATFORM-UPGRADE-REGRESSION-003 | Lifecycle scenarios; Browser checks; Execution and evidence |

## Artifact identity

`scripts/upgrade/prepare-release.mjs` resolves latest Stable once using
GitHub release metadata, rejects draft/prerelease/non-Stable tags, dereferences
the tag to a commit, downloads the platform's published runtime archive and
matching SHA256 asset, and verifies archive bytes before extraction. Use the
standard archive and its bundled host agentctl. Preserve archive permissions
and reject extraction outside the owned artifact directory. The candidate
uses binaries built by the managed E2E runner from the checked-out revision.

An explicit Stable tag option permits replay. Preparation publishes its manifest
only after all acquisition/build checks succeed and refuses to overwrite it.
The CI runner accepts a prepared manifest via `KANDEV_UPGRADE_MANIFEST`;
CI transfers a tarred runtime so executable modes survive artifact transport. Each run writes a JSON manifest
containing source/digest identities, engine version, seed version, resolved
configuration, phase times, and required scenario results. Resolve the baseline
once before the engine/browser matrix and distribute the same artifact manifest
to all jobs. Never resolve latest separately inside a worker.

Planning evidence on 2026-10-20: published Stable v0.97.0, peeled tag commit
`e43881c7555372897b57ec51c705f1e05da43c40`; inspected main/HEAD
`013facd0198f6a2b0192e32b5e694f4fa55c96c6`. These are evidence, not hardcoded
future baseline selection.

## Isolated fixture

`apps/web/e2e/fixtures/upgrade-backend.ts` owns the runtime. Do not extend `backendFixture`
in a way that executes its initial candidate spawn: that would migrate the
database before seeding. Reuse process-group teardown, port assignment,
structured logging, and readiness patterns from `fixtures/backend.ts` without
changing the ordinary fixture's startup contract.

Allocate an owned temporary root with isolated HOME, KANDEV_HOME_DIR, config,
local Git repository/origin, agent homes, keys, ports and worktrees. PostgreSQL
uses an isolated test database, never an arbitrary supplied production DSN.
Check ownership before deletion. Keep paths stable across both binary versions.
Both applications must resolve their own matching host agentctl; do not leave
the candidate helper adjacent to the baseline executable.

Use production profile selection with explicit mock-provider/test timing
overrides shared by both phases. Do not select the e2e profile wholesale: it
can enable different feature defaults. Record all overrides and keep external
network integrations disabled. The deterministic mock protocol is verified on
both versions during baseline preflight. Build the baseline mock-agent helper
from the exact baseline source; this does not replace the released
backend binary. Never work around a protocol failure with raw database inserts.

Preflight validates old command/config shape, readiness endpoint, API shapes,
profile discovery, and a complete mock turn. Put explicit baseline API
adaptations in `helpers/upgrade-api.ts`; candidate operations use current
`helpers/api-client.ts` and existing page objects. Unrecognized shapes fail
with an explicit capability error. No speculative adapter hierarchy is needed.

## Seed and preservation

`helpers/upgrade-seed.ts` creates a versioned synthetic scenario through the
baseline's HTTP/WS APIs. It records returned IDs and normalized API snapshots,
including generated timestamps where preservation is expected. Never fabricate
legacy schemas with current repository constructors.

Create workspace/repository, a multistep workflow, agent/executor profiles,
user preference, and uniquely marked task descriptions, plan content, user
messages and agent responses. Seed separate tasks for pending, waiting,
completed, archived and interrupted scenarios. At least one task retains
multiple sessions so selected/primary identity cannot be inferred from count
alone. Persist a small worktree file and verify its content after restoration.

Produce interrupted state by starting a controllably blocked mock turn,
observing its execution evidence, and terminating the baseline process group.
Do this after all other seed snapshots. Verify no runtime process survives;
this suite exercises cold restart recovery, not retained-process adoption.

Store immutable seed expectations separately from the expected post-action
snapshot. Compare semantic projections, not entire response objects or raw DB
bytes. Declare exact allowed transformations by field and baseline version;
blanket removal of timestamps, errors, or state fields is forbidden. After
candidate actions, capture their expected results and verify them on replay.

## Database checks

Run the candidate against baseline storage, await `/ready`, and verify running
version/commit identity. Application reads establish visibility and permissions;
read-only database checks establish integrity. The integrity oracle runs after
owned processes stop, using SQLite integrity/foreign-key checks and explicit
domain relationship queries. On PostgreSQL, inspect constraints and equivalent
domain orphans in the isolated database. Account explicitly for older schemas
that lack particular FK constraints; absence cannot prove relationship health.

For SQLite, inspect the pre-migration backup created by the real startup path
and verify baseline sentinels in a separate read-only connection. Follow
[SQLite file hygiene](sqlite-file-hygiene.md) and the current backup design;
do not assume the original VACUUM INTO implementation of ADR 0008 remains.
PostgreSQL has no automatic SQLite snapshot assertion.

Boot candidate a second time on the same data. Check readiness, immutable seed,
post-action state, integrity, and duplicate identities/messages/step entries.
This tests migration replay and persistence, independently of first-boot success.

## Lifecycle scenarios

`tests/upgrade/release-upgrade.spec.ts` runs each engine as a separate
CI job/configuration, using identical scenario assertions:

1. Read every seeded task/history/plan without triggering work. Compare task,
   session and execution identities before/after passive reads.
2. Start the seeded pending task; send a follow-up to the waiting session;
   explicitly resume the completed conversation; recover interrupted work via
   its supported recovery action. Assert deterministic response markers,
   preserved historical order, and expected session/turn identity separately.
3. On a new task, exercise start, response, controlled long turn, stop, resume,
   workflow step move, completion, archive and unarchive. Observe actual
   runtime/message evidence in addition to persisted state.
4. Restart and verify all resulting durable state. Do not infer live execution
   from a RUNNING database row or accept readiness alone as lifecycle success.

Reuse `ApiClient.createTask`, `moveTask`, `archiveTask`, `unarchiveTask`,
`stopSession` and session APIs where applicable. Inspect current session resume
and workflow specs during implementation to use their exact request shapes.
Use causal HTTP/WS waits and bounded `expect.poll`, never arbitrary sleeps.
Explicit conversation recovery uses the production `session.recover` action
with `action: resume`, which grants completed-session resume authority; a plain
`session.launch` resume does not. A genuinely completed baseline conversation is
created through a workflow profile switch whose source step uses the `complete`
end policy. Task completion itself is a separate public task-state mutation.

Archive cleanup is asynchronous. Before unarchive, read the owned cleanup job
state with bounded polling until `succeeded` or `cancelled`; `failed` is an error.
These read-only live job observations are lifecycle synchronization, separate
from the offline integrity oracle. Immediate unarchive while cleanup is running
is outside this scenario. Backup checks use the immutable original `seed.json`,
so candidate-created tasks must be absent from the pre-migration snapshot.

## Browser checks

`tests/upgrade/mobile-release-upgrade.spec.ts` and the desktop spec open the
candidate `/t/<seeded-task>` route, verify old history and plan, and submit a
follow-up through `SessionPage`. Reuse the existing completed-session and
mobile completed-session resume patterns. Assert the response belongs to the
same selected conversation; fail on page exceptions or relevant failed API
calls. Browser smoke runs on SQLite; the complete API/runtime lifecycle runs
on SQLite and PostgreSQL. Report this coverage distinction explicitly.

No rendered product UI changes are proposed, so no new UI composition or ASCII
preview is needed. Existing desktop/phone interfaces are the test subjects.

## Execution and evidence

The standalone `.github/workflows/upgrade-regression.yml` runs on pushes
to main, a nightly schedule, and manual dispatch on main. Do not register
pull-request triggers. Guard rehearsal jobs with `github.ref ==
'refs/heads/main'` so manual dispatch from another branch cannot launch them.
For pushes, test the triggering commit; for scheduled/manual runs, resolve main
once. Pin checkout/build and every matrix job to that exact candidate SHA and
record it separately from the workflow definition SHA. Use read-only permissions,
pinned actions and no secrets in candidate execution. Respect existing one-worker
and memory-aware shard constraints. Fail if a requested engine or required
scenario is absent/skipped; normal E2E jobs exclude upgrade specs so they do not
implicitly download releases or require PostgreSQL.

Phase-specific failures identify acquisition, baseline preflight/seed,
candidate boot/migration, preservation, lifecycle, or replay. Keep logs,
manifest, seed snapshots, semantic diff, JUnit and Playwright trace on failure;
retain only synthetic data. Teardown executes even on seed/startup failure and
terminates every owned process before deleting temporary storage. Infrastructure
retries may retry bounded downloads, never silently switch baselines or restart
a failing lifecycle scenario until green.

## Related designs and decisions

- [Required persisted store parity](postgres-domain-store-parity.md)
- [SQLite file hygiene](sqlite-file-hygiene.md)
- [Replayable migrations](../../../decisions/0027-replayable-schema-migrations.md)
- [Separate backups from compaction](../../../decisions/2026-10-12-separate-sqlite-backups-from-compaction.md)
- [Implementation plan](../../../plans/release-upgrade-regression/plan.md)

## CI entry points

`bash scripts/upgrade/run.sh` is an internal CI command that invokes `run.mjs`, which provisions a disposable
PostgreSQL 16 service when needed and calls the managed E2E runner with one
worker. The runner parses the Playwright JSON report and requires every named
scenario to run exactly once and pass. `--no-build` reuses candidate artifacts.

Fixture/oracle unit tests use `e2e/upgrade.vitest.config.ts` because ordinary
Vitest deliberately excludes fixture tests. `upgrade-global-setup.ts` validates
only the candidate artifacts this suite actually needs. Regular E2E projects
retain their original global setup and exclude upgrade specifications.

## CI-only execution

The acquisition function, runner, shell entry point, Playwright global setup
and runtime fixture require `CI=true`, `GITHUB_ACTIONS=true` and
`GITHUB_REF=refs/heads/main`. The managed E2E runner checks upgrade project names
before Docker probing or any build. No local override is provided. CI contract
and guard tests run within the workflow; maintainers dispatch the workflow on
main instead of reproducing workloads on their machine.

When baseline and candidate report the same binary version, the rehearsal still
executes preservation, lifecycle and replay scenarios but no version-change
backup is required. Candidate identity comes from `/api/v1/system/info.commit`,
so an exact Stable tag without a SHA suffix is a valid candidate build.
