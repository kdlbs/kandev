---
status: completed
spec: docs/specs/ui/app-status-bar.md
---

# App Status Bar — implementation plan

## Outcome

Ship one app-owned status surface: 24 px in-flow bar on desktop/tablet, native Status drawer on phone. It shows only connection plus existing opted-in host/active-executor metrics, and exposes live plugin regions without changing chat-local status UI.

## Fixed decisions

- Slots: `app-status-bar-left`, `app-status-bar-right`; exact props live in [spec](../../specs/ui/app-status-bar.md#plugin-slots) and [plugin API](../plugins/PLUGIN-API.md#app-status-bar-slots).
- One responsive presentation mounts at once. Desktop=`full`, tablet=`compact`, drawer=`full`.
- Metrics subscribe only when legacy `showInTopbar` preference permits: desktop/tablet while bar mounted, phone only while drawer open.
- Shell uses an in-flow footer. `--app-status-bar-height` offsets audited desktop fixed overlays only.
- Orca is conceptual reference only. Ship pinned MIT attribution; do not transplant its component or add data providers.

## Waves

| Wave | Tasks | Gate |
|---|---|---|
| 1 | [01](task-01-plugin-status-slots.md), [02](task-02-status-builtins.md) | Stable slot identity; built-in metric/connection primitives tested. |
| 2 | [03](task-03-status-surface-shell.md) | One 24 px shell-mounted desktop/tablet surface; header metrics removed. |
| 3 | [04](task-04-mobile-status-entry-points.md), [05](task-05-fixed-overlay-audit.md), [06](task-06-plugin-fixture-desktop-e2e.md), [08](task-08-attribution-public-docs.md) | Phone access, fixed controls, live plugin E2E, records/attribution land. |
| 4 | [07](task-07-mobile-status-e2e.md) | Pixel 5 flow proves drawer parity and geometry. |
| 5 | [09](task-09-integration-verification.md) | Focused then full verification clean. |

## Dependency graph

```text
01 plugin slots ─┐
                 ├─> 03 status surface + shell ─┬─> 04 mobile entry points ─┐
02 built-ins ────┘                              ├─> 05 fixed-overlay audit   ├─> 07 mobile E2E ─> 09 verify
                                                ├─> 06 fixture + desktop E2E ─┘
                                                └─> 08 attribution/docs ──────> 09 verify
```

## File / test map

- **Plugin contract:** typed status props, stable owned registrations, owner-aware error boundaries, typed left/right wrapper, unit tests.
- **First-party surface:** connection item; status metrics presentation; app bar/drawer/provider; remove `TopbarMetrics` mounts and update settings copy.
- **Shell and geometry:** app/legacy shells, shell-owned route heights, audited fixed controls, explicit z-index and CSS variable offsets.
- **Mobile:** native Status actions in Home, task, Settings, PageTopbar, Office; no second footer.
- **Proof:** fixture plugin hot enable/disable desktop test; `mobile-status-drawer.spec.ts`; layout/overlay assertions; licenses-page test.
- **Records:** public plugin/operations/features docs, resource-metrics decision amendment, MIT notice generation.

## Verification order

Run focused task commands after each wave. E2E must rebuild production assets (`make build-web`, and `make build-backend` if Go changes) or use `make test-e2e`; never run against stale output.

Final order:

```sh
make fmt
make typecheck test lint
```

Also run `node --test scripts/validate-public-docs.test.mjs` and `node scripts/validate-public-docs.mjs`. Resolve viewport regressions at narrow desktop, tablet, and configured `mobile-chrome` Pixel 5 before declaring ready.

## Risks

- Nested viewport roots can create scrollbars: convert only shell-owned routes to `h-full min-h-0` and retain `h-dvh` for genuine phone sheets/dialogs.
- A fixed footer does not move fixed overlays: use one CSS variable only at audited controls.
- Responsive double mounting can duplicate plugins/metrics: provider selects exactly one host presentation.
- Array-index error-boundary keys leak failure state: stable registration IDs are mandatory.
- Full-bleed plugin routes intentionally opt out of host chrome; their authors own Status trigger placement.

## Tasks

| ID | Status | Wave | Task |
|---|---|---:|---|
| 01 | done | 1 | [Stable plugin status slots](task-01-plugin-status-slots.md) |
| 02 | done | 1 | [Status built-ins](task-02-status-builtins.md) |
| 03 | done | 2 | [Status surface shell](task-03-status-surface-shell.md) |
| 04 | done | 3 | [Mobile Status entry points](task-04-mobile-status-entry-points.md) |
| 05 | done | 3 | [Fixed-overlay audit](task-05-fixed-overlay-audit.md) |
| 06 | done | 3 | [Plugin fixture desktop E2E](task-06-plugin-fixture-desktop-e2e.md) |
| 07 | done | 4 | [Mobile Status E2E](task-07-mobile-status-e2e.md) |
| 08 | done | 3 | [Attribution and public docs](task-08-attribution-public-docs.md) |
| 09 | done | 5 | [Integration verification](task-09-integration-verification.md) |
