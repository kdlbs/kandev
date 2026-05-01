# Design

## Direction

Kandev should read as a restrained developer workbench. The shell is dense and quiet, with panels, command surfaces, and state indicators arranged so users can keep their place across tasks, sessions, repositories, and integrations.

The primary scene is a developer switching between agent output, git state, review context, files, terminals, preview panels, and external systems during focused code work. The design should reduce glare, avoid ornamental chrome, and keep work state legible for long sessions.

## Color

Use OKLCH tokens for theme colors. Neutrals should be lightly tinted rather than pure grayscale, with lower chroma near white and near black to prevent glare.

### Theme Strategy

- Light theme: low-glare tinted surfaces, not pure white. Use subtle panel separation through borders and slight surface shifts.
- Dark theme: low-glare charcoal neutrals, not pure black. Preserve contrast in code, terminal, and diff views without neon accents.
- Accent: one primary accent for focus rings, selected navigation, active segmented controls, and the primary command in a scope.
- Status palette: semantic success, warning, danger, info, running, pending, archived, and remote states. Status must include icon or text support so color is never the only signal.
- Data and diffs: keep git addition and deletion colors consistent across cards, file trees, Monaco, Pierre diffs, and terminal-adjacent UI.

### Token Contract

Use the existing shadcn variable shape as the contract: `--background`, `--foreground`, `--card`, `--popover`, `--muted`, `--border`, `--input`, `--ring`, `--primary`, `--accent`, `--destructive`, chart tokens, sidebar tokens, and git diff tokens.

Future token work should normalize app and package themes so `apps/web/app/globals.css`, `apps/packages/theme/src/globals.css`, `apps/web/lib/theme/colors.ts`, editor themes, and terminal themes stay in sync.

## Typography

Use `Figtree` as the preferred UI family, falling back to Geist and system UI. Use `Monaspace Neon` first for code and terminal surfaces, falling back to `Geist Mono`, platform monospace, and standard monospace.

- Product UI type should stay compact and fixed, not viewport-fluid.
- Page and panel headings should be modest. Reserve larger type for settings page titles and empty states, not dense work surfaces.
- Code, shell, log, branch, path, SHA, and command values should use the mono stack.
- Numeric status and counts should use tabular numerals where alignment matters.

## Layout

The shell should be panel-first. Panels, sidebars, topbars, and dock headers define the working frame; content should not be wrapped in decorative cards unless a repeated item, modal, or tool needs a frame.

- Use a unified topbar hierarchy: global orientation first, active task/session/workflow state second, primary scoped command third, secondary utilities last.
- Avoid nested cards and floating section cards. Prefer full-width bands, split panes, table rows, list rows, and dock panels.
- Keep gutters tight but intentional. Dense areas need clear alignment more than extra whitespace.
- Preserve stable dimensions for toolbars, panel tabs, icon buttons, counters, and segmented controls so labels and loading states do not shift layout.
- Prefer command menus, split buttons, segmented controls, popovers, and contextual panel headers over broad action strips.

## Components

Use shadcn components from `@kandev/ui`. Do not import shadcn primitives from `@/components/ui/*`.

- Buttons: primary for the main scoped command; outline or ghost for secondary actions; icon buttons with tooltips for compact utilities.
- Menus: use for secondary commands, view options, scripts, layouts, and integration actions.
- Segmented controls: use for mutually exclusive view modes or range filters.
- Inputs: search and command inputs should be direct and compact, with loading and dirty states visible.
- Badges: use for counts and semantic state, not decoration.
- Tables and lists: use dense rows with clear hover, selected, focus, empty, and loading states.
- Panels: panel headers own panel-specific actions such as split, close, add panel, run script, open preview, or collapse side regions.
- Dialogs and sheets: reserve for creation flows, confirmations, complex configuration, and mobile overflow navigation.

## Motion

Motion should be short, functional, and reversible.

- Use 150-250 ms transitions for hover, menu, sheet, popover, and panel reveal states.
- Respect reduced motion. Disable nonessential animation when users request reduced motion.
- Avoid decorative motion and page-load choreography.
- Loading motion should be limited to spinners, skeletons, progress, or status indicators where it communicates state.

## Responsive Chrome

Desktop should show the full workbench: global chrome, task/session topbar, dock panels, contextual panel headers, and secondary action menus.

Tablet should preserve orientation and the primary command, then collapse secondary actions into menus or sheets. Search can narrow, but must remain reachable when it is a core workflow.

Mobile should prioritize a small topbar with current product orientation, urgent status, and a menu trigger. View filters, display controls, settings links, and noncritical navigation belong in a sheet. Task/session work should keep primary commands reachable without forcing horizontal overflow.

## Accessibility

Target WCAG AA across light and dark themes. Every icon-only control needs an accessible name or tooltip. Keyboard focus must remain visible in topbars, menus, popovers, dialogs, dock headers, and panel controls. Status must be identifiable without color. Reduced motion should keep workflows usable with no loss of meaning.
