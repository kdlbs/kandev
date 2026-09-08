---
status: active
system: integrations
created: 2026-08-12
updated: 2026-08-22
owners:
  - kandev
---
# Pull request outcome attribution Requirements

## Overview

Kandev already synchronizes pull request state. The sync contains useful facts that were not persisted: draft state, changed file count, the merger, the latest closing actor, and whether auto-merge was observed. Persisting these facts lets integrations and reports use the upstream record without guessing.

## Requirements

### REQ-INTEGRATIONS-PR-OUTCOME-ATTRIBUTION-001: Pull request outcome attribution

**Intent:** Kandev already synchronizes pull request state. The sync contains useful facts that were not persisted: draft state, changed file count, the merger, the latest closing actor, and whether auto-merge was observed. Persisting these facts lets integrations and reports use the upstream record without guessing.

#### Acceptance criteria

- **AC-INTEGRATIONS-PR-OUTCOME-ATTRIBUTION-001.1:** **AC-01 to AC-03:** schema creation and replay add nullable columns without defaults, do not silently ignore migration errors, and remain idempotent.
- **AC-INTEGRATIONS-PR-OUTCOME-ATTRIBUTION-001.2:** **AC-04 to AC-06:** migration does not backfill rows; activation is recorded once, including on fresh installs, and later boots keep the original instant.
- **AC-INTEGRATIONS-PR-OUTCOME-ATTRIBUTION-001.3:** **AC-07 and AC-07a:** every explicit projection, row writer, and legacy table rebuild carries all five columns.
- **AC-INTEGRATIONS-PR-OUTCOME-ATTRIBUTION-001.4:** **AC-08 and AC-08a:** GraphQL requests the shared fields and selects the most recent closed event.
- **AC-INTEGRATIONS-PR-OUTCOME-ATTRIBUTION-001.5:** **AC-09 to AC-11:** REST, gh, GraphQL, and no-op paths report only the fields they actually observed through populated flags.
- **AC-INTEGRATIONS-PR-OUTCOME-ATTRIBUTION-001.6:** **AC-12 and AC-12a:** populated values preserve real zero and false values, and absent optional fields remain `NULL`.
- **AC-INTEGRATIONS-PR-OUTCOME-ATTRIBUTION-001.7:** **AC-13 to AC-15a:** unpopulated values are preserved; closure attribution is written only from a valid GraphQL actor and follows the latest event.
- **AC-INTEGRATIONS-PR-OUTCOME-ATTRIBUTION-001.8:** **AC-16 and AC-17:** the auto-merge timestamp is write-once.

## Migrated source detail

## Why

Kandev already synchronizes pull request state. The sync contains useful facts
that were not persisted: draft state, changed file count, the merger, the
latest closing actor, and whether auto-merge was observed. Persisting these
facts lets integrations and reports use the upstream record without guessing.

## Scope and boundary

The feature persists exactly five nullable fields on `github_task_prs`:

| Field | Meaning |
| --- | --- |
| `is_draft` | The upstream draft value. `NULL` means it was not observed. |
| `changed_files` | The upstream changed-file count. `0` is a real value. |
| `merged_by_login` | The upstream merger login, when present. |
| `closed_by_login` | The latest GraphQL closed-event actor, when present. |
| `auto_merge_observed_at` | The first time Kandev observed auto-merge armed. |

These are observations, not inferred causes. `auto_merge_observed_at` does not
mean that auto-merge caused the merge. A missing closing actor remains `NULL`.

Core does not store human-authored closure reasons or replacement links. A
plugin can own that workflow, including its taxonomy, storage, and UI.

The feature has no core UI, mutation endpoint, or translation keys. The web
package receives the five fields in its `TaskPR` type for future integrations,
but no core component consumes them.

## Persistence

Fresh databases declare the five columns in the GitHub task-PR table. Existing
databases add missing columns with fail-loud, idempotent `ALTER TABLE` steps.
No migration updates or backfills a task-PR row. All five columns are nullable,
have no default, and are added to every explicit read and write column list.

Schema initialization writes the current UTC instant once to
`kandev_meta.github_task_pr_outcome_activated_at`. The write uses a set-if-absent
operation, so concurrent startup cannot replace the first value. A migration
or activation failure stops startup.

The legacy multi-repository table rebuild copies all five columns in both its
new-table DDL and its `INSERT ... SELECT` list.

## Upstream acquisition

- The shared GraphQL field block requests draft state, changed files, merger,
  auto-merge, and the most recent `CLOSED_EVENT` actor.
- The gh CLI and REST pull-request clients request the fields they expose.
  Neither path claims a closing actor because that field is not available from
  those pull-request responses.
- A full REST, gh, or GraphQL read marks the outcome group as populated. A
  partial or no-op read leaves it unpopulated.
- Draft and changed-file values have field-level presence flags. This preserves
  the difference between an observed `false` or `0` and an absent value.

## Sync writer

The normal sync writer applies populated observations. An unpopulated sync
preserves stored values, including `NULL`. A populated response with no merger
clears `merged_by_login` to `NULL`; it does not store an empty string. Closure
attribution follows its own populated flag.

`auto_merge_observed_at` is a latch. The first populated observation with
auto-merge armed sets it. Later observations cannot clear or replace it. The
SQL update also uses `COALESCE` so concurrent syncs cannot overwrite the first
timestamp.

Replace and restore operations resolve the five fields inside their write
transaction while reading the outgoing row. This preserves values when an
operation has no new upstream observation. The stored row is re-read before a
sync event is published, so the event matches the committed state.

After the task-PR write succeeds, the normal sync reconciles the
repository-qualified comparison target. The reconciliation receives the same
task ID and pull-request payload as the sync. A reconciliation error does not
reverse the task-PR write.

## Acceptance criteria

- **AC-01 to AC-03:** schema creation and replay add nullable columns without
  defaults, do not silently ignore migration errors, and remain idempotent.
- **AC-04 to AC-06:** migration does not backfill rows; activation is recorded
  once, including on fresh installs, and later boots keep the original instant.
- **AC-07 and AC-07a:** every explicit projection, row writer, and legacy table
  rebuild carries all five columns.
- **AC-08 and AC-08a:** GraphQL requests the shared fields and selects the most
  recent closed event.
- **AC-09 to AC-11:** REST, gh, GraphQL, and no-op paths report only the fields
  they actually observed through populated flags.
- **AC-12 and AC-12a:** populated values preserve real zero and false values,
  and absent optional fields remain `NULL`.
- **AC-13 to AC-15a:** unpopulated values are preserved; closure attribution is
  written only from a valid GraphQL actor and follows the latest event.
- **AC-16 and AC-17:** the auto-merge timestamp is write-once.
- **AC-18 to AC-18c:** unchanged syncs publish no update, changed syncs publish
  the committed row, and failed writes publish nothing.
- **AC-18d:** a successful normal sync keeps its task and pull-request context
  through persistence, comparison-target reconciliation, and event publication.
- **AC-19:** draft pull requests remain non-mergeable in the stored state.
- **AC-30 and AC-30b:** the backend emits explicit nullable JSON keys and the
  frontend exposes types without adding a core screen.
- **AC-36 to AC-39:** activation-scoped writer-health checks can distinguish a
  missing observation from a legitimate pre-activation or partial-read `NULL`.
- **AC-38:** the populated and unpopulated sync counters remain available in
  dev-mode expvar output.
- **AC-43, AC-43a, and AC-43p:** replace and restore resolve and preserve
  outcome fields inside their own transaction using explicit observation flags.

## Regression scenarios

- **GIVEN** a stored task PR and a comparison-target observer, **WHEN** a normal
  sync applies an authoritative pull-request payload, **THEN** the write succeeds
  before the observer receives the same task and pull-request identity.
- **GIVEN** comparison-target reconciliation returns an error, **WHEN** the
  task-PR write succeeds, **THEN** the stored task-PR update remains committed.

## Verification

Backend unit and persistence tests cover migration replay, source decoding,
presence flags, writer behavior, concurrency, event publication, and table
rebuilds. The frontend change is type-only. No browser or mobile test is needed
because this feature has no user-visible core surface.
