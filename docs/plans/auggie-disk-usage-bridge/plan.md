---
status: done
spec: docs/specs/office/costs.md
created: 2026-08-11
---

# Plan: Auggie disk usage bridge

## Goal

On Auggie stream complete with nil wire usage, read new completed exchanges from `~/.augment/sessions/<acpSessionId>.json`, watermark `sequenceId`, and publish `session_prompt_usage.updated.<sessionId>` so Kandy Token Grotto and Office costs see real tokens.

## Approach (locked)

Host disk bridge (Approach A). Wire-wins when `payload.Data.Usage != nil`. v1 Local + Worktree only.

## Work

| Wave | Task | Status |
|------|------|--------|
| 1 | Spec amend (`costs.md`) | done |
| 2 | Private reader + unit tests | done |
| 3 | Complete-path bridge + watermark + tests | done |
| 4 | Targeted backend verification | done |

## Files

| Path | Change |
|------|--------|
| `docs/specs/office/costs.md` | Auggie disk bridge contract |
| `apps/backend/internal/agent/auggieusage/` | Reader, types, fixtures, tests |
| `apps/backend/internal/task/models/models.go` | `SessionMetaKeyAuggieUsageSeq` |
| `apps/backend/internal/orchestrator/event_handlers_streaming.go` | Bridge on complete |
| New orchestrator test file | Bus event + watermark + wire-wins |

Note: `internal/agent/usage` is subscription billing; keep Auggie session-file reader in `auggieusage` to avoid package collision.

## Design notes

- Reader: `ReadNewExchangeUsages(path, afterSeq) ([]ExchangeUsage, error)` — privacy-safe structs only.
- Home: `filepath.Join(userHome, ".augment", "sessions", acpID+".json")`.
- ACP id: metadata `acp.session_id`, else `payload.Data.ACPSessionID`, else empty skip.
- One publish per complete with summed deltas; model = last exchange model_id.
- Optional deferred re-read: short `time.AfterFunc` if file missing at complete; do not block handler.
- Soft-fail on I/O/JSON errors with warn log (no content).

## Verification

```bash
cd apps/backend && go test ./internal/agent/auggieusage/ ./internal/orchestrator/ -count=1 -run 'Auggie|PromptUsage|auggie'
```

## Out of scope

Plugin walk UI; Copilot; Docker/SSH/Sprites; XP fakes; plugin label rename.
