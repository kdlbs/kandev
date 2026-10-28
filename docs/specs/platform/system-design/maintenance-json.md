---
status: current
system: platform
requirements:
  - REQ-PLATFORM-MAINTENANCE-JSON-001
---

# Database maintenance JSON design

## Boundary and requirement mapping

The native launcher presents `maintenance.Outcome`; the existing
[maintenance design](pr-watch-and-bounded-storage.md#maintenance-command)
continues to own analysis, execution, backup, locks and compaction. This design
implements all criteria of `REQ-PLATFORM-MAINTENANCE-JSON-001` at the CLI
presentation boundary. No new query, metric, dependency, or execution policy
is introduced.

## JSON contract

Keys are lowercase snake_case. Every listed key is present unless expressly
marked optional; no listed value is null. Numbers are JSON integers, booleans
are JSON booleans, and paths are strings. Unknown future fields may be added;
consumers should ignore them. There is no version field.

- `mode`: string `dry_run` or `execute`, derived from `Outcome.Executed`.
- `report`: object containing `generated_at` (UTC RFC3339 timestamp),
  `database_size_bytes` (bytes), `candidates`, `delivery_retention`, and
  `total_candidate_rows` and `total_estimated_bytes` (integers).
- `candidates`: object with `duplicate_git_snapshots`,
  `obsolete_plan_revisions`, and `orphaned_message_payloads`. Each group is
  an object with `rows`, `estimated_bytes`, and `sample_limit_reached`.
- `delivery_retention`: object with `available` and `unavailable_reason`
  (string, empty when available; otherwise `schema_unavailable` or
  `format_unsupported`), and `candidates`. When available,
  `candidates` is an object with groups `projected_delivery_payloads`,
  `settled_delivery_prompts`, `retired_delivery_event_receipts`,
  `retired_delivery_session_evidence`, and `orphaned_delivery_identities`,
  each with the same three group fields. When unavailable, omit `candidates`;
  do not represent missing measurements as zero.
- `execution` is omitted for dry runs and present for executed outcomes. It
  has `backup_path`, `partial`, and integer counts `deleted_git_snapshots`,
  `deleted_plan_revisions`, `deleted_message_payloads`,
  `deleted_delivery_event_payloads`, `deleted_settled_delivery_prompts`,
  `retired_delivery_event_receipts`, `purged_retired_delivery_rows`,
  `delivery_orphan_jobs_scheduled`, and `delivery_retirement_jobs_completed`.
  These are the existing human report's committed counts and job counts, not
  a sum of candidates. `partial` is the existing bounded-work flag: a sample
  limit in any candidate group, pending delivery work, or a delivery execution
  error can set it. It is not a general success indicator and can be false
  when compaction fails.
- `compaction` is omitted unless compaction completed successfully. When
  present it has `size_before_bytes`, `size_after_bytes`, and `rollback_path`.

The report is captured before execution. Totals use `Report.TotalCandidateRows`
and `Report.TotalEstBytes` over reported groups, including available bounded
samples. Byte estimates describe row content, not guaranteed reclaimed disk
space. Sample-limit flags mean the respective group may have more candidates;
retired session evidence shares its sampling budget. The JSON contains no IDs,
row content, credentials, or configuration. Unavailability reason is the
existing bounded structural reason, not arbitrary errors.

## Launcher flow and failures

Add a boolean flag in `maintenanceDatabaseArgs` and its parser, document it in
help, and branch only at presentation after the existing `maintenance.Run`.
Use purpose-built launcher JSON structs with explicit tags and a mapping from
`Outcome`; do not serialize domain structs implicitly. Use an encoder or
writer that returns its error and writes exactly one object plus newline. Keep
human output as-is. On `Run` error with `Executed`, first emit the mapped
committed outcome then diagnose the domain error and exit 1. Before an
executed outcome, keep existing error-only handling. If JSON output fails,
diagnose it and exit 1 even if the domain succeeded; if both fail, report both
without claiming full output was emitted. A broken writer can leave a partial
object and cannot be repaired by the command. Help and argument errors retain
their current early-return behavior. No logger output is added to stdout.

## Verification boundary

Launcher tests exercise the real `runMaintenance` path and disposable SQLite
fixtures with captured stdout, stderr and exit status. Decode one object and
verify EOF; cover default text, dry-run nonmutation, execution and backup,
lock rejection, compaction, unavailable schema, bounded samples, and failure
after committed progress. A narrow injectable presentation/run seam may make
post-commit failure deterministic without changing domain behavior; use real
SQLite for the ordinary paths. Separately use a failing writer to verify output
errors. Existing configuration-clearing and temporary-directory patterns apply.
