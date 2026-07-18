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
Go Boot Payload -> Hydrate Store -> Components Read Store -> Hooks Subscribe
```

**Never fetch data directly in components.**

## Store Structure (Domain Slices)

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

- `messages.bySession[sessionId]`, `shell.outputs[sessionId]`, `gitStatus.bySessionId[sessionId]`
- `tasks.activeTaskId`, `tasks.activeSessionId`, `workspaces.activeId`
- `repositories.byWorkspace`, `repositoryBranches.byRepository`

**Hydration:** Go injects `window.__KANDEV_BOOT_PAYLOAD__` into the SPA shell before React mounts. `lib/state/hydration/merge-strategies.ts` has `deepMerge()`, `mergeSessionMap()`, `mergeLoadingState()` to avoid overwriting live client state. Pass `activeSessionId` to protect active sessions.

For rebasing or finishing PRs written against the old Next.js runtime, follow
[`docs/nextjs-spa-migration.md`](../../docs/nextjs-spa-migration.md).

**Hooks Pattern:** Hooks in `hooks/domains/` encapsulate WS subscription + store selection. WS client deduplicates subscriptions automatically.

## WebSockets

**Format:** `{id, type, action, payload, timestamp}`.

Use subscription hooks only; the WS client auto-deduplicates.

When changing task lifecycle WS handlers (`task.updated`, `task.deleted`,
`task.state_changed`), check both kanban and Office surfaces. Archive/delete
events may need to update kanban caches, `tasks.activeTaskId` / session pin
state, recent/sidebar prefs, Office refetch triggers such as
`setOfficeRefetchTrigger("tasks")`, and route redirects for `/t/:id`,
`/tasks/:id`, and `/office/tasks/:id`. Add focused tests for every affected
surface.

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
- **Dialog Enter-to-confirm:** the base `@kandev/ui` `DialogContent` / `AlertDialogContent`
  activate the dialog's semantic action on plain Enter (`packages/ui/src/lib/dialog-default-action.ts`),
  so per-dialog "submit on Enter" input handlers are unnecessary — let the base own it.
  Resolution: `AlertDialogAction` → an explicit `data-dialog-default-action` button → the single
  primary (`type="submit"` or `data-variant="default"|"destructive"`) button in `DialogFooter`.
  More than one primary candidate (counting disabled ones), a disabled resolved action, or one inside
  a `hidden`/`aria-hidden` subtree → no-op (never guesses). Left alone: `textarea`/contenteditable,
  Shift/Cmd/Ctrl/Alt+Enter, `event.repeat` auto-repeat, mid-IME composition (`isComposing` or keyCode
  229), already-`preventDefault`ed events, and Enter fired from a focused interactive control that owns
  Enter (any action button — including outline/secondary like Copy/Back — `<select>`, combobox, or a
  listbox option / menu item). Only a slot-marked `alert-dialog-cancel` / `dialog-close` is treated as
  a dismiss control and overridden (the Radix-focuses-Cancel case). A plain single-line `<input>` is
  _not_ exempt — type-to-confirm dialogs rely on Enter firing the primary.
  Pass `enterConfirms={false}` to opt a dialog out; mark the intended button with
  `data-dialog-default-action` when a footer has several action buttons.
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
- **Interactive help inside Radix tooltips:** do not nest a `Tooltip` root inside
  another `TooltipContent` when the inner trigger must remain interactive.
  Tooltip roots under one provider coordinate open state, so the inner tooltip
  can close and unmount its parent before the pointer reaches it. Render the
  secondary help inline in the existing content or use a disclosure primitive
  with independent open state. Touch-pinned help must close on a second trigger
  tap, outside interaction, and Escape; verify desktop pointer and mobile-sized
  touch flows.
- **Renaming a `data-testid`:** set the new id as `data-testid="<new>"` and keep
  the old id as `data-legacy-testid="<old>"`, then migrate e2e specs to the new
  id in the same PR. JSX rejects two `data-testid` attributes on one element,
  and Playwright's `getByTestId` only matches one attribute name — the
  `data-legacy-testid` alias lets existing specs keep selecting the element
  while the migration is in flight.
- **Dockview session panel activation:** session chat panels can become active
  through tab pointer/keyboard events, global tab-cycling shortcuts,
  reopen/menu actions, and Dockview close controls. When changing
  `tasks.activeSessionId` or active-session sync, audit all of those paths. Use
  store state in addition to Dockview `api.isActive`; the current session's chat
  tab may be Dockview-inactive while Files/Changes is active. Same-session
  clicks must not leave stale activation intent, and Dockview
  `.dv-default-tab-action` close controls should be treated as close/delete
  actions rather than session-switch intent.
- **GitHub PR status UI:** visual PR/CI status surfaces should use the shared
  helpers in `apps/web/components/github/pr-task-icon.tsx`
  (`hasPRChecksPassedForDisplay`, `hasPRChecksInProgressForDisplay`, and
  `hasPRChecksPassedWithoutReviewWaitForDisplay`) instead of re-deriving status
  from `checks_state`, `checks_total`, or `checks_passing` locally. Aggregate
  check counts are a display-only fallback when `checks_state` is empty; they may
  make chips or task icons render passed/in-progress, but must not enable merge
  actions. Merge readiness must use `isPRReadyToMerge`, which requires GitHub's
  explicit `checks_state === "success"` rollup. When changing PR status behavior,
  update both `pr-task-icon.test.ts` and `pr-status-chip.test.tsx`.
- **Task repository labels:** user-facing task/card repo chips should display a
  stable repo slug or name (`owner/repo` when known, otherwise the repo name),
  not a local filesystem path. Local clone paths or folder paths belong in
  hover/title/tooltip metadata. Tasks with no repository, or only a non-repo
  local folder, should not render a repo chip.

## Markdown safety

Any renderer that enables embedded raw HTML must pair `rehype-raw` immediately
with `rehype-sanitize`, and enable that combination only on the intended
surface. Do not broaden raw-HTML support to chat, comments, or other renderers
without a separate security decision. Add regression coverage for permitted
README markup and for stripping executable HTML and unsafe URLs.

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
- jsdom synthetic mouse events do not reliably open Radix Tooltip. In component
  tests, render under `TooltipProvider` and assert the keyboard-focus path with
  `fireEvent.focus`. Cover pointer hover in Playwright with `locator.hover()` and
  assert the visible portaled `role="tooltip"`; do not remove a hover regression
  solely because `mouseenter` or `pointerMove` failed in jsdom.
- In Playwright tests, avoid strict locators that assume only one `terminal-panel` or `.xterm` exists. Mobile and dockview layouts can mount multiple terminal instances; scope to the active panel or use `.first()` / `.last()` deliberately with a comment or helper.
- Shared E2E helpers that inspect mounted React/DOM internals must be scoped to the active panel/container, not global selectors, because hidden or stale mounted panels can coexist in dock/mobile layouts.
