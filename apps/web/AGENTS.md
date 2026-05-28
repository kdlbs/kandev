# Frontend (Next.js) — architecture and conventions

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

- SSR prefetch hydrates the TanStack Query cache (via `HydrationBoundary` / dehydrated state) for server-owned data.
- WS events flow into the cache via per-domain bridges (`lib/query/bridge/*.ts`) — registered from `QueryBridge` in `lib/query/provider.tsx`.
- Components read via `useQuery(queryOptions.foo(...))` or via domain hooks in `hooks/domains/<x>/`. Never call fetch APIs directly in components.
- Zustand still owns client-only state (active IDs, UI toggles, layout) and serves as the transitional mirror for not-yet-migrated server state.

## Query Layer

TanStack Query is the canonical server-state layer. Migrated domains: features, comments, integrations, automations, workspace, settings, jira, linear, github, gitlab, kanban, office, session, session-runtime. (`features` and `automations` Zustand slices were fully removed; the rest are transitional mirrors.)

```text
lib/query/
├── keys.ts             # qk.* typed key factories (single source of truth for cache keys)
├── query-options/      # per-domain queryOptions() — used in useQuery + SSR prefetch
├── bridge/             # WS→TQ-cache handlers (transitional, registered from QueryBridge)
├── streams/            # ring buffers for high-frequency streams (shell/process/terminal)
├── client.ts
└── provider.tsx        # QueryProvider + QueryBridge
```

**Bridge contract:** each `bridge/<domain>.ts` mirrors `lib/ws/handlers/<domain>.ts` but writes into the TQ cache via `queryClient.setQueryData` instead of Zustand. Bridges are deleted as consumers finish reading from queries instead of the Zustand mirror.

**Streams:** high-frequency output (shell, process, terminal) bypasses TQ and uses the ring-buffer registry in `lib/query/streams/ring.ts` — TQ's per-chunk notify is a perf cliff at thousands of chunks/sec.

## Store Structure (Domain Slices)

These slices hold **client-only state** (active IDs, UI toggles, layout) and serve as transitional mirrors for server state not yet fully migrated to TanStack Query.

```text
lib/state/
├── store.ts                        # Root composition
├── default-state.ts                # Default state + initial state merge
├── slices/                         # Domain slices
│   ├── kanban/                    # boards, tasks, columns
│   ├── session/                   # sessions, messages, turns, worktrees
│   ├── session-runtime/           # shell, processes, git, context
│   ├── workspace/                 # workspaces, repos, branches
│   ├── settings/                  # executors, agents, editors, prompts (incl. userSettings)
│   ├── office/                    # office agents, skills, projects, dashboard, issues
│   ├── comments/                  # code review diff comments
│   ├── github/                    # GitHub PRs, reviews
│   └── ui/                        # preview, connection, active state, sidebar views
├── hydration/                     # SSR merge strategies

hooks/domains/{kanban,session,workspace,settings,comments,github}/  # Domain-organized hooks
lib/api/domains/                    # API clients
├── kanban-api, session-api, workspace-api, settings-api, process-api
├── plan-api, queue-api, workflow-api, stats-api, github-api
├── office-api, office-extended-api, tree-api
├── user-shell-api, debug-api, secrets-api, sprites-api, vscode-api
├── health-api, utility-api
```

**Key State Paths:**

- `messages.bySession[sessionId]`, `shell.outputs[sessionId]`, `gitStatus.bySessionId[sessionId]`
- `tasks.activeTaskId`, `tasks.activeSessionId`, `workspaces.activeId`
- `repositories.byWorkspace`, `repositoryBranches.byRepository`

**Hydration:** `lib/state/hydration/merge-strategies.ts` has `deepMerge()`, `mergeSessionMap()`, `mergeLoadingState()` to avoid overwriting live client state. Pass `activeSessionId` to protect active sessions.

**Hooks Pattern:** Hooks in `hooks/domains/` wrap `useQuery(queryOptions.*)` (or the domain's `query-options` factory). The WS client still deduplicates subscriptions; the cache those subscriptions write into is now TanStack Query (via bridges), not Zustand.

## WebSockets

**Format:** `{id, type, action, payload, timestamp}`.

Use subscription hooks only; the WS client auto-deduplicates.

## Component conventions

- Components: <200 lines, extract to domain components, composition over props.
- Hooks: domain-organized in `hooks/domains/`, encapsulate subscription + selection.
- **Interactivity:** all buttons and links with actions must have `cursor-pointer` class.

## Code-quality limits

Enforced by `apps/web/eslint.config.mjs` (warnings, will become errors):

- Files: ≤600 lines · Functions: ≤100 lines
- Cyclomatic complexity: ≤15 · Cognitive complexity: ≤20
- Nesting depth: ≤4 · Parameters: ≤5
- No duplicated strings (≥4 occurrences) · No identical functions · No unused imports
- No nested ternaries

When you hit a limit, extract a helper function, custom hook, or sub-component. Prefer composition over growing a single function.
