---
status: draft
created: 2026-05-12
owner: cfl
---

# Office Costs — Disk Runner for Providers Without ACP Usage

## Why

The `office-costs` spec covers cost ingestion for ACP CLIs that emit token usage on the wire (claude-acp, opencode-acp, gemini). Two large gaps remain:

- **codex-acp** is recorded with `estimated=true` because the ACP wire only carries a cumulative `usage_update.used` counter — no per-turn input/output/cached split. Empirical inspection of codex-acp's rollout file (`~/.codex/sessions/.../rollout-*.jsonl`) shows a `token_count` event_msg per turn with the full OpenAI-style split (`input_tokens`, `cached_input_tokens`, `output_tokens`, `reasoning_output_tokens`). Accurate cost is available *on disk*, just not on the wire.
- **amp-acp** emits nothing on the wire and is not tracked at all today. Its threads JSON (`~/.local/share/amp/threads/*.json`) carries full per-turn usage plus a `credits` field — the same data `@ccusage/amp` already parses.

We don't want to own session-file parsers in our repo. Format drift is the most-churning surface in this space (OpenCode migrated JSON→SQLite mid-2025; Codex's schema has evolved across versions). The `@ccusage/*` family already maintains these parsers, ships per-provider as pinned npm packages, supports `--json` output, and groups by the same session ID we record as `TaskSession.ACPSessionID`.

This spec introduces a small binary that wraps pinned `@ccusage/*` packages, reads their JSON output, and feeds cost events into the existing `office_cost_events` pipeline for providers where ACP wire data is missing or incomplete.

## What

### A new binary: `cmd/usage-runner`

- Standalone Go binary built into the backend Docker image alongside `kandev` and `agentctl`.
- Invoked by the office service after `session/complete` events for relevant providers, and periodically (every 60s) to catch sessions completed during backend downtime.
- Spawns the appropriate pinned `@ccusage/<provider>` subprocess via `npx -y @ccusage/<provider>@<pinned-version> session --json`, reads stdout, and emits normalized `CostEvent` records.

### Provider coverage

- **codex** — disk runner is the preferred source. Cost events promoted from `estimated=true` to `estimated=false`, with the full token split. The existing cumulative-delta wire path remains as a fallback when the disk runner can't run (Node missing, npm subprocess failure, file not yet flushed).
- **amp** — disk runner is the only source. Cost events emitted with `estimated=false`. The Amp `credits` field is captured alongside USD cost in a new `provider_credits` column.
- **claude, opencode, gemini** — disk runner is NOT used. Their ACP wire data is already authoritative; running ccusage against them would be redundant work and could disagree with provider-reported cost.
- **auggie, copilot** — out of scope (no `@ccusage/*` package exists for either; copilot's billing model is request-multiplier-based, not token-based).

### Join key

`TaskSession.ACPSessionID == ccusage.sessionId`. Verified empirically against current versions of codex-acp and opencode-acp:

- codex: `session_meta.id` in rollout JSONL equals the ACP `session/new` sessionId equals the rollout filename suffix.
- amp: thread JSON top-level `id` equals the agent's session id.

No injection, no PID/mtime correlation, no fsnotify watcher needed.

### ccusage version policy — track `@latest`

- The runner spawns `npx -y @ccusage/<provider>@latest session --json`. No version pin in our repo.
- The `-y` flag auto-accepts npx's "Need to install package X, OK to proceed?" prompt, so the first spawn (cold npm cache) is non-interactive.
- Trade-off: tracking `@latest` means an upstream breaking change in ccusage's `--json` output can silently break cost ingestion without a code change on our side. Mitigated by:
  - Schema-validating ccusage's JSON output at decode time; on schema failure, the runner returns an error, the office subscriber falls back to the wire-side estimated rows for codex, and amp coverage temporarily degrades to untracked. The backend does not crash.
  - A nightly CI smoke job that runs the runner against committed fixture inputs (a fake `HOME` with synthetic rollout / threads files) and asserts the JSON contract still holds. A failure here notifies maintainers within 24h.
- Rationale for not pinning: ccusage's per-provider packages release frequently with parser fixes for agent CLI updates. Pinning means every agent-CLI update triggers a code review cycle for a one-line version bump. Tracking `@latest` keeps Kandev current with zero deliberate maintenance.

### Idempotency

- ccusage's `session --json` rolls all per-turn events up into a single summary per `(session_id, model)`. The runner therefore emits one cost row per session-model pair, not per turn.
- `provider_event_id` is `ccusage:<provider>:<session_id>:<model>` — deterministic from the aggregate row's identity.
- Cost events are upserted by `(session_id, provider_event_id)`. Re-running over a session **replaces** its row(s) with the newer totals; no growth, no duplicates.
- Coexistence with the wire-side per-turn rows from `office-costs`: rows are stored side by side. Wire rows have `provider_event_id IS NULL`, disk-runner rows have non-NULL. Sum queries are unaffected; the cost-explorer aggregations don't care about granularity. For codex specifically, the disk runner deletes the wire-side `estimated=true` rows for the same session after a successful aggregate row lands, to avoid double-counting against budgets.

### Failure modes

- If `npx` is unavailable on the host, the runner logs a one-time warning per session and falls back silently. Codex sessions retain their estimated rows; amp sessions remain untracked. The backend does not crash.
- If ccusage's `--json` output shape changes upstream, the runner's schema validator returns a decode error. The office subscriber treats the run as no-op (no rows touched); codex rows remain on the wire-side estimated path, amp rows are absent. The nightly fixture-smoke CI alerts maintainers.
- If `@ccusage/<provider>@latest` is yanked from npm, the next runner invocation fails. Backend continues; coverage degrades the same way as a parse failure.

### Performance

- The runner is bounded: one ccusage spawn per provider per invocation, not one per session. Spawn cost ~300–600ms; runs amortize across all completed sessions for that provider.
- Runner invocations are coalesced — if a `session/complete` fires while a runner is already executing for that provider, the second invocation is dropped.
- The 60s periodic sweep catches sessions that completed during a coalesced gap or a backend restart.

## Scenarios

- **GIVEN** a completed codex-acp session that used a single model, **WHEN** the disk runner executes, **THEN** one cost row exists for that session — input/cached_input/output/reasoning summed across the session, `estimated=false`, `provider_event_id="ccusage:codex:<sessionId>:<model>"`. The wire-side `estimated=true` rows previously emitted for that session are deleted. Re-running the runner replaces the row with newer totals; row count remains 1.

- **GIVEN** a codex session that used two models (e.g. model switched mid-session), **WHEN** the disk runner executes, **THEN** two rows exist — one per `(session, model)` pair — each with its own totals and `provider_event_id`.

- **GIVEN** a completed amp-acp session, **WHEN** the disk runner executes, **THEN** one cost row per model exists for the session including the amp `credits` value alongside USD cost. The agent appears in the cost explorer where it was previously absent.

- **GIVEN** Node/`npx` is not available on the host, **WHEN** the disk runner attempts to spawn ccusage, **THEN** it logs one warning per provider per process lifetime, marks the run as skipped, and the backend continues normally. Codex sessions retain their wire-side estimated rows; amp sessions remain untracked.

- **GIVEN** a pinned `@ccusage/codex` version is bumped, **WHEN** CI runs, **THEN** the recorded-fixture test asserts the new version's `--json` output still matches the expected shape. If the shape changed, CI fails before merge.

- **GIVEN** a codex session that completed during a backend restart, **WHEN** the 60s periodic sweep runs after backend startup, **THEN** the runner discovers the rollout file via ccusage's normal scan, emits cost rows, and the session shows accurate cost in the explorer.

## Out of scope

- Auggie support — no `@ccusage/*` package exists.
- Copilot support — billing is request-multiplier-based, not token-based; separate scope from this spec.
- Retroactive ingestion of sessions from before this feature ships (cost rows exist with `estimated=true` from the wire path; not rewritten).
- Replacing the wire-side ingestion for claude-acp / opencode-acp / gemini. Their wire data is authoritative.
- Subscription utilization tracking (covered in `subscription-usage` spec).
- A user-facing UI surface for this binary. It's an internal subsystem; visibility flows through the existing cost explorer.

## Open questions

- **Node/npx in the backend container**: the kandev backend image currently does not include Node. We need to add it (or run the disk runner as a sidecar that does). Adding ~50MB to the image is acceptable; an `apt-get install nodejs npm` is the simplest path. Alternative: bundle ccusage as a single-file build via `bun build --compile` per provider, eliminating the Node dependency at runtime.
- **Future providers**: when a new agent CLI lands (e.g., a hypothetical `@ccusage/auggie` or new `@ccusage/copilot`), this binary should accept new provider plugins via a registry, not require new code per provider. The plan should define the plugin shape.
- **Tokscale as alternative wrapper** (considered, deferred):
  [tokscale](https://github.com/junhoyeo/tokscale) is a single Rust binary that natively parses sessions for 20+ agent CLIs — including codex and amp, plus Goose, Kimi, Qwen, Roo Code, Crush, Zed, and several others that have no `@ccusage/*` counterpart. One subprocess vs N. Actively maintained (v2.1.1, 2026-05-10).

  **Blocker**: its CLI does not expose per-session output today. The `GroupBy` enum at `crates/tokscale-core/src/lib.rs:100` exposes only `Model`, `ClientModel`, `ClientProviderModel`, `WorkspaceModel`. Internally, `UnifiedMessage` carries `session_id` (used for dedup at `sessions/mod.rs:34`), so the data is present but never aggregated or serialized by session.

  **Contribution shape** (~150–250 LOC PR):
  - Add `Session` and `ClientSession` to `GroupBy` enum.
  - Add `aggregate_by_session()` in `aggregator.rs` modeled on the existing `aggregate_by_date()` (line 14, ~971 LOC file).
  - Wire `--group-by session,model` through `tokscale-cli` and the JSON output schema.
  - Tests for both aggregator function and CLI output.

  **Decision for v1**: stay with `@ccusage/*` because per-session output is already shipped. Revisit tokscale when (a) we need to track a provider tokscale supports natively that ccusage doesn't (Kimi, Goose, Qwen, etc.), or (b) the session-grouping contribution lands upstream. If we switch later, the runner's `Entry` shape doesn't change — only the subprocess command and decoder do.
