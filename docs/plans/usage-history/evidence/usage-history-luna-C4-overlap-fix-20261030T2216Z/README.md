# C4 overlap and pending-accounting lookup proof

Source head: `90767205f97f4b8268526255b935c3626c1a07fe` (uncommitted owned changes). The coordinator's current schema-2 phase authorized this bounded prototype. No full benchmark was run or marked ready.

## Changes

- Split overlap matching into point and coarse interval branches. The point branch uses the existing interval index; the coarse branch uses a partial reverse interval index on `(scope_id, selection_partition, retired, coverage_end, coverage_start)` for non-native non-point rows.
- Keep exact event ID, blank external ID, `selection_coverage`, model/provider resolution, and 201-reference preparation behavior. SQL ordering is omitted only for overlap lookup: callers return `Preparing` before consuming more than 200 references, and complete sets are sorted in memory.
- Replace the pending-accounting `EXISTS` probes with ordered scalar first-row lookups. SQLite selects the exact pending partial index; PostgreSQL's default planner selects its matching partial index. The predicate still includes `turns IS NOT NULL OR native_accounting_version < 1`.

## Measured results

The tagged 20,000-row fixture compares the production SQL with the previous query and records actual SQLite scan-status visits and PostgreSQL `EXPLAIN ANALYZE BUFFERS` rows/buffers. PostgreSQL uses its default planner; no planner override was used.

| Lookup | Engine | Previous rows/visits | New rows/visits | Previous buffers | New buffers |
|---|---|---:|---:|---:|---:|
| External point overlap | SQLite | 20,002 | 4 | n/a | n/a |
| Native event overlap | SQLite | 20,002 | 4 | n/a | n/a |
| Coarse interval overlap | SQLite | 20,002 | 53 | n/a | n/a |
| External point overlap | PostgreSQL | 4 | 4 | 15 | 7 |
| Native event overlap | PostgreSQL | 4 | 4 | 15 | 7 |
| Coarse interval overlap | PostgreSQL | 52 | 53 | 26 | 20 |
| Empty pending guard | SQLite | 10,000 | 0 | n/a | n/a |
| Real turns-pending row | SQLite | 1,864 | 1 | n/a | n/a |
| Version-only pending row | SQLite | 4,593 | 1 | n/a | n/a |
| Empty pending guard | PostgreSQL | 20,002 | 0 | 4,001 | 2 |
| Real turns-pending row | PostgreSQL | 20,002 | 1 | 4,001 | 2 |
| Version-only pending row | PostgreSQL | 4 | 1 | 2 | 2 |

PostgreSQL's original overlap plan was already selective on this fixture. Its measured row work stays flat while buffers fall. The coarse PostgreSQL case visits one extra row and uses fewer buffers; this result is reported as measured.

## Validation

- Final tagged SQLite and actual PostgreSQL scan proof: `sqlite-postgres-final-scan-proof.log` — PASS. Includes exact result-set parity, native nonblank event ID, two distinct native events at one point, blank external IDs, coarse interval, and empty/turns/version-only pending cases.
- Tagged SQLite race scan proof: `sqlite-race-scan-proof.log` — PASS.
- Focused normal suite: `focused-normal-proof.log` — PASS on SQLite and PostgreSQL for native accounting repair, version-zero repair after turns are cleared, oversized group resume, and overlap selection parity.
- Focused race suite: `focused-race-proof.log` — PASS on SQLite.
- Scoped Go lint: `scoped-lint.log` — PASS, zero issues. Package-scoped `go vet`: `scoped-vet.log` — PASS.
- `postgres-initial-20k-failure.log` preserves the original failing guard plan. `postgres-ordered-pending-prototype.log` preserves the successful ordered-probe prototype.

Full five-year SQLite and PostgreSQL benchmark gates remain incomplete and were not run in this phase. Parent review and separate full-run approval are still required.

## Integration note (2026-10-30)

This proof records the source frame at `90767205f97f4b8268526255b935c3626c1a07fe`. The four-file source commit `7d5fd6ee76cb6791fc2fa77b8dd6c87582371cf7` passed normal hooks.
It later replayed range-equal as core HEAD `6a4b002a22ba959dbd636f2fb42eefc31c7dafd2` on A6 `f306b7081105c231f56e309e18378372b8c6472d`, based on main `aebac5d5395e9983124b398198bace146d38506b`.

The parent matched all four source and log hashes after integration, with zero failures and skips. The 15 A6 scoped hashes also remain unchanged. Current-source full SQLite and PostgreSQL runs remain pending. The integration adds no readiness fast path and changes no source, count, oracle, or deadline contract.

The pre-integration backup preserves the original README and checksum manifest.
The current manifest includes this integration note; the raw proof-log checksums remain unchanged.
