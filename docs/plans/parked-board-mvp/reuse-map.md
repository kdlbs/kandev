# Reuse map + MR disposition — waiting-attribution sliced delivery

**Date:** 2026-08-10
**Source of truth for reuse:** branch `feature/waiting-attribution-hxr` (task
`waiting-attribution_cca74ncd`), also open as **PR #2476** (nova28 fork → kdlbs/kandev:main).

## The headline

**The monolith is a near-complete implementation of ALL five slices.** Re-slicing is **not a
rebuild — it is a harvest + decompose** of code that already exists, compiles, and is largely
tested. For most slices the agent's job is "cherry-pick these files from the branch and wire
them"; the real *engineering* is concentrated in one place (V1's projection extraction) and the
concurrency slices (V2/V3), which must **pull apart** the one monolithic projection into its
one-shot / sampler / consistency layers.

Every parked/probe file already on the branch (44 files):

- **Probe (both Unix + stubs):** `probe.go`, `probe_darwin.go`, `probe_linux.go`,
  `probe_windows.go` (stub → `unknown`), `probe_other.go` + tests (`probe_unix_test.go`,
  `probe_darwin_test.go`, `probe_linux_test.go`, `probe_notdarwin_test.go`, `probe_test.go`)
- **Transport:** `client_probe.go`, `manager_probe.go`, `agent_probe.go` (+ tests)
- **Recogniser:** `background_launch_recognizer.go` (+ test)
- **turn_started:** `turn_started_emission_test.go`
- **DTO:** `task/dto/parked.go` (+ test)
- **Projection (monolithic):** `orchestrator/parked_state.go`, `parked_projection.go`,
  `parked_eviction_ordering_test.go`, `parked_architecture_test.go` (+ tests)
- **Frontend:** `kanban-card-content.tsx`, `kanban-card.tsx`, `state-icons.tsx`,
  `lib/kanban/parked-projection.ts`, `lib/ws/handlers/kanban-parked*`, `task-parked-merge.ts`,
  `tasks-parked*`, `lib/state/slices/session/session-parked-merge.ts`,
  `lib/state/hydration/hydrator-kanban-parked*`, `hooks/.../use-all-workflow-snapshots-parked`,
  `e2e/.../parked-session-affordance.spec.ts`

Two facts that shape V1 specifically:

- **The runtime flag is NET-NEW.** Nothing under `runtimeflags/` references parked — the monolith
  shipped **unflagged**. V1 must add `parkedOnBackgroundWork` to the registry + profiles.
- **`probe_windows.go` is already a stub returning `unknown`** → V1 gets "Windows renders as
  today" for free; a real Windows walk is the only true greenfield probe work (Windows slice).

## Per-slice reuse matrix

Legend: **HARVEST** = cherry-pick ~as-is · **EXTRACT** = pull a subset out of the monolithic
file (the hard part) · **NEW** = does not exist on the branch · **DEFER** = on the branch but
belongs to a later slice, leave it out of this one.

### V1 `parked-board-mvp` (Darwin+Linux, one-shot, board) — ~90% harvest

| Action | Files / work |
|---|---|
| **HARVEST** | All probe files (both Unix + Windows stub) + tests; transport (`client_probe.go`, `manager_probe.go`, `agent_probe.go`); `dto/parked.go`; `background_launch_recognizer.go` (Claude inline + `Kind==shell`); `parked_architecture_test.go` (AC-35); board frontend (`kanban-card-content.tsx`, `kanban-card.tsx`, `state-icons.tsx` board bits, `lib/kanban/parked-projection.ts`, `kanban-parked` WS handler, `hydrator-kanban-parked*`, `use-all-workflow-snapshots-parked`, board e2e spec) |
| **EXTRACT** | `parked_state.go` / `parked_projection.go` → take **only** the three-term formula + settle hook + clear-on-`RUNNING`. **Leave out** the sampler loop, eviction, tombstone, epoch/revision. *This is V1's one real engineering task.* |
| **NEW** | Runtime flag (`runtimeflags/registry.go` + `profiles.yaml`, off everywhere) + disabled-path tests; clear-on-observed-`RUNNING` (substitutes for the deferred `turn_started`); resolve the turn-stamp bypass; test debt — zombie-*descendant* (AC-27a), §L-shaped trees (AC-70/70a), fix scrambled AC labels in `probe_unix_test.go` |
| **DEFER** | `turn_started_emission_test.go` (→V5); `parked_eviction_ordering_test.go` (→V2/V3); `session-parked-merge.ts`, `task-parked-merge.ts`, `tasks-parked*` (→V3/V4) |

### V2 `parked-live-sampling`
**EXTRACT** the sampler-loop half of `parked_state.go`/`parked_projection.go` +
`parked_eviction_ordering_test.go` (sampler/eviction parts). Freeze the sampler shutdown state
diagram. Mostly harvest-by-extraction.

### V3 `parked-projection-consistency`
**EXTRACT** the epoch/revision/tombstone/multi-session half of the projection; **HARVEST**
`session-parked-merge.ts`, `task-parked-merge.ts`, `tasks-parked*` (the merge/revision consumer).
Freeze lock order + `(epoch,revision)` function + tombstone retention.

### V4 `parked-everywhere`
**HARVEST** the non-board frontend already on the branch: full `state-icons.tsx` resolvers,
sidebar `task-item.tsx`, session switcher, `/tasks` row, mobile, tooltips, pseudo-locale.

### V5 `parked-turn-boundary-and-seam`
**HARVEST** `turn_started_emission_test.go` + the `turn_started` event + the public-registry
half of `background_launch_recognizer.go`; contract amendment; flag graduation.

### Windows slice
**NEW** — replace the `probe_windows.go` stub with a real walk (the only greenfield probe).

## Reuse mechanics (how a slice harvests)

Each slice branches from **`origin/main`** (clean) and pulls files from the monolith branch:

```bash
git checkout -b feature/parked-v1-board origin/main
git checkout feature/waiting-attribution-hxr -- <harvest paths…>
# then EXTRACT: hand-edit parked_state.go / parked_projection.go down to the one-shot subset
# then NEW: add the flag, clear-on-RUNNING, tests
```

So the monolith branch must **stay alive as the harvest source** until the slices have taken
what they need — which drives the MR decision below.

## The current MR — PR #2476

**State:** OPEN · **not Draft** · **MERGEABLE** · base `main` · head
`feature/waiting-attribution-hxr` on nova28's fork · 23 pushed commits · +12,094/−358 · 114
files. (Local worktree is further ahead — 46 commits incl. uncommitted round-14 fixes — so the
PR is a slightly older snapshot of the same monolith.)

**Problem:** it is the non-converging monolith, and it is **one click from merge** (mergeable +
not draft). It must not merge as-is — but its branch is the harvest source for every slice.

**Recommendation (least-destructive):**

1. **Immediately convert #2476 to Draft and retitle** →
   `[DO NOT MERGE] waiting-attribution monolith — harvest source for sliced delivery`. Removes
   the accidental-merge risk without losing anything. *(Requires modifying the PR — needs your
   go-ahead; I won't touch a public PR without it.)*
2. **Keep the branch as the harvest source.** Do not delete it while slices are being cut.
3. **Cut each slice as its own branch off `main` → its own small PR**, harvesting from the
   monolith branch per the matrix above.
4. **Close #2476 unmerged once V1 (and ideally V2–V5) have harvested** what they need; then delete
   the branch and clean the worktree (kandev leaves worktrees behind — see kandev Rule 3 cleanup).

**Rejected alternatives:** merging #2476 (it is the thing that didn't converge); force-resetting
#2476 down to just V1 (throws away the harvest history and needs V1 built before the PR is safe);
deleting the branch now (destroys the harvest source).

## Proposed Kandev sub-tasks (via /ksdd:kandev, queued)

Subtasks of `e73b0020` (inherit workspace/workflow/agent/executor), `start_agent: false`,
`workspace_mode: new_workspace`, `base_branch: main`. One per slice; V1's prompt carries the full
harvest+extract+new list above, V2–V5/Windows carry scope + "spec via /spec before start".

| Task | Ships | Start posture |
|---|---|---|
| `parked V1 — board MVP (Darwin+Linux, one-shot)` | the ability, on the board | queued; spec ready (`parked-board-mvp-spec.md`) |
| `parked V2 — live sampling` | self-clearing affordance | queued; spec TBD |
| `parked V3 — projection consistency` | multi-session/restart correctness | queued; spec TBD |
| `parked V4 — all surfaces` | sidebar/tasks/switcher/mobile | queued; spec TBD |
| `parked V5 — turn boundary + vendor seam` | precise clearing + registry + graduation | queued; spec TBD |
| `parked — Windows probe` | Windows support | queued; spec TBD (only greenfield probe) |
