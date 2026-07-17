---
id: "07-docs"
title: "Author docs: Host data API + manifest api_read/api_write resource list"
status: done
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

## Output

**Summary:** Documented the Host data API (ADR 0042) in the wire/SDK contract
reference and reconciled the spec section against the shipped code.

**Files changed:**
- `docs/plans/plugins/GRPC-CONTRACT.md` — added the 9 read RPCs + 3 deferred
  write RPCs to the `service Host` block (§3); added a new "§3a. Host data API
  (ADR 0042)" subsection covering the resource/capability table, deferred
  writes, service-layer/DTO conventions, pagination/timestamp/nullable/scoping
  conventions; added the `Host` interface's 6 data accessors plus the 6 reader
  interfaces (`TaskReader`, `SessionReader`, `WorkspaceReader`,
  `WorkflowReader`, `AgentProfileReader`, `RepositoryReader`) with their real Go
  signatures, and an authoring example (manifest snippet + `Sessions().CodeStats`
  call) to §4; rewrote the §5 "Capability gating" bullet to remove the stale
  "api_read/api_write reserved" wording and describe the real per-RPC gating
  plus the write RPCs' current `Unimplemented` behavior.
- `docs/specs/plugins/spec.md` — fixed capability-vocabulary drift against code
  (see below) and added a "Declaring data access" note under the manifest
  capabilities section; reworded the interceptor description to match the
  actual per-RPC inline checks (no shared unary interceptor exists in code);
  updated the readable-resources table's `ListAgentProfiles` row.
- `docs/plans/plugins/host-data-api/plan.md` — marked task-07 done in the wave
  list.

**Code-vs-doc drift found and corrected (spec.md only; code is source of
truth, left unchanged):**
- The `ListAgentProfiles` capability is `api_read:agent_profiles` in code
  (`internal/plugins/host_data.go: resourceAgentProfiles = "agent_profiles"`,
  confirmed by `host_data_test.go`), not `api_read:agents` as the spec had it
  in the manifest example, the resource-vocabulary sentence, and the readable-
  resources table. Fixed all three.
- The spec's manifest example listed `"projects"` as an `api_read` resource;
  there is no `ListProjects` RPC, resource constant, or reader anywhere in the
  proto or `internal/plugins/host_data.go`. Removed it from the example.
- The spec described `api_read`/`api_write` as "reserved for future Host RPCs"
  in the manifest example comments and said the capability-gating interceptor
  covers "state, secrets; api_read/api_write reserved" — both predate this
  ADR shipping; `api_read` is live now (write RPCs remain deferred/
  `Unimplemented`). Reworded both.
- The spec attributed capability gating to "a unary server interceptor on
  Host"; there is no `grpc.UnaryServerInterceptor` in `internal/plugins` or
  `pkg/pluginsdk` — each Host/Host-data method checks its own capability
  inline at the top of the handler (`host.go`, `host_data.go`). Reworded to
  describe the actual per-RPC check without implying a shared interceptor
  layer (this was pre-existing drift, unrelated to ADR 0042, caught while
  editing the same paragraph).
- `GRPC-CONTRACT.md`'s pre-ADR text already read "api_read/api_write reserved
  for future" in §5 and the `service Host` proto snippet only listed the 6
  pre-ADR RPCs; both were stale relative to the shipped proto and are now
  updated in place.

**Not touched (out of scope / already correct):**
- `docs/decisions/0042-plugin-host-data-api.md` — read for context per the
  task's Inputs; its `Status: proposed` front matter looks stale given the
  code is fully implemented, but the task's edit scope was spec.md, not the
  ADR, so it is left for whoever finalizes the ADR's status.
- `docs/plans/plugins/PLUGIN-API.md` — this is the native frontend (JS) UI
  plugin contract; it has no backend Host/SDK content and the Host data API
  has no UI surface, so nothing there needed updating. The SDK reference went
  into `GRPC-CONTRACT.md` §4 per the task's "and/or" framing.
- `docs/public/plugins*.md` — do not exist on this branch (confirmed); per the
  task instructions these live on `feature/kandev-plugin-system-docs` and are
  out of scope here.
- `apps/backend/internal/plugins/manifest/manifest_test.go` has an unrelated
  test fixture using `api_read: ["tasks", "agents"]` as a manifest capability
  example — manifest capability values are freeform strings with no vocabulary
  validation at that layer (confirmed: no enum/allow-list check in
  `internal/plugins/manifest`), so this is a test fixture choice, not a
  functional bug; flagged here as a code-side echo of the same naming drift,
  left unchanged since it's code, not docs.
</content>
