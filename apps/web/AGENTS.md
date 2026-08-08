# Frontend (Vite/React SPA) — architecture and conventions

Scoped guidance for `apps/web/`. Repo-wide rules (commit format, code-quality limits, etc.) live in the root `AGENTS.md`.

## Plugin authoring

For plugin UI work, begin with the [canonical plugin authoring guide](../../docs/public/plugins-authoring.md). Follow: choose recipe → edit `manifest.yaml` → implement → validate → package → smoke test. The frontend contract pair is `../../docs/plans/plugins/PLUGIN-API.md` plus `lib/plugins/types.ts`; concrete shared Host UI exports are in `lib/plugins/host-api.ts`, and registration/cleanup behavior is in `lib/plugins/registry.ts` and `lib/plugins/host.ts`. Keep the guide and that contract pair synchronized; do not invent hooks such as task panels, task-menu actions, per-user `host.storage`, rich-text components, or Kanban-card injection when they are absent from the current source.

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

### Responsive and touch surfaces

- Use `hooks/use-responsive-breakpoint.ts` for application layout decisions. Its mobile boundary matches the sidebar's 768px `md` boundary, and it also models tablet, compact desktop, full desktop, and pointer precision; do not substitute the UI package's generic `useIsMobile` hook. Tablet is a coarse-pointer fallback between `md` and `lg`, so no fine-pointer width reports it — a spec that needs the tablet layout has to emulate touch.
- When a Tailwind visibility class gates the same surface the hook picks, both must use the same boundary. A `sm:` class paired with a hook-driven mobile branch leaves 640-767px in a state neither side renders.
- Use `useTouchDrawer` when a hover/popover disclosure needs a coarse-pointer `Drawer` alternative. Width-based phone composition and pointer-based disclosure behavior are related but not interchangeable.
- Existing Radix DropdownMenu and ContextMenu surfaces receive inset, safe-area-aware bottom-sheet treatment below 640px in `app/globals.css`. Reuse those primitives for contextual actions and add focused coverage for long or nested menus instead of creating a parallel mobile menu.
- Mobile capability parity does not require desktop layout parity. Load `/mobile-parity` for the Kandev surface decision guide, mobile design contract, and verification requirements.

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
│   ├── gitlab/                    # GitLab MRs, watches, MR automation options
│   └── ui/                        # preview, connection, active state, sidebar views
├── hydration/                     # SSR merge strategies

hooks/domains/{kanban,session,workspace,settings,comments,github,gitlab}/  # Domain-organized hooks
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

Quick Chat stores server conversations in `quickChat.sessions` and browser-local terminals in
`quickChat.terminalTabs`; `activeKind` and terminal IDs track selection. `quick-terminal-actions.ts`
owns lifecycle/fallback; terminal descriptors never enter conversation APIs or get lost in reconciliation.

**Hydration:** Go injects `window.__KANDEV_BOOT_PAYLOAD__` into the SPA shell before React mounts. `lib/state/hydration/merge-strategies.ts` has `deepMerge()`, `mergeSessionMap()`, `mergeLoadingState()` to avoid overwriting live client state. Pass `activeSessionId` to protect active sessions.

For rebasing or finishing PRs written against the old Next.js runtime, follow [`docs/nextjs-spa-migration.md`](../../docs/nextjs-spa-migration.md).

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
- **Self-documenting settings:** every setting must explain in visible, plain-language copy what
  changes, when the setting applies, and when the user should choose each non-obvious option. State
  important exclusions, precedence, cost, or destructive consequences next to the control when they
  can affect the decision. Do not rely on tooltips, external documentation, or implementation terms
  alone to teach the setting.
- **Settings save coordination:** settings surfaces with local unsaved state must register a
  contributor with `useSettingsSaveContributor` (or use `SettingsPageTemplate`) so the shared
  floating **Save changes** control, navigation guard, and discard flow own persistence. Do not add
  page-local Save/Cancel controls. Contributor `save` callbacks must reject on failure so the
  coordinator can report an error; `discard` must restore the contributor's authoritative baseline.
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
- **Renaming a `data-testid`:** set the new id as `data-testid="<new>"` and keep the old id as
  `data-legacy-testid="<old>"`, then migrate e2e specs to the new id in the same PR. JSX rejects two
  `data-testid` attributes on one element, and Playwright's `getByTestId` only matches one attribute
  name — the `data-legacy-testid` alias lets existing specs keep selecting the element mid-migration.
- **Dockview session panel activation:** session chat panels can become active through tab
  pointer/keyboard events, global tab-cycling shortcuts, reopen/menu actions, and Dockview close
  controls. When changing `tasks.activeSessionId` or active-session sync, audit all of those paths.
  Use store state in addition to Dockview `api.isActive`; the current session's chat tab may be
  Dockview-inactive while Files/Changes is active. Same-session clicks must not leave stale
  activation intent, and Dockview `.dv-default-tab-action` close controls should be treated as
  close/delete actions rather than session-switch intent.
- **Conditional review-panel ownership:** the reusable `pr-detail` panel may be
  visible only while the active task has a linked PR or MR. A custom Default
  layout's canonical panel supplies the preferred group and tab index; it does
  not make an empty tab persistent. After review data hydrates, review loss
  removes any canonical panel regardless of how it entered the runtime layout.
  Restoration, maximized layouts, and a session's offered/dismissed marker
  defer or suppress automatic insertion; linked existing panels synchronize
  provider and review identity without moving them.
- **GitHub PR status UI:** visual PR/CI status surfaces should use the shared helpers in
  `apps/web/components/github/pr-task-icon.tsx` (`hasPRChecksPassedForDisplay`,
  `hasPRChecksInProgressForDisplay`, `hasPRChecksPassedWithoutReviewWaitForDisplay`) instead of
  re-deriving status from `checks_state`, `checks_total`, or `checks_passing` locally. Aggregate
  check counts are a display-only fallback when `checks_state` is empty; they may make chips or task
  icons render passed/in-progress, but must not enable merge actions. Merge readiness must use
  `isPRReadyToMerge`, which requires GitHub's explicit `checks_state === "success"` rollup. When
  changing PR status behavior, update both `pr-task-icon.test.ts` and `pr-status-chip.test.tsx`.
- **Task repository labels:** user-facing task/card repo chips should display a
  stable repo slug or name (`owner/repo` when known, otherwise the repo name),
  not a local filesystem path. Local clone paths or folder paths belong in
  hover/title/tooltip metadata. Tasks with no repository, or only a non-repo
  local folder, should not render a repo chip.

## Internationalization (i18n)

**Externalization is complete.** #2367 localized `app/office`, the last
un-migrated area. A hardcoded user-facing literal is now a regression, not
leftover migration work. New copy must go through `t()` / `<Trans>` wherever you
write it.

User-facing strings are localized with i18next + react-i18next, keyed as
`namespace:key`. Add the English text to `src/locales/en/<namespace>.json`, then
reference it with `t("settings:deleteExecutor")` (`useTranslation()` in
components, the module-level `t` from `@/lib/i18n` in plain helpers). Use
`<Trans i18nKey=... values={...}>` only for copy containing markup — and never a
`t()` call inside its children, which shifts the message's tag indices.

Never translate user/domain data, code identifiers, `data-testid`, or a literal
that is also compared with `===`, used as a map key, or typed as a string-literal
union. When a prop is both display copy and logic (`label: "Reviewers" |
"Assignees"`), split it into a `kind`/`origin` discriminant plus a translated
label rather than translating in place.

`lib/i18n/provider.tsx` initializes i18next at module load. Do not remove that
call: react-i18next suspends on an uninitialized instance and there is no
Suspense boundary above the root, so the app renders a blank page with no error
of any kind. Unit tests cannot catch it — `vitest.setup.ts` pre-initializes.

Never write a plural ending yourself: use `t(key, { count })` with `_one`/`_other`
keys. Passing the morpheme as a value (`{ s: n === 1 ? "" : "s" }`) is
untranslatable — the plural rule ends up at the call site.

Never assign `t()` to a module-level constant. It resolves at import, before a
locale is active, and never updates on a switch — and the pseudo-locale cannot
see it, because the text _is_ translated, just frozen. Store the key and resolve
at render, or make the value a component. `check-module-scope-t.mjs` enforces it.

`pnpm lint` fails on hardcoded UI strings (`i18next/no-literal-string` is an
**error**), but **only on the `i18nGuardFiles` allowlist** in
`eslint.i18n.options.mjs`, which covers every area the migration touched. It
stays an allowlist because a repo-wide error breaks every unrelated PR that adds
a label — what made the first attempt unmergeable. **Append a path in the same PR
that adds it or externalizes it** (`lib/sidebar` is one still off the list).
Never delete an entry to make a build pass; `check-guard-allowlist.mjs` rejects
that unless the file is gone. Use `pnpm run lint:i18n <path>` to preview the
guard on a path not yet on the list.

Separately, `pnpm run i18n:ratchet` (pre-commit + CI) guards **new code
everywhere**, regardless of the allowlist: a file you added must be clean outright,
and a file you modified is judged on the lines you touched. Untouched literals are
never reported, so it cannot ask you to migrate code you did not write — the same
contract as `golangci-lint --new-from-rev` for Go.

The rule **only sees literals in JSX** — `confirm()` arguments and copy in plain
`.ts` helpers are invisible to it — and it **skips anything assigned to a
SCREAMING_CASE identifier**, so `const ROWS = [{ label: "Disk usage" }]` passes
silently. A clean lint is not proof a file is done. `pnpm run i18n:check` gates key/catalog
drift, `<Trans>` tag indices, inline plurals and module-scope `t()`, and the
**pseudo-locale** (Settings → General → Appearance, dev/e2e) is the completeness
check — any plain-English text under it was never externalized. The tooling needs
**Node 24**. Full guide:
[`docs/i18n.md`](../../docs/i18n.md); spec:
[`docs/specs/platform/i18n.md`](../../docs/specs/platform/i18n.md).

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

## Plugin system

The frozen frontend contract is `docs/plans/plugins/PLUGIN-API.md`; `lib/plugins/types.ts`
is its TS mirror — the two must change together. `lib/plugins/registry.ts` is the
reactive singleton `PluginRegistry`; every `register*` call there needs a matching
cleanup entry in `unregisterPlugin` and `totalCount()`, or a disabled/uninstalled
plugin leaks a stale registration.

- **Task panels** (`registerTaskPanel`): one generic dockview component, `"plugin-panel"`, shared by
  every plugin — identity lives in `params: { pluginId, panelKey }` (id helpers in
  `lib/state/layout-manager/plugin-panels.ts`). `renderPanel` in `dockview-shared.tsx` and
  `dockview-panel-content.tsx` each get exactly one `"plugin-panel"` case (lookup tables, not
  switches). `PluginTaskPanel` (`components/task/`) resolves the registration behind a
  `PluginErrorBoundary`; `mobileEnabled: true` also renders it via the phone bottom nav
  (`session-mobile-bottom-nav.tsx`) with `presentation: "mobile"`.
- **Kanban card contributions:** `registerTaskMenuAction({ group: "edit", ... })` turns the flat
  `Edit` item into an `Edit` submenu (`kanban-card-edit-submenu.tsx`);
  `registerComponent("task-card-indicators", ...)` renders beside `PRTaskIcon` via `<PluginSlot/>`; `task-card-tags` renders in its own row below the badges (for contributions too wide for the title-row indicators spot, e.g. tag chips).
- **`host.storage`:** authenticated per-user key/value storage (`lib/plugins/host-api.ts`), backed by
  `/api/plugins/{id}/user-state/...` (`docs/decisions/2026-08-01-per-user-plugin-storage.md`).
  `subscribe` (`lib/plugins/user-state-sync.ts`) wraps `registerWsHandler` with own-plugin filtering
  and own-tab echo suppression via a per-tab `writerId`.
- **`host.ui.RichTextEditor`/`RichTextReadOnly`** (`components/editors/tiptap/rich-text-editor.tsx`): narrow Plan-panel-tiptap wrappers; update `PLUGIN-API.md` before widening props beyond `{ taskId, value, onChange, placeholder, className, testId }` / `{ value, className, testId }`.

## Testing notes

- jsdom drops `secure` cookies over `http`, so `document.cookie` reads back empty. To assert a cookie write in a Vitest unit test, intercept the setter with `Object.defineProperty(document, "cookie", { set: ... })` and restore it after.
- jsdom synthetic mouse events do not reliably open Radix Tooltip. In component tests, use
  `TooltipProvider` and assert keyboard focus; cover pointer hover in Playwright with `locator.hover()`
  and a visible `role="tooltip"`, keeping regressions that jsdom cannot exercise.
- In Playwright tests, avoid strict locators that assume only one `terminal-panel` or `.xterm` exists. Mobile and dockview layouts can mount multiple terminal instances; scope to the active panel or use `.first()` / `.last()` deliberately with a comment or helper. Shared E2E helpers that inspect mounted React/DOM internals must also be scoped to the active panel/container, not global selectors, because hidden or stale mounted panels can coexist in dock/mobile layouts.
