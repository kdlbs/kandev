# Frontend (Vite/React SPA) — architecture and conventions

Scoped guidance for `apps/web/`. Repo-wide rules (commit format, code-quality limits, etc.) live in the root `AGENTS.md`.

## UI Components

**Shadcn Components:** Import from `@kandev/ui` package:

```typescript
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Dialog } from "@kandev/ui/dialog";
// etc...
```

**Do NOT** import from `@/components/ui/*` - always use `@kandev/ui` package.

- Always prefer native shadcn components over custom implementations.
- Check `apps/packages/ui/src/` for available components (pagination, table, dialog, etc.).
- For data tables, use `@kandev/ui/table` with TanStack Table; use shadcn Pagination components.
- Only create custom components when shadcn doesn't provide what's needed.

## Data Flow Pattern (Critical)

```text
Go Boot Payload -> Seed Query Cache + Hydrate UI Store -> Domain Hooks -> Components
WebSocket Events -> Query Bridge -> Query Cache -> Mounted UI
```

**Never fetch server data directly in components.** Server-owned data should go
through TanStack Query keys/options and domain hooks. Zustand is for
client-only UI state, persisted user preferences, and explicitly documented
temporary indexes during the TanStack migration.

## Store Structure (Domain Slices)

```text
lib/query/
├── client.ts                       # Browser QueryClient defaults
├── keys.ts                         # Typed query-key factories
├── provider.tsx                    # Query provider and e2e exposure
├── query-options/                  # Domain query option factories
└── bridge/                         # WS event -> query cache updates

lib/state/
├── store.ts                        # Root composition
├── default-state.ts                # Default state + initial state merge
├── slices/                         # Domain slices
│   ├── kanban/                    # active workflow/task/session UI state
│   ├── session/                   # sessions, messages, turns, worktrees
│   ├── session-runtime/           # shell, processes, git, context
│   ├── workspace/                 # workspace list and active workspace UI state
│   ├── settings/                  # userSettings and local persisted preferences
│   ├── comments/                  # code review diff comments
│   ├── github/                    # GitHub PRs, reviews
│   └── ui/                        # preview, connection, active state, sidebar views
├── hydration/                     # SSR merge strategies

hooks/domains/{kanban,session,workspace,settings,comments,github}/  # Domain-organized hooks
lib/api/domains/                    # API clients
├── kanban-api, session-api, workspace-api, settings-api, process-api
├── plan-api, queue-api, workflow-api, stats-api, github-api
├── user-shell-api, debug-api, secrets-api, sprites-api, vscode-api
├── health-api, utility-api
```

**Key State Paths:**

- `tasks.activeTaskId`, `tasks.activeSessionId`, `workflows.activeId`, `workspaces.activeId`
- `messages.bySession[sessionId]`, `turns.bySession[sessionId]`, and
  `taskSessions.items[sessionId]` are retained live session indexes for stream
  ordering, active-session chrome, missed-frame recovery, and editor/panel UI.
- `shell.outputs[environmentId]`, `processes.*`, `gitStatus.byEnvironmentId`,
  `sessionCommits.byEnvironmentId`, `contextWindow.bySessionId`,
  `prepareProgress.bySessionId`, `sessionModels.bySessionId`, and
  `userShells.byEnvironmentId` are retained runtime indexes for high-frequency
  streams, environment-scoped cleanup, and terminal/session UI.
- Workspace repositories, repository branches/scripts, workflow lists, workflow
  snapshots, task details, settings catalogs, integrations, office data, and
  system data are TanStack Query data.

**Hydration:** Go injects `window.__KANDEV_BOOT_PAYLOAD__` into the SPA shell
before React mounts. Boot and app-state payloads seed TanStack Query through
`lib/query/seed.ts` before route hooks fetch. The Zustand hydrator still merges
client-only UI state and persisted preferences; `lib/state/hydration/merge-strategies.ts`
has `deepMerge()`, `mergeSessionMap()`, `mergeLoadingState()` to avoid
overwriting live client state. Pass `activeSessionId` to protect active
sessions.

For rebasing or finishing PRs written against the old Next.js runtime, follow
[`docs/nextjs-spa-migration.md`](../../docs/nextjs-spa-migration.md).

**Hooks Pattern:** Hooks in `hooks/domains/` encapsulate query selection,
mutations, and WS subscription intent. WS client deduplicates subscriptions
automatically.

## WebSockets

**Format:** `{id, type, action, payload, timestamp}`.

Use subscription hooks only; the WS client auto-deduplicates.

Server-state WS handlers should live in `lib/query/bridge/` and write or
invalidate the same query keys the mounted UI reads. Legacy `lib/ws/handlers/*`
files are only for retained client effects, high-frequency streams, or
documented temporary migration paths.

## Component conventions

- **Framework adapters during Next removal:** Client components should import
  links, router hooks, dynamic imports, images, and theme hooks from the local
  adapter modules (`components/routing/*`, `lib/routing/*`,
  `components/theme/app-theme`) instead of importing `next/*` or
  `next-themes` directly. The routing/image/dynamic adapters now provide
  browser-native behavior for the Vite SPA while legacy Next entrypoints are
  phased out.
- Components: <200 lines, extract to domain components, composition over props.
- Hooks: domain-organized in `hooks/domains/`, encapsulate subscription + selection.
- **Interactivity:** all buttons and links with actions must have `cursor-pointer` class.
- **Radix tooltip on disabled buttons:** disabled buttons do not receive pointer/focus events, so wrap the disabled `Button` in a focusable span and put `TooltipTrigger asChild` on that span:
  ```tsx
  <Tooltip>
    <TooltipTrigger asChild>
      <span tabIndex={disabled ? 0 : -1} className="inline-flex">
        <Button disabled={disabled}>Run</Button>
      </span>
    </TooltipTrigger>
    <TooltipContent>{disabledReason}</TooltipContent>
  </Tooltip>
  ```
  Keep the wrapper focusable only while disabled; when enabled, the button itself owns focus.
- **Renaming a `data-testid`:** set the new id as `data-testid="<new>"` and keep
  the old id as `data-legacy-testid="<old>"`, then migrate e2e specs to the new
  id in the same PR. JSX rejects two `data-testid` attributes on one element,
  and Playwright's `getByTestId` only matches one attribute name — the
  `data-legacy-testid` alias lets existing specs keep selecting the element
  while the migration is in flight.

## Code-quality limits

Enforced by `apps/web/eslint.config.mjs` (warnings, will become errors):

- Files: ≤600 lines · Functions: ≤100 lines
- Cyclomatic complexity: ≤15 · Cognitive complexity: ≤20
- Nesting depth: ≤4 · Parameters: ≤5
- No duplicated strings (≥4 occurrences) · No identical functions · No unused imports
- No nested ternaries

When you hit a limit, extract a helper function, custom hook, or sub-component. Prefer composition over growing a single function.

## Testing notes

- jsdom drops `secure` cookies over `http`, so `document.cookie` reads back empty. To assert a cookie write in a Vitest unit test, intercept the setter with `Object.defineProperty(document, "cookie", { set: ... })` and restore it after.
- In Playwright tests, avoid strict locators that assume only one `terminal-panel` or `.xterm` exists. Mobile and dockview layouts can mount multiple terminal instances; scope to the active panel or use `.first()` / `.last()` deliberately with a comment or helper.
- Shared E2E helpers that inspect mounted React/DOM internals must be scoped to the active panel/container, not global selectors, because hidden or stale mounted panels can coexist in dock/mobile layouts.
