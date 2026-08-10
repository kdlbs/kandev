# parked-board-mvp — seed branch

This branch (`feature/parked-v1-seed`) is **scaffolding for the V1 slice task**, not a
merge candidate. It was committed with `--no-verify` because the seeded probe package is
**dead code until V1 wires it** (the `unused` linter flags it in isolation — expected).

What is here:
- `docs/specs/parked-board-mvp/spec.md` — the frozen V1 contract.
- `docs/plans/parked-board-mvp/reuse-map.md` — harvest / extract / new / defer map.
- `docs/plans/parked-board-mvp/split-proposal.md` — full context (v4).
- `apps/backend/internal/agentctl/server/process/probe*.go` — the platform-independent
  probe **implementation**, harvested from `feature/waiting-attribution-hxr` (compiles on
  main; unused until wired).

What is NOT here (interleaves with edits to existing files — harvest per the reuse map,
bringing each file's companion edits so it compiles):
- probe **tests** (need `AgentPID` on `process.Manager`, which needs
  `adapter.Config.RecordTurnStart`)
- transport, dto/parked.go, the orchestrator projection (extract the one-shot subset),
  and the board frontend.
- The parent spec with full AC text lives at
  `feature/waiting-attribution-hxr:docs/specs/disambiguate-waiting/spec.md`.
