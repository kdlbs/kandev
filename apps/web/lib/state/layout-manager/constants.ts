import type { LayoutPanel } from "./types";

// Layout sizing constants (single source of truth).
// Pinned column max width is computed at runtime — see ./caps.ts.
//
// Ratio applied to dockview width to compute initial defaults when no user
// override exists. The left app sidebar now sits outside dockview, so the
// right pane uses a slightly narrower dockview-relative ratio to leave more
// room for the main task surface on laptop-sized screens.
export const LAYOUT_SIDEBAR_RATIO = 2.5 / 10;
export const LAYOUT_RIGHT_RATIO = 3 / 10;

// Well-known group/panel IDs
export const SIDEBAR_GROUP = "group-sidebar";
export const CENTER_GROUP = "group-center";
export const RIGHT_TOP_GROUP = "group-right-top";
export const RIGHT_BOTTOM_GROUP = "group-right-bottom";
export const TERMINAL_DEFAULT_ID = "terminal-default";
/**
 * Read-only output panel for the repository dev script. It renders the dev
 * process's buffered output rather than a PTY, so it deliberately carries no
 * `terminalId` — that is what keeps `handleTerminalPanelClosed` from trying to
 * park or destroy a user shell when the tab is closed.
 */
export const DEV_SERVER_PANEL_ID = "dev-server";
export const SIDEBAR_LOCK = "no-drop-target" as const;

export const PROMPT_HISTORY_PANEL_ID = "prompt-history";

/** Canonical single-instance panels supported by reusable layout profiles. */
export const REUSABLE_PANEL_IDS = [
  "chat",
  "files",
  "changes",
  "pr-detail",
  TERMINAL_DEFAULT_ID,
  "plan",
  "browser",
  "vscode",
  "todos",
  PROMPT_HISTORY_PANEL_ID,
] as const;
export type ReusablePanelId = (typeof REUSABLE_PANEL_IDS)[number];

/** Fixed panel IDs that can be saved in layout configs. */
export const KNOWN_PANEL_IDS = new Set([
  "chat",
  "plan",
  TERMINAL_DEFAULT_ID,
  "browser",
  "vscode",
  "changes",
  "files",
  "pr-detail",
  "mr-detail",
  "todos",
  DEV_SERVER_PANEL_ID,
  PROMPT_HISTORY_PANEL_ID,
]);

/** Components whose panels are structural and should survive filterEphemeral,
 *  even when the panel ID is dynamically generated. */
export const STRUCTURAL_COMPONENTS = new Set([
  "chat",
  "plan",
  "changes",
  "files",
  "terminal",
  "browser",
  "vscode",
  "pr-detail",
  "mr-detail",
  // Every plugin-contributed task panel shares this one generic component
  // name (see lib/state/layout-manager/plugin-panels.ts) — structural
  // regardless of which plugin registered it.
  "plugin-panel",
]);

/**
 * Default panel configurations for known panels.
 *
 * `title` and `titleKey` are deliberately two different things:
 *
 *   - `title` is the CANONICAL, always-English value. `toSerializedDockview`
 *     writes it into the stored layout JSON, so it has to be locale-independent
 *     — otherwise the locale a layout happened to be saved in is frozen into it
 *     and survives every later switch.
 *   - `titleKey` is what the user reads. `panelTitle()` resolves it whenever a
 *     panel is built or a saved layout is restored, so the tab always shows the
 *     ACTIVE locale regardless of which one wrote the layout.
 *
 * This is the key/persisted-value split docs/i18n.md prescribes for a display
 * string that is also stored. Before it, two of these titles were translated at
 * creation (`t("task:browser")`, `t("common:todos")`) and the rest were not, so
 * the tab bar was half-translated AND persisted whichever locale created it.
 *
 * `vscode` has no `titleKey` on purpose: "VS Code" is a product name, and
 * `panelTitle()` falls back to `title` when a key is absent.
 */
// i18n-exempt: canonical English persisted in saved layouts; `titleKey` beside it is what renders.
export const PANEL_REGISTRY: Record<string, Omit<LayoutPanel, "id"> & { titleKey?: string }> = {
  chat: {
    component: "chat",
    title: "Agent",
    titleKey: "task:panelAgent",
    tabComponent: "permanentTab",
  },
  plan: { component: "plan", title: "Plan", titleKey: "task:panelPlan", tabComponent: "planTab" },
  changes: {
    component: "changes",
    title: "Changes",
    titleKey: "task:panelChanges",
    tabComponent: "changesTab",
  },
  files: { component: "files", title: "Files", titleKey: "task:panelFiles" },
  browser: {
    component: "browser",
    title: "Browser",
    titleKey: "task:browser",
    params: { url: "" },
  },
  vscode: { component: "vscode", title: "VS Code" },
  [TERMINAL_DEFAULT_ID]: {
    component: "terminal",
    // terminalTab is the custom dockview tab that adds the `#N` badge
    // and a right-click rename/destroy menu. The hook
    // useEnsureDefaultTerminalOrdinary migrates the legacy
    // `shell-default` PTY into a DB-backed ordinary terminal on
    // session-page mount so the badge logic has a seq to display.
    tabComponent: "terminalTab",
    title: "Terminal",
    titleKey: "task:panelTerminal",
    params: { terminalId: "shell-default" },
  },
  [DEV_SERVER_PANEL_ID]: {
    component: "terminal",
    title: "Dev Server",
    titleKey: "task:devServer",
    params: { type: DEV_SERVER_PANEL_ID },
  },
  "pr-detail": { component: "pr-detail", title: "PR Details", titleKey: "task:panelPrDetails" },
  "mr-detail": {
    component: "mr-detail",
    title: "Merge Request",
    titleKey: "task:panelMergeRequest",
  },
  todos: { component: "todos", title: "Todos", titleKey: "common:todos" },
  [PROMPT_HISTORY_PANEL_ID]: {
    component: PROMPT_HISTORY_PANEL_ID,
    title: "Prompt History",
    titleKey: "task:promptHistory",
  },
};

/**
 * `panelTitle()` — the locale-resolved counterpart of `canonicalPanelTitle()` —
 * lives in `./panel-title.ts`. It needs `@/lib/i18n`, and this module must stay
 * loadable from plain Node because e2e specs import constants from it; see that
 * file's comment.
 */

/**
 * Registry entry for a panel.
 *
 * Falls back to the COMPONENT when the id is not an exact key, because several
 * panel families mint dynamic ids from their component — `browser:<url>`,
 * `pr-detail|<key>`, `mr-detail|<key>`. Matching on the id alone left those
 * outside the canonical/localized split entirely: they were created with a
 * localized title, captured verbatim into the saved layout, and then could not
 * be re-localized on restore, so the layout showed whichever locale created it.
 */
export function panelRegistryEntry(
  id: string,
  component?: string,
): (typeof PANEL_REGISTRY)[string] | undefined {
  return PANEL_REGISTRY[id] ?? (component ? PANEL_REGISTRY[component] : undefined);
}

/** The canonical English title stored in a saved layout, if the panel is known. */
export function canonicalPanelTitle(id: string, component?: string): string | undefined {
  return panelRegistryEntry(id, component)?.title;
}

/**
 * Create a LayoutPanel from the registry by ID.
 *
 * The title is canonical English: a `LayoutPanel` is the shape that gets
 * persisted, and `toSerializedDockview` localizes it when it reaches dockview.
 */
export function panel(id: string): LayoutPanel {
  const config = PANEL_REGISTRY[id];
  if (!config) throw new Error(`Unknown panel: ${id}`);
  const { titleKey: _titleKey, ...rest } = config;
  return { id, ...rest };
}

/** Generate panel config for a session tab. */
export function sessionPanelConfig(
  sessionId: string,
  title: string,
  isPrimary: boolean,
): LayoutPanel & { tabComponent: string; params: Record<string, unknown> } {
  return {
    id: `session:${sessionId}`,
    component: "chat",
    title,
    tabComponent: "sessionTab",
    params: { sessionId, isPrimary },
  };
}
