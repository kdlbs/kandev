# Frontend (Vite/React SPA) — architecture and conventions

Scoped guidance for `apps/web/`. Repo-wide rules (commit format, code-quality limits, etc.) live in the root `AGENTS.md`.

## Plugin authoring

For plugin UI work, begin with the [canonical plugin authoring guide](../../docs/public/plugins-authoring.md). Follow: choose recipe → edit `manifest.yaml` → implement → validate → package → smoke test. The independently consumable author contract is `@kandev/plugin-sdk` in `../packages/plugin-sdk`; `../../docs/plans/plugins/PLUGIN-API.md` and `lib/plugins/types.ts` document and implement host compatibility. Concrete shared Host UI exports are in `lib/plugins/host-api.ts`, and registration/cleanup behavior is in `lib/plugins/registry.ts` and `lib/plugins/host.ts`. New and official plugins use typed `host.context` reads and never copy/import private `AppState` or Zustand slice shapes. Extend the SDK, host implementation, contract docs, and exact-consumer compatibility test together.

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
- Use `useTouchDrawer` when a hover/popover disclosure needs a coarse-pointer `Drawer` alternative. Width-based phone composition and pointer-based disclosure behavior are related but not interchangeable. Apply the 44px minimum to coarse-pointer hit areas, touch rows, and mobile controls only; keep fine-pointer desktop controls at the surrounding design-system density and do not reuse a touch-sized `h-11` class as the shared visual button size.
- Existing Radix DropdownMenu and ContextMenu surfaces receive inset, safe-area-aware bottom-sheet treatment below 640px in `app/globals.css`. Reuse those primitives for contextual actions and add focused coverage for long or nested menus instead of creating a parallel mobile menu.
- Mobile capability parity does not require desktop layout parity. Load `/mobile-parity` for the Kandev surface decision guide, mobile design contract, and verification requirements.

## Data Flow Pattern (Critical)

```text
Go Boot Payload -> Hydrate Store -> Components Read Store -> Hooks Subscribe
```

**Never fetch data directly in components.**

### Browser capability boundaries

- Use `generateUUID()` for client-only non-security IDs, not `crypto.randomUUID()`; use `copyToClipboard()` for copy actions, not `navigator.clipboard.writeText()`. Keep fallbacks non-security; test missing or rejected capabilities with an `rg` audit.

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

Quick Chat stores server conversations in `quickChat.sessions` and browser-local terminals in `quickChat.terminalTabs`; `activeKind` and terminal IDs track selection. `quick-terminal-actions.ts` owns lifecycle/fallback; terminal descriptors never enter conversation APIs or get lost in reconciliation.

**Hydration:** Go injects `window.__KANDEV_BOOT_PAYLOAD__` into the SPA shell before React mounts. `lib/state/hydration/merge-strategies.ts` has `deepMerge()`, `mergeSessionMap()`, `mergeLoadingState()` to avoid overwriting live client state. Pass `activeSessionId` to protect active sessions.

For rebasing or finishing PRs written against the old Next.js runtime, follow [`docs/nextjs-spa-migration.md`](../../docs/nextjs-spa-migration.md).

**Hooks Pattern:** Hooks in `hooks/domains/` encapsulate WS subscription + store selection. WS client deduplicates subscriptions automatically.

## WebSockets

**Format:** `{id, type, action, payload, timestamp}`.

Use subscription hooks only; the WS client auto-deduplicates.

**Task overview vs. session detail:** Shared task rows read `Task.statusSummary` and `task.status_summary.updated`; rich streams stay session-detail-only. Extend the bounded projection per the [spec](../../docs/specs/platform/requirements/bounded-task-status-delivery.md) and [ADR](../../docs/decisions/2026-08-01-separate-task-summary-session-stream-traffic.md).
**Branch-scoped task state:** For live worktree/session state plus `task_prs`, key by `(repository, checked-out branch)`, not task/repository alone. `branch_switched` invalidates prior status/commits; reject late results with a generation/identity guard and preserve siblings. Historical PRs affect Changes only when `repository_id` and normalized `head_branch` match; Review/PR history may still show them. Test single/multi-repo cases and desktop/mobile Changes behavior.
**HTTP/WS cache races:** When HTTP hydrates a cache also updated by WebSockets, guard responses with per-scope revision and request/workspace generation; discard or refresh stale responses and cover deferred responses. `useEnsureTaskSession` re-fires `session.ensure` when an open task page sees zero sessions after a prior ensure, so deleting the last session from that page spawns a replacement; test zero-session states through the backend/API instead.

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
- **Code-host dashboards:** GitHub, GitLab, and plugin code-host pages must use
  the provider-neutral primitives in `components/integrations/` for
  change-request lists, rows, toolbars, scope controls, task preset menus, and
  linked-task indicators. Use the shared semantic `IntegrationIcon` glyphs instead of
  copying first-party SVG paths. Keep provider API/state logic in adapters; do not fork row
  anatomy or add dashboard review/launch flows outside the native task dialog and
  registered review surface. Plugins may override create transport only through an
  authenticated action; host verifies repository, session, and worktree branch authority.
- **Code-host task status:** registered providers publish `ReviewItemSummary.taskStatus`.
  Host chrome renders shared topbar, composer, popover/drawer, eager linked-row summaries,
  hover/focus refresh, and semantic colors. Initial refreshes are leased and deduplicated;
  do not add provider color fields, visual slots, or pollers; use `change-request-*` anatomy.
- **Code-host review detail:** GitHub and compatible review providers render
  `components/integrations/change-request-detail.tsx` (also exposed as
  `host.ui.ChangeRequestDetail`). Providers own normalized data/capabilities/actions,
  not parallel headers, review/check/comment sections, scroll containers, or mobile
  geometry.
- **Code-host task links:** keep one-field pull/merge-request linking in
  `components/integrations/task-change-request-link-form.tsx`. First-party providers
  compose it directly; plugins call `host.openTaskLinkDialog`. Link submenu children
  name the target only (for example, `Bitbucket Pull Request`) and preserve their
  registered provider icon.
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
- **Renaming a `data-testid`:** use `data-legacy-testid` for the old id while
  migrating specs; JSX and Playwright only support one `data-testid` attribute.
- **Dockview session activation:** audit pointer/keyboard tabs, shortcuts,
  reopen/menu actions, and close controls; combine store state with
  `api.isActive`, clear same-session intent, and treat default-tab close as
  delete rather than session switching.
- **Conditional review panels:** show `pr-detail` only for active tasks with a
  linked PR/MR; default layouts only provide preferred placement. Hydrated review
  loss removes canonical panels, while restoration/maximized and offered/dismissed
  markers suppress insertion; existing panels sync identity without moving.
- **Dockview environment switching:** reconcile ephemeral panels before restoring views;
  correlate ID-less groups by stable ID or position. Treat `chat`/`session:*` as semantic only with a non-null `activeSessionId`.
- **GitHub PR status UI:** use the shared `pr-task-icon.tsx` display helpers and
  `isPRReadyToMerge`; aggregate counts are display-only and cannot enable merges.
  Update `pr-task-icon.test.ts` and `pr-status-chip.test.tsx` with behavior changes.
- **GitHub PR associations:** retain terminal/merged siblings for tabs/unlink;
  derive `openPRs` only for aggregate status/automation and test desktop/mobile
  terminal unlink plus two-to-one collapse/focus.
- **Task repository labels:** user-facing task/card repo chips should display a
  stable repo slug or name (`owner/repo` when known, otherwise the repo name),
  not a local filesystem path. Local clone paths or folder paths belong in
  hover/title/tooltip metadata. Tasks with no repository, or only a non-repo
  local folder, should not render a repo chip.

## Internationalization (i18n)

**Externalization is complete.** A hardcoded user-facing literal is a
regression, not leftover migration work. New copy goes through `t()` / `<Trans>`
wherever you write it.

Add English keys to `src/locales/en/<namespace>.json`; use `useTranslation()` in
components and module-level `t` only inside plain helper calls. `<Trans>` is only
for markup, and a `t()` child corrupts its tag indices. Do not translate domain
data, identifiers, test IDs, discriminants, comparison tokens, or map keys; split
display copy from logic first. Keep `lib/i18n/provider.tsx` module
initialization; removing it blanks the app. Use `_one`/`_other` plural keys,
never English suffixes. Never capture `t()` in a module-level constant; it
freezes the boot locale. No Unicode em dash (U+2014) in copy or locale values.

`pnpm lint` fails on hardcoded UI strings: `i18next/no-literal-string` is an
**error on every `.ts`/`.tsx` file** (tests, `*.test-helpers.*`, `*.test-utils.*`
and `e2e/**` excluded). It was scoped to `i18nGuardFiles` during the migration;
measured at zero violations across all 2560 source files, it was widened.
`i18nGuardFiles` remains the migration record and the `lint:i18n <path>` preview
scope: append when you externalize a path, never delete an entry
(`check-guard-allowlist.mjs` rejects that). `i18n:ratchet` guards new/changed
lines independently. The rule sees only JSX literals; SCREAMING_CASE tables,
plain `.ts` helpers, parameter defaults and toast/setter arguments are gated by
`scripts/check-nonjsx-copy.mjs`, which scans the **whole tree by exclusion**.
Silence a legitimate one with `// i18n-exempt: <reason>` (required) as a `//`
LINE comment — the detector's pattern is line-anchored, so a marker inside a
`/** */` block is silently ignored.

**Real-locale catalogs gate.** `pt-pt`, `zh-cn`, `zh-hk`, `zh-tw` are complete;
`check-i18n-keys.mjs` fails on a missing/extra key, a dropped `{{placeholder}}`
or `<n>` tag, an empty value, or a value identical to English. Untranslatable
values are handled in two tiers: those `looksLikeCopy` rejects as non-copy need
no declaration; prose that reads the same in the target language goes in
`src/locales/<locale>/_verbatim.json` (or the shared `_verbatim.json`) with a
mandatory reason — reasonless or stale entries are errors. For zh-tw/zh-hk run
`pnpm run i18n:zh-hant`, do not hand-translate.

`i18n:check` also gates key/catalog drift, `<Trans>` indices, inline plurals,
module-scope `t()`, em dashes, and the **pseudo-locale** check. Needs **Node 24**.
Guide: [`docs/i18n.md`](../../docs/i18n.md); spec
[`docs/specs/platform/requirements/i18n.md`](../../docs/specs/platform/requirements/i18n.md).

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

The public frontend contract is `apps/packages/plugin-sdk`; `docs/plans/plugins/PLUGIN-API.md`
and `lib/plugins/types.ts` are its detailed host implementation — all three must change
together. `lib/plugins/registry.ts` is the reactive singleton `PluginRegistry`; every
`register*` call needs matching cleanup in `unregisterPlugin` and `totalCount()`, or a disabled/uninstalled
plugin leaks a stale registration.

- **Task panels** (`registerTaskPanel`): one generic dockview component, `"plugin-panel"`, shared by
  every plugin — identity lives in `params: { pluginId, panelKey }` (id helpers in
  `lib/state/layout-manager/plugin-panels.ts`). `renderPanel` in `dockview-shared.tsx` and
  `dockview-panel-content.tsx` each get exactly one `"plugin-panel"` case (lookup tables, not
  switches). `PluginTaskPanel` (`components/task/`) resolves the registration behind a
  `PluginErrorBoundary`; `mobileEnabled: true` also renders it via the phone bottom nav
  (`session-mobile-bottom-nav.tsx`) with `presentation: "mobile"`.
- **Task contributions:** `registerTaskMenuAction({ group: "edit", ... })` adds card-only actions to
  the `Edit` submenu. Group `"primary"` adds flat actions to card and desktop/mobile task-row menus.
  Card indicator/tag slots stay card-specific; `task-row-metadata` is generic for sidebar and `/tasks` rows.
- **Sidebar workspace actions:** `registerComponent("sidebar-workspace-actions", ...)` renders after Quick Terminal/Quick Chat in the desktop sidebar's New Task row and in the shared phone navigation sheet, forwarding `SidebarWorkspaceActionsSlotProps` with `presentation: "desktop" | "mobile"`; mobile plugin controls own a 44px touch target and accessible name.
- **`host.storage`:** authenticated per-user key/value storage (`lib/plugins/host-api.ts`), backed by
  `/api/plugins/{id}/user-state/...` (`docs/decisions/2026-08-01-per-user-plugin-storage.md`).
  `subscribe` (`lib/plugins/user-state-sync.ts`) wraps `registerWsHandler` with own-plugin filtering
  and own-tab echo suppression via a per-tab `writerId`.
- **`host.ui.RichTextEditor`/`RichTextReadOnly`** (`components/editors/tiptap/rich-text-editor.tsx`): narrow Plan-panel-tiptap wrappers; update `PLUGIN-API.md` before widening props beyond `{ taskId, value, onChange, placeholder, className, testId }` / `{ value, className, testId }`.

## Testing notes

- `vitest.config.ts` pins `process.env.NODE_ENV = "test"`, and that line is load-bearing. React exports `act()` only from its development build, so under `NODE_ENV=production` — which the runtime image sets (`Dockerfile`) and every container/agent shell inherits, while CI's image does not — `@testing-library/react` falls back to `react-dom/test-utils` and **every** `render()`/`renderHook()` throws `TypeError: React.act is not a function` before any assertion, with CI still green. `vitest-environment.test.tsx` and the `vitest.setup.ts` preflight fail by name if the pin goes, and the `Run tests` step in `.github/workflows/frontend-tests.yml` exports `NODE_ENV=production` on purpose so those guards fire in CI too rather than only in a container.
- **E2E waits name the cause, they do not budget for the effect.** The suite's dominant flake is `await expect(x).toBeEnabled({ timeout: 30_000 })` — an assertion on a UI shadow of a backend event whose hand-picked budget expires under load. `e2e/helpers/causal-waits.ts` has one primitive per transport (`waitForHttp`, `watchWs().waitForEvent`, `watchWs().waitForResponse`): arm it before the action, await it after, then assert the UI with its **default** timeout. "The backend reached state X" needs no primitive — `expect.poll` against `e2e/helpers/api-client.ts` already reads the backend, not the DOM. `watchWs(page)` only sees sockets opened after it is called, so it goes before the first `page.goto()`. Confirm a causal chain by probing a live run with a throwaway `page.on("response")` logger; reading the components predicts it wrong often enough to matter. The only sanctioned wall-clock wait is `dwell(page, ms, category, reason)`, or `dwell(ms, category, reason)` where no `Page` exists (fixtures, api-client retries) — `category` is the closed `DwellCategory` union (`negative-assertion` is permanent, `unverified` is debt to drive to zero), `reason` says why no event exists, no options; a delay inside a `page.route()` handler is `injectLatency(ms, reason)` instead, since the delay is the stimulus rather than a wait; raw `page.waitForTimeout` and promise sleeps are not sanctioned. Guide and worked examples: `e2e/README.md`. **The sleep ban is enforced in two layers**, the same shape as the i18n guards, and the conversion is complete so both cover the whole tree. `eslint-rules/no-unsanctioned-sleep.mjs` is an AST rule (`e2e/` has ~700 `test.setTimeout()` calls, Playwright's per-test timeout setter, which a regex cannot separate from a sleep — nor a sleep from a `Promise.race` guard); it is an **error across all of `e2e/`** via `e2eSleepGuardFiles`, and it also rejects a `dwell`/`injectLatency` not imported from `e2e/helpers/causal-waits`, since a failed import otherwise lints clean and throws on exactly the loaded shard the wait existed to survive. `pnpm run e2e:sleep-ratchet` (CI + pre-commit) is the second layer and judges the **change, not the file**. The guard **only ever widens** — never narrow it to make a build pass; `e2e-sleep-wiring.test.ts` asserts coverage via ESLint's own config resolution and fails if the CI step or hook disappears. Details: `e2e/README.md` ("How this is enforced").
- jsdom secure cookies need cookie-setter interception; Radix Tooltip tests use keyboard focus, while Playwright covers pointer hover with `locator.hover()`. Scope terminal selectors to the active panel/container; mobile and dockview may mount multiple instances, so shared helpers must not use global selectors. Local full Vitest suites use the configured 20-percent worker budget; do not pass all-worker overrides or overlap suites.
