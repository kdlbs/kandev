# Cost Tracking — Provider Overview

Cross-reference of what each agent CLI exposes, where Kandev gets it from, and what coverage looks like after the two specs in this family ship.

Specs:
- [`office-costs`](../office-costs/spec.md) — ACP-wire ingestion (live, in-progress)
- [`office-costs-disk-runner`](./spec.md) — disk fallback via pinned `@ccusage/*` wrappers (draft)
- [`subscription-usage`](../subscription-usage/spec.md) — OAuth-based utilization for subscription plans (draft)

## Per-provider matrix

Verified empirically against real CLIs on 2026-05-12 via `apps/backend/bin/acpdbg prompt …`.

| Provider | ACP wire — tokens | ACP wire — cost | Disk fallback | Subscription utilization | Coverage when both specs ship |
|---|---|---|---|---|---|
| **claude-acp** | ✅ `result.usage` — in / out / cachedRead / cachedWrite | ✅ `usage_update.cost.amount` (USD) | not used | ✅ `api.anthropic.com/api/oauth/usage` (5h + 7d windows) | **Accurate** — provider-reported cost |
| **opencode-acp** | ✅ `result.usage` — in / out / thoughtTokens | ⚠️ `usage_update.cost.amount` (often `0` on BYOK) | not used | ❌ | **Accurate** — provider-reported cost when present, else models.dev × tokens |
| **gemini** | ✅ `result._meta.quota.token_count` — in / out only | ❌ | not used | ❌ | **Accurate** — models.dev × tokens, no cache split |
| **codex-acp** | ⚠️ cumulative `usage_update.used` only — no per-turn split | ❌ | ✅ `~/.codex/sessions/.../rollout-*.jsonl` `token_count` event_msg with full split (input / cached_input / output / reasoning) via `@ccusage/codex` | ✅ `chatgpt.com/backend-api/wham/usage` headers | **Accurate after disk-runner** (today: estimated) |
| **amp-acp** | ❌ | ❌ | ✅ `~/.local/share/amp/threads/*.json` per-turn + `credits` field via `@ccusage/amp` | ❌ | **Accurate after disk-runner** (today: untracked) |
| **copilot-acp** | ⚠️ static `_meta.copilotUsage:"1x"` per-model multiplier (not a turn count) | ❌ | ❌ no `@ccusage/copilot` package; request-multiplier billing model | ❌ | **Untracked** — out of scope |
| **auggie** | ❌ | ❌ | ❌ no `@ccusage/auggie` package | ❌ | **Untracked** — out of scope |

## Join key

For every provider where Kandev tracks cost, the join is:

```
TaskSession.ACPSessionID  ==  agent's own session id  ==  ccusage's session id
```

Verified per provider:

| Provider | `session/new` returns | Filesystem evidence | Mapping |
|---|---|---|---|
| claude-acp | `e8295054-5c5b-4e45-bcaf-6f90fcb54395` | `e8295054-5c5b-4e45-bcaf-6f90fcb54395.jsonl` | filename = sessionId |
| codex-acp | `019e1e27-af49-7cd1-89b7-7bad1c3f3be2` | `rollout-2026-05-12T22-46-17-019e1e27-af49-7cd1-89b7-7bad1c3f3be2.jsonl` (= `session_meta.id` inside the file) | sessionId is filename suffix and the `session_meta.id` key |
| opencode-acp | `ses_1e1e3f0b6ffeGLI5Os07YdHnj6` | `ses_1e1e3f0b6ffeGLI5Os07YdHnj6.json` (legacy) + same as PK in `opencode.db` | filename / PK = sessionId |
| amp-acp | thread id | top-level `id` in thread JSON | payload `id` = sessionId |

No injection of our session ID into the agent's invocation is required.

## Cost resolution layers (priority order)

For each cost event, the system picks the first source that yields a value:

1. **Provider-reported cost on wire** — claude-acp always; opencode-acp sometimes. Stored as `int64(amount * 10000)` subcents. Authoritative; pricing lookup skipped. Granularity: per-turn.
2. **Disk-runner per-session aggregate** (codex, amp only) — one row per `(session, model)` from ccusage's `session --json`. Replaces wire-side estimated rows for codex; only source for amp. Pricing via ccusage's own LiteLLM integration or `models.dev` fallback.
3. **models.dev × wire tokens** — gemini, opencode-acp on BYOK, codex while disk runner is unavailable. Granularity: per-turn.
4. **Cumulative-delta estimation** (codex-acp only, wire path) — flagged `estimated=true`. Replaced by layer 2 when the disk runner runs.
5. **No coverage** — `cost_subcents=0`, `estimated=true`. UI shows "pricing unavailable".

The `office_cost_events` table holds both per-turn rows (layers 1, 3, 4) and per-session aggregate rows (layer 2). They coexist; sum queries treat them uniformly because each row's `cost_subcents` is correct at its own granularity. Rows are distinguished by `provider_event_id IS NULL` (per-turn) vs non-NULL (aggregate) for queries that care about granularity.

## Subscription vs API key (orthogonal)

Subscription detection is driven by credential presence on the `AgentProfile` (not by which provider). Per the `subscription-usage` spec:

| Auth mode | What "cost" means in UI |
|---|---|
| API key (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, etc.) | Actual dollar spend — `$X.XX` |
| Subscription (OAuth `~/.claude/.credentials.json`, Codex `~/.config/codex/auth.json`) | Utilization gauges per window + `~$X.XX est.` equivalent for ranking |

Only claude and codex have public usage APIs we can poll for subscription utilization. Other providers' subscription users see estimated dollars but no utilization gauges.

## Caveats worth flagging when sharing

- **"Cost" in Kandev is always an estimate** unless the agent itself reports it. The same dollar number can come from three different sources (provider, models.dev lookup, or unavailable). The `estimated` flag on each row distinguishes them.
- **Copilot is intentionally untracked.** Its billing is "premium requests × per-model multiplier" against the user's monthly plan, not tokens. Modeling it requires a separate accounting model that nothing in this family of specs addresses.
- **Auggie is untracked** until someone writes an `@ccusage/auggie` parser or Auggie publishes a usage API. We don't plan to write one.
- **models.dev** is community-curated. New models may be missing for days after release. Affected rows show `cost_subcents=0` with "pricing unavailable" until the dataset catches up.
- **Codex's per-turn split currently lands as estimated** because the ACP wire only gives cumulative bytes. The disk runner promotes those rows to accurate; until the disk runner ships, codex spend numbers should be read as best-effort.
