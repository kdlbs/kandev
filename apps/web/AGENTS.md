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
- **Migrating a domain is not done when the bridge writes to TQ — the UI consumer must also _read_ from `useQuery`, not the Zustand mirror.** "Bridge wrote, UI doesn't read" is a silent-desync bug class: WS events update the TQ cache but the component still selects from Zustand, so the UI only refreshes on reload (the "agent messages need a refresh" bug). When migrating, flip the consumer and the writer together.
- **Panels rendering agentctl-poll-driven data (e.g. `gitStatus`, pushed by a 3s-fast/30s-slow workspace poll) must resync on dockview tab activation.** There's a cold-start race where the `session.focus`→fast-poll push is lost, leaving content stale up to 30s. See `useResyncOnTabActivate` (editor) and `useResyncGitStatusOnTabActivate` (diff panel, via `client.refreshSessionData()`) in `components/task/dockview-shared.tsx` — copy that pattern for any new poll-backed panel.

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

## WS event accounting

E2E enforces that every backend→FE WS event is observably received, parsed, and applied. Enabled in CI on the e2e shards via `KANDEV_E2E_WS_ASSERT=1`; the hooks are baked into the bundle only when `NEXT_PUBLIC_KANDEV_E2E_MOCK=true` (set by the e2e build). It fails a test when:

- the FE never processed an event the backend sent (a per-connection or per-session `seq` gap — receipt layer, `lib/ws/ws-account.ts`), or
- a session-routed event ran a bridge handler that did NOT mutate the TQ cache (apply layer, the bridge audit in `lib/query/bridge/index.ts`).

**Adding a new WS message type:** register it in BOTH `lib/ws/handlers/<domain>.ts` (Zustand mirror) AND `lib/query/bridge/<domain>.ts` wrapped via `wrapBridgeHandler(qc, action, fn)` — OR add the action to `BRIDGE_SKIPPED_ACTIONS` / `BRIDGE_SKIPPED_PREFIXES` (keep the copy in `lib/query/bridge-audit-diff.ts` in sync) with an inline reason. Legitimate skip categories: control-plane / subscription acks, request/response acks (the receipt layer skips `type !== "notification"`), ring-buffer streams (handled outside TQ), and documented Zustand-only events. Invalidation-only handlers (`invalidateQueries`/`removeQueries`) count as "applied" — `wrapBridgeHandler` spies the cache-write methods, so it works even when the invalidated key isn't cached yet.

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
