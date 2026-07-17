---
id: "05-rewrite-agent-stats-plugin"
title: "Rewrite kandev-plugin-agent-stats to use the SDK data API"
status: pending
wave: 4
depends_on: ["03-sdk-data-accessors", "04-host-data-impl"]
plan: "plan.md"
spec: "../../../specs/plugins/spec.md"
adr: "../../../decisions/0043-plugin-host-data-api.md"
---

# Task 05: Rewrite kandev-plugin-agent-stats to use the SDK data API

The proof the carrot is sufficient: replace the agent-stats plugin's direct SQLite
access with the Host data API. This is a **separate repository** at
`/home/jcfs/.kandev/tasks/tokens-per-task-loc_9tk/kandev-plugin-agent-stats/` — do
not vendor it into the kandev repo; reference it.

## Scope
- Delete `readSessions` + `sessionsQuery` and the `modernc.org/sqlite` dependency
  from `server/stats.go`. Stop opening `~/.kandev/data/kandev.db`.
- Fetch sessions via `host.Sessions().List(...)` and per-session LOC via
  `host.Sessions().CodeStats(...)`. Map the returned `Session` +
  `SessionCodeStats` into the existing `sessionRow` shape (agent display, model,
  `acp_session_id`, committed + peak-pending lines).
- Keep `effectiveLines` semantics (max of committed and peak-pending) on the DTO
  fields; keep the tokscale join on `acp_session_id` unchanged (external tool,
  unaffected).
- Declare `capabilities.api_read: ["sessions"]` in the plugin `manifest.yaml`.
- Remove the `KANDEV_AGENT_STATS_DB` config path (no DB path needed); keep the
  tokscale override.

## Acceptance
- The plugin no longer imports a SQLite driver or opens any kandev DB file.
- Stats report still renders sessions + LOC + tokscale ratios using data obtained
  solely through the Host data API.
- `manifest.yaml` declares `api_read: ["sessions"]`.

## Verification
- In the plugin repo: `go build ./...` and its existing `go test ./...`.

## Files likely touched (separate repo)
- `.../kandev-plugin-agent-stats/server/stats.go`
- `.../kandev-plugin-agent-stats/server/plugin.go`
- `.../kandev-plugin-agent-stats/manifest.yaml`
- `.../kandev-plugin-agent-stats/go.mod` (drop sqlite dep)

## Inputs
- SDK data accessors from task-03; live host impl from task-04.
- Current plugin: `server/stats.go` (`sessionRow`, `aggregate`, `effectiveLines`),
  `server/plugin.go` (`loadConfig`, `collect`).

## Dependencies
Tasks 03, 04.

## Output contract
Summary, confirmation the SQLite import is gone, mapping notes (DTO →
`sessionRow`), build/test result, and status update here + in `plan.md`.
</content>
