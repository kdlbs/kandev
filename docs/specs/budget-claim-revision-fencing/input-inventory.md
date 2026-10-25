# Budget claim fencing: input inventory

Companion to [spec.md](spec.md) (`REQ-OFFICE-COSTS-003`), alongside
[verification.md](verification.md), [prior-art.md](prior-art.md) and
[amendments.md](amendments.md). Split out for the specification linter's per-file size
ceiling; the five files are one spec.

Every shape below was read out of the code on #3287's head, not inferred. It is the
sampled state Build starts from — reference, not acceptance criteria.


Sampled by reading the code, not inferred.

**`office_budget_policies`** (`repository/sqlite/base.go`) — columns `id, workspace_id,
scope_type, scope_id, limit_subcents, period, alert_threshold_pct, action_on_exceed,
created_at, updated_at`. **No revision or version column.** `GetBudgetPolicy` and
`ListBudgetPolicies` both use `SELECT *` with `sqlx` `StructScan`/`SelectContext` into
`models.BudgetPolicy`, so adding a column **requires** a matching struct field — `sqlx`
fails with `missing destination name <column>` otherwise. This is a hard coupling, not a
style choice.

**`office_budget_claims`** (added by #3287) — `policy_id, period_key, level, claimed_at`,
`PRIMARY KEY (policy_id, period_key, level)`, `FOREIGN KEY (policy_id) REFERENCES
office_budget_policies(id) ON DELETE CASCADE`. Created by `createCostTables()` with
`CREATE TABLE IF NOT EXISTS`, **after** `office_budget_policies` (Postgres rejects the
forward reference). `Repository.Claim` is `INSERT ... ON CONFLICT(policy_id, period_key,
level) DO NOTHING`; `claimed` is `RowsAffected() == 1`; `db.IsForeignKeyViolation(err)`
maps to `(false, nil)`.

**`Repository.UpdateBudgetPolicy`** (#3287) — `BeginTxx`; `DELETE FROM
office_budget_claims WHERE policy_id = ?`; `UPDATE office_budget_policies SET ... WHERE id
= ?`; `Commit`. A test-only failpoint `failBudgetPolicyUpdateErr` sits between the two.

**`evaluatePolicy`** (#3287) — reads the clock once, derives `boundary` once, derives
`periodKey` once, then `switch { case spent >= limit: ...; case spent >= threshold: ... }`.
The exceeded branch calls `claimAndEmit(..., claimLevelExceeded, func() {
s.claimCompanionAlert(...) })`; the `afterClaim` callback runs **between** the exceeded
claim and the emission decision. This callback seam is the mechanism Race B exploits.

**`office_activity_log`** — `id TEXT PRIMARY KEY` (a random UUID), `created_at TIMESTAMP`,
index `(workspace_id, created_at DESC)`. Every reader orders `ORDER BY created_at DESC`
with **no tiebreak column**. `CreateActivityEntry` sets `created_at = time.Now().UTC()` at
write time, only when the caller left it zero. `ActivityLogger.LogActivity` returns
nothing.

**Budget surfaces on current `main`** — `dashboard/service_inbox.go:170`
`listBudgetInboxEntries` lists `budget.alert` **and** `budget.exceeded` as two separate
`ListActivityEntriesByAction` calls concatenated (`append(alerts, exceeded...)`), each
already sorted internally — **not** interleaved by timestamp. That arrived with PR #3276
(`677e01204`), which merged after #3287 branched, so #3287's statement that "only
`budget.alert` feeds the Office inbox list" is **stale against `main`**.
`apps/web/.../activity/activity-feed.tsx` renders `res.activity` in server order with no
client-side sort, so the **activity feed** is the one surface where the two rows interleave
and `created_at` alone decides.

**`updateBudget` handler** (`costs/handler.go:168`) — `GetBudgetPolicy` →
`applyBudgetPatch` → `UpdateBudgetPolicy`, an unfenced read-modify-write that returns the
in-memory `policy` as the response body. `models.BudgetPolicy` is mapped by hand to
`apps/web/lib/state/slices/office/types.ts:138` in camelCase, which ignores unrecognised
JSON fields.
