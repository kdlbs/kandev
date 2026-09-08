# Budget claim fencing: prior art

Companion to [spec.md](spec.md) (`REQ-OFFICE-COSTS-003`), alongside
[verification.md](verification.md), [input-inventory.md](input-inventory.md) and
[amendments.md](amendments.md). Split out for the specification linter's per-file size
ceiling; the five files are one spec. This file
is the research record behind [Design decisions](spec.md#design-decisions) — it is
provenance, not contract, and nothing in it is an acceptance criterion.


Three legs, each opening with a receipt.

### Our own prior reasoning (wiki) — searched, unavailable on this runner

Receipt: `wiki-query` could not be run. `ls -d ~/.claude/skills/*wiki*` → no match;
`~/.obsidian-wiki/` does not exist; `OBSIDIAN_VAULT_PATH` is unset; `qmd` is not on
`PATH`. This card runs on the **neo** SSH executor, whose home is `/Users/neo`; neither the
vault nor the `qmd` MCP server is provisioned here. Intended query: idempotent alerting,
fencing tokens, crossing-versus-level notification semantics. Nothing retrieved and nothing
claimed — the forks below are **not** established as unopposed by prior positions; re-run
with vault access before treating them as settled. #3287's own `costs-03.md` recorded the
same negative result from a different machine for a different cause, so two independent
runs have now failed this leg.

### What other products shipped (saas-kb) — unavailable

Receipt: the `saas-kb` MCP server and its `search_fsm_docs` tool are absent from this
session's tool list; no tool discovery is exposed. Intended query: `category: "ai_sdlc"`,
budget/spend-limit alerting and its concurrency handling in agent platforms (Devin,
OpenHands, Warp, Factory.ai, Augment). Nothing retrieved; not substituted.

### In-repo prior art — found, and it decides the mechanism

Receipt: `grep -rn "revision\|generation" apps/backend/internal --include='*.go'`, plus
reading each hit's write path. Three implementations of this fencing pattern already
exist, and they agree on all four details:

| Site | Column | Bump | Fence |
| --- | --- | --- | --- |
| `office/repository/sqlite/workspace_groups.go:233,267` | `ownership_generation INTEGER NOT NULL DEFAULT 1` | `task/repository/sqlite/task.go:1555` `= ownership_generation + 1` | `WHERE id = ? AND ownership_generation = ?`, `(bool, error)` from `RowsAffected() == 1` |
| `gitlab/store_config.go:57` | `revision` | `= gitlab_configs.revision + 1` | `RETURNING revision` reads the post-bump value back |
| `user/store/sqlite.go:342` | `settings_revision` | `= settings_revision + 1` | `WHERE id = ? AND settings_revision = ?` + `RETURNING` — a full CAS |

`ClaimWorkspaceGroupCleanup`'s comment states the governing principle: *"The active-member
predicate is part of the same write so a caller cannot race a membership admission between
its read and this claim."* That is Race A, already solved, one package away. It is on
`main` but not on #3287's head, which branched earlier — read it there, not on the build
base.

A fourth precedent decides the *migration* shape rather than the fence.
`office/repository/sqlite/run_outcome_activation.go` guards a schema-dependent action
behind a `columnExists` probe and states why in its own comment: `MigrateLogger.Apply`
"swallows failures at WARN, so this probe is the only thing standing between a failed
migration and a consumer wrongly believing the mechanism is live." That helper sits in the
same package as `office_budget_claims`; it and its reasoning carry
`AC-OFFICE-COSTS-003.3a`.

**Researched, then cut from the contract — provenance for deferred work.** Serializing the
recreate so two initializers cannot both perform it was researched here and then excluded;
it lives in [spec.md's Out of scope](spec.md#out-of-scope), and this paragraph is the record
that seeds it. `task/repository/sqlite/worktree_ownership_migration.go` is the repository's
one-time destructive cutover. It probes for a legacy artifact with `tableExists` and returns
early when it is gone, takes a fixed advisory lock whose comment states the rule —
*"All instances must derive the same bigint so a second boot waits for the first to finish
the cutover instead of racing it"* — and, in its own words, "never uses the best-effort
MigrateLogger path, whose contract swallows unexpected failures." It is the only advisory-lock
precedent in play: `run_outcome_activation.go` above takes no lock. A follow-up should read it
there and choose a new lock id rather than reusing `taskWorktreeCutoverLockID`.

**What this spec takes:** the column shape, the in-SQL `+ 1` bump (never
read-modify-write in Go), the predicate-inside-the-write fence, `(claimed bool, err error)`
read from `RowsAffected`, and `RETURNING revision` for the read-back. **What it departs
from:** `users.settings_revision` fences a *user-facing* write against a client-supplied
expected revision. This spec's revision is **server-assigned and never accepted from a
client** — it fences a background evaluation against a user-facing write, the opposite
direction. Client-supplied would make it optimistic concurrency for the budget PATCH
endpoint, excluded under [spec.md's Out of scope](spec.md#out-of-scope).
