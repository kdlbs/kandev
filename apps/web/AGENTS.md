# Frontend (Vite/React SPA) — architecture and conventions

Scoped guidance for `apps/web/`. Repo-wide rules (commit format, code-quality limits, etc.) live in the root `AGENTS.md`. Primary commands: `pnpm run lint` runs the Oxlint configuration; `pnpm run typecheck` runs `tsrs --noEmit --fast` after generation. Retained ESLint and classic TypeScript API roles, focused previews, and wiring evidence are in [`TOOLCHAIN.md`](TOOLCHAIN.md).

## Plugin authoring

For plugin UI work, begin with the [canonical plugin authoring guide](../../docs/public/plugins-authoring.md). Follow: choose recipe → edit `manifest.yaml` → implement → validate → package → smoke test. The independently consumable author contract is `@kdlbs/plugin-sdk` in `../packages/plugin-sdk`; `../../docs/plans/plugins/PLUGIN-API.md` and `lib/plugins/types.ts` document and implement host compatibility. Concrete shared Host UI exports are in `lib/plugins/host-api.ts`, and registration/cleanup behavior is in `lib/plugins/registry.ts` and `lib/plugins/host.ts`. New and official plugins use typed `host.context` reads and never copy/import private `AppState` or Zustand slice shapes. Extend the SDK, host implementation, contract docs, and exact-consumer compatibility test together. Standard mounted action slots use `host.ui.Action`/`ActionGroup`; preserve raw slots and `host.ui.Button`, avoid broad descendant CSS, and use touch-sized phone/coarse-pointer surfaces except the 24px tablet status bar. Phone status actions use the Status drawer.

## UI Components

Before UI work, read [Layout in DESIGN.md](../../DESIGN.md#layout) for workbench and settings composition. For UI components, responsive behavior, and touch interactions, also read [UI component guidance](UI-COMPONENTS.md) and use `/mobile-parity`.

## Data Flow Pattern (Critical)

```text
Go Boot Payload -> Hydrate Store -> Components Read Store -> Hooks Subscribe
```

**Never fetch data directly in components.** State in `lib/state/` is below UI/routes: components and routes may consume it, but state must not import `components/` or `app/`, including type-only, re-export, or dynamic imports. Put shared values in dependency-neutral modules.

### Browser capability boundaries

- Use `generateUUID()` for client-only non-security IDs, not `crypto.randomUUID()`; use `copyToClipboard()` for copy actions, not `navigator.clipboard.writeText()`. Keep fallbacks non-security; test missing or rejected capabilities with an `rg` audit.

## Store Structure (Domain Slices)

When changing or tracing store state, read [store structure](STORE-STRUCTURE.md).

## WebSockets

For WebSocket subscriptions, cache hydration, or task lifecycle events, read [WebSocket guidance](WEBSOCKETS.md).

## Component conventions

Before changing UI components, read the relevant [component conventions](COMPONENT-CONVENTIONS.md), including task links, navigation, integration surfaces, and accessibility.

## Internationalization (i18n)

For web localization changes, read [localization guidance](LOCALIZATION.md).

## Markdown safety

Any renderer that enables embedded raw HTML must pair `rehype-raw` immediately
with `rehype-sanitize`, and enable that combination only on the intended
surface. Do not broaden raw-HTML support to chat, comments, or other renderers
without a separate security decision. Add regression coverage for permitted
README markup and for stripping executable HTML and unsafe URLs.

## Code-quality limits

Enforced by `apps/web/oxlint.config.mjs` (warnings become failures through `--deny-warnings`):

- Files: ≤600 lines · Functions: ≤100 lines · Absolute-positioned virtualizer rows do not collapse CSS margins; do not layer row padding/gaps over an item's existing margin. Preserve the old rendered gap with a focused bounding-box regression.
- Cyclomatic complexity: ≤15 · Cognitive complexity: ≤20
- Nesting depth: ≤4 · Parameters: ≤5
- No duplicated strings (≥4 occurrences) · No identical functions · No unused imports
- No nested ternaries

When you hit a limit, extract a helper function, custom hook, or sub-component. Prefer composition over growing a single function.

## Plugin system

For host plugin system changes, read [plugin system guidance](PLUGIN-SYSTEM.md).

## Sidebar task views

`taskOverview.byId` owns lightweight task records. Board/workflow `taskIds` and sidebar page memberships reference them; compatible `tasks` arrays expose the exact canonical objects. `withTaskOverviewNormalization` merges legacy board, optimistic, and Office writes before publication. Owners cover boards, active detail, displayed pages, and reusable pages; final release evicts the record. Read journals protect live changes/deletions within 1,000 IDs / 1 MiB. Task timestamps and summary revisions are independent. See [shared task state](../../docs/specs/ui/system-design/sidebar-shared-task-state.md) for coverage and reconciliation. Complete `sqlite_nocase_v1` scopes page locally, including sets above 100; other views fetch bounded pages. Covered views have no query or updating announcement. Do not mount sidebar-only all-workflow fetches. `sidebarViewsByWorkspace` stores personal views: use `selectSidebarViews`, retain workspace identity through saves/rollback, and reserve `sidebarViews` for legacy hydration. The backend owns migration/defaults; writes use scoped `sidebar_view_state`. `SidebarTaskPageCache` shares reads per store and retains at most five first pages, 2 MiB including entities, for five minutes from fetch. Later pages remain display-only. Context generations fence reuse. Soft invalidation clears reusable pages but can publish reconciled provisional rows while one trailing refresh recovers membership; provisional pages are never reusable. Summary changes invalidate affected pages. Access denial clears rows and outstanding reads for every consumer. Sidebar page reads share one bounded recovery owner in `lib/sidebar/sidebar-task-page-recovery.ts`. Initial reads use five attempts within thirty seconds of visible time; a view with a displayed page uses nine attempts within a 150-second visible budget. Hidden tabs pause delays and budget, and Retry-After is honored. Joining consumers do not extend an active cycle. After exhaustion, manual Retry and a later foreground episode start immediately; background reads start only sixty seconds after the cycle ends. Disclosure-sync exhaustion restarts only through explicit Retry. Do not add per-consumer retry timers. Keep query status in `SidebarTaskQueryStatus`, without duplicate archive errors or layout-shifting banners.

## Testing notes

For web tests and E2E fixtures, read [testing notes](TESTING-NOTES.md).
