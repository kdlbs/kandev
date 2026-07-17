---
id: "07-docs"
title: "Author docs: Host data API + manifest api_read/api_write resource list"
status: pending
wave: 4
depends_on: ["04-host-data-impl"]
plan: "plan.md"
spec: "../../../specs/plugins/spec.md"
adr: "../../../decisions/0042-plugin-host-data-api.md"
---

# Task 07: Author docs for the Host data API

Document the Host data API for plugin authors: which resources are readable, the
`api_read`/`api_write` capability vocabulary, and the SDK accessor surface.

## Scope
- **Wire reference (`docs/plans/plugins/GRPC-CONTRACT.md`):** add the Host data
  RPCs to the `service Host` block (§3) and note capability gating uses
  `api_read:<resource>` / `api_write:<resource>` (§5), replacing the "reserved"
  language. Note write RPCs are deferred (`Unimplemented`).
- **SDK reference (`docs/plans/plugins/GRPC-CONTRACT.md` §4 and/or
  `PLUGIN-API.md`):** document the author accessors (`host.Tasks().List(...)`,
  `host.Sessions().List(...)`, `host.Sessions().CodeStats(...)`, etc.) and that
  they return Go-native structs.
- **Manifest reference:** wherever the manifest `capabilities` block is documented
  for authors, list the `api_read` / `api_write` resource vocabulary (`tasks`,
  `sessions`, `workspaces`, `workflows`, `agents`, `repositories`, `comments`) and
  what each unlocks, with the `api_read: ["sessions"]` example from the agent-stats
  plugin.
- **Public docs:** if a user-facing plugin authoring page exists under
  `docs/public/**` at implementation time, add the resource list there too; there
  is none today, so the authoritative author docs are the plans references above.
  Do not invent a new public page in this task.

## Acceptance
- `GRPC-CONTRACT.md` lists the Host data RPCs and the per-resource capability
  gating; the "reserved / not implemented" wording for `api_read`/`api_write` is
  removed or scoped to the deferred write RPCs.
- The manifest capability vocabulary is documented with the readable-resource
  list and an example.

## Verification
- Markdown-only; no build. `cd apps && pnpm --filter @kandev/web lint` only if a
  `docs/public` page is touched (none expected).

## Files likely touched
- `docs/plans/plugins/GRPC-CONTRACT.md`
- `docs/plans/plugins/PLUGIN-API.md` (if the SDK/author surface is documented
  there)

## Inputs
- Final RPC set + accessor names from tasks 01/03/04.
- Spec: "Host data API"; ADR 0042.

## Dependencies
Task 04 (behavior finalized).

## Output contract
Summary, files changed, and status update here + in `plan.md`.
</content>
