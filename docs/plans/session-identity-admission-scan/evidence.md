# Session identity admission evidence

Collected on 5 November 2026. The live instance was inspected read-only. All
query work ran on a disposable copy of a boot snapshot.

## Incident timeline

Source: `/root/.kandev/logs/backend-logs-2026-11-05-000002.log`. Times are UTC.

| Task created | Session entry created | Prepare interval | POST /tasks | Blocked GET /task-sessions/:id released |
| --- | --- | --- | --- | --- |
| 10:25:15.108 | 10:25:17.518 | 2.41 s | 2,438 ms | none logged |
| 10:25:41.276 | 10:25:41.571 | 0.30 s | 307 ms | none logged |
| 10:25:55.868 | 10:26:02.725 | 6.86 s | 6,885 ms | 10:26:02.725 (6,193 ms) |
| 10:26:21.995 | 10:26:25.677 | 3.68 s | 3,708 ms | 10:26:25.674 (3,166 ms) |
| 10:26:46.713 | 10:26:46.972 | 0.26 s | 273 ms | 10:26:46.972 (142 ms) |

Probe failures occurred at 10:25:17.475 and 10:26:02.475, both at
`writer_acquire` with `writer_in_use=1` and `reader_in_use=0`. Each falls inside
a slow prepare interval, and in each case the writer was released when the
session entry was created. Persistence recovered on the next probes, at
10:25:30.475 and 10:26:15.476. The slow HTTP requests that ended inside those
intervals were GitHub API reads, which do not hold the writer. Logs cannot
prove that nothing else held it. The query measurements below are consistent
with the durations, but the live host's page-cache state was not measured.

## Dataset

The data is a copy of
`/root/.kandev/data/backups/kandev-v0.98.0-210-g067632acaa2-20261105T092955Z.db`
(14,697,992,192 bytes) at `/tmp/kandev-contention-repro/kandev.db`.

| Table | Rows |
| --- | --- |
| agent_delivery_effects | 1,015,205 (259.8 MiB table pages) |
| agent_delivery_inbox | 1,013,313 |
| agent_delivery_submissions | 1,071 |
| agent_delivery_cursors | 273 |
| agent_delivery_cleanup_jobs | 0 |
| task_sessions | 2,294 |

The host had 28 GiB RAM and about 20 GiB page cache, on ext4 over LVM. The host
was shared.

## Query measurements

The measurements used the `sqlite3` CLI with `.timer on` and literal IDs that
match no rows. "Cold" means that the copy's pages were evicted first with
`os.posix_fadvise(fd, 0, 0, POSIX_FADV_DONTNEED)`.

| Statement | Plan for effects | Warm | Cold |
| --- | --- | --- | --- |
| Current `OR` across `LEFT JOIN` | `SCAN e` | 0.243–0.257 s | 4.69–4.91 s |
| Effects branch without `session_id <> ''` | `SCAN agent_delivery_effects` | n/a | 4.49–5.67 s |
| Indexed effects branch with `session_id <> ''` | `SEARCH ... idx_agent_delivery_effects_owned_retirement` | under 1 ms | 13–51 ms |

Equivalence: the original and indexed effect predicates matched for 400 IDs
drawn from `task_sessions`, `agent_delivery_cursors`, and
`agent_delivery_effects`, with 0 mismatches.

## Implementation re-measurement

These measurements ran after Task 01, against the same copy. Python's `sqlite3`
module bound the parameters as the Go driver does, with a read-only connection
per run and a nonexistent UUID. The full race suite for the sqlite package was
running at the same time, which slowed the original's cold reads.

| Statement | Plan for effects | Warm | Cold |
| --- | --- | --- | --- |
| Original (6 parameters) | `SCAN e` | 288–879 ms | 10.3–28.4 s |
| Final (5 parameters) | `SEARCH ... idx_agent_delivery_effects_owned_retirement` | 4.6–6.1 ms | 18.8–28.1 ms |

Equivalence: across 401 IDs, both statements returned the same result, with 0
mismatches. The IDs were a seeded random sample from all 2,294 distinct IDs in
sessions, cursors, effects, inbox, and submissions, plus one nonexistent UUID.

## Not established

- The page-cache state on the live host during the incident was not measured
  directly.
- Other writer holders in the wider system remain unattributed. They did not
  appear in these two windows.
