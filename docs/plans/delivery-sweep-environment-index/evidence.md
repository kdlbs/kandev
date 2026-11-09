# Issue 5088 diagnostic evidence

## Source and scope

- Issue: [kdlbs/kandev#5088](https://github.com/kdlbs/kandev/issues/5088).
- Investigated checkout: `5e751beb1f4d1ce3747895dfffbfb84ecf809238`.
- Date: 2026-11-05.
- Environment: Go 1.26.0, Linux amd64, SQLite 3.51.1.
- Assignment: GitHub confirmed `carlosflorencio` as an assignee.

The reporter described a 160.8-second query, probe deadlines, and widespread 503
responses. Those production measurements remain reporter evidence.
The local experiment reproduced the query-plan defect and its index correction.
It did not reproduce the complete HTTP outage or use the reporter's database.

## Reproduction

A temporary package-internal Go test used `newInternalTestRepo` and the real
`taskrepo.Provide` bootstrap. It seeded one workspace and repository, one target
task, and 3,500 owner tasks with separate environments.
The target had one inherited session binding per environment.
Each environment had one active repository binding and one snapshot.
This intentional task skew made the existing task-only index unselective.

The seed used one transaction and fixed timestamps starting at
`2026-11-05T12:00:00Z`. Snapshot times increased by one second.
After `ANALYZE`, the test explained `mostRecentGitSnapshotObservationQuery`.
It then called `mostRecentInputObservation` before and after index creation.
The latter call includes the other existing input-source queries.

The diagnostic command was:

```bash
(cd apps/backend && go test -trimpath ./internal/delivery -run '^TestReproIssue5088EnvironmentBindingLookup$' -count=1 -v)
```

The temporary test passed and was removed at the design handoff.
The permanent regression and its reproducible fixture are work-order deliverables.

## Observed plans

The failing lookup was:

```text
CORRELATED SCALAR SUBQUERY 1
SCAN binding
```

After this statement:

```sql
CREATE INDEX idx_task_sessions_environment_task
ON task_sessions(task_environment_id, task_id);
```

SQLite reported:

```text
CORRELATED SCALAR SUBQUERY 1
SEARCH binding USING COVERING INDEX idx_task_sessions_environment_task (task_environment_id=? AND task_id=?)
```

Both plans used `idx_git_snapshots_environment` for snapshot access.
The provenance lookup used the session primary key.
The other environment-repository lookup used `idx_task_environment_repos_env_id`.

## Observed measurements

| Operation | Elapsed time |
| --- | ---: |
| Observation before index | 275.543 ms |
| Index creation | 0.838 ms |
| Observation after index | 3.658 ms |

Both observations returned `2026-11-05T12:58:19Z`.
These are single diagnostic samples, not production latency guarantees or a
controlled benchmark across machines.

## Upgrade findings

`initSchemaContext` runs `initSessionSchema` before `runMigrations`.
The environment column enters legacy session tables inside `runMigrations`.
The later `migrateSessionsRemoveWorkflowStepID` and
`migrateSessionsRemoveAgentExecutionID` paths can rebuild the session table.
Index creation must follow both rebuilds.

The common migration path also runs on PostgreSQL.
The proposed index DDL uses portable syntax.
No PostgreSQL timing or planner result was collected during this investigation.

## Limits and cleanup

The experiment changed only a temporary test database.
The repository retains no temporary Go test or production change.
The index removes one confirmed nested scan.
No general database-load or persistence-health claim follows from this result.
