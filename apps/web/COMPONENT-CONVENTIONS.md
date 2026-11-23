## Component conventions

- **Framework adapters:** Import links, router hooks, dynamic imports, images,
  and theme hooks from `components/routing/*`, `lib/routing/*`, and
  `components/theme/app-theme`. Do not import `next/*` or `next-themes` directly.
- Task links: `lib/links.ts::linkToTask` is the only `/t/:taskId` builder; pass raw IDs, use `TaskLink` or `AppLink`, and use `linkToTask` for router pushes. Keep compatibility `/tasks/:id`, Office/API paths, and route-recognition prefixes separate.
- Components stay under 200 lines; extract domain components. Hooks belong in `hooks/domains/` and encapsulate subscription plus selection. `ChangesPanelBody` owns the sole scroller; route working-tree, PR, commit, inline-file, and status row descriptors through `ChangesTimelineViewport`, keeping full collections for counts/actions and keying state by task/session/environment.
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
- **Self-documenting settings:** describe each setting's effect, when it applies, and how to choose non-obvious options; keep exclusions, precedence, costs, destructive consequences, errors, permissions, managed values, and input constraints visible.
  Use `SettingsInfo` for optional detail (desktop hover/focus, touch drawer); do not rely on external docs or implementation terms alone.
- **Settings composition:** use `SettingsGroup` for bordered groups and `SettingsRow` for simple preferences; keep one domain owner/save contributor and attach discovery to actual controls. Keep sections expanded and use existing header tabs for larger pages. Preserve specialized editor/table/diagnostic/credential layouts.
- **Settings save coordination:** register drafts with `useSettingsSaveContributor` or
  `SettingsPageTemplate`; they own shared save, navigation, and discard. Avoid page-local controls.
  Reject failures and restore the authoritative baseline. For normalized or fallback saves, snapshot
  the raw draft before awaiting; apply canonical values only if the draft still matches; test dirty clearing and preservation of in-flight edits.
- **Settings tabs:** use `components/settings/settings-tabs.tsx` in `SettingsPageHeader`; preserve drafts with validated URL `tab` state, map discovery fragments to the owning tab, and use 44px controls on phones and coarse pointers.
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
  correlate ID-less groups by stable ID or position. Setup can precede asynchronous layout restoration. Reconcile dependent state after restoration settles. Dispose added subscriptions. Test late restoration and mixed valid or stale candidates before ambiguity checks. Treat `chat`/`session:*` as semantic only with a non-null `activeSessionId`.
- **GitHub PR status UI:** use the shared `pr-task-icon.tsx` display helpers and
  `isPRReadyToMerge`; aggregate counts are display-only and cannot enable merges.
  Update `pr-task-icon.test.ts` and `pr-status-chip.test.tsx` with behavior changes.
- **GitHub PR associations:** retain terminal/merged siblings for tabs/unlink;
  derive `openPRs` only for aggregate status/automation and test desktop/mobile terminal unlink plus two-to-one collapse/focus.
- **Task repository labels and projections:** user-facing task/card repo chips
  should display a stable repo slug or name (`owner/repo` when known, otherwise the repo name),
  not a local filesystem path; local paths belong in metadata. Tasks with no repository should not render a repo chip.
  When a UI projection adds normalized collection data for grouping, preserve scalar filter fields such as
  `repositoryPath` and test both grouping and filtered views.
