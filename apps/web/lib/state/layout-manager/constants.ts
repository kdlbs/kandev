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
export const SIDEBAR_LOCK = "no-drop-target" as const;

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

/** Default panel configurations for known panels. */
export const PANEL_REGISTRY: Record<string, Omit<LayoutPanel, "id">> = {
  chat: { component: "chat", title: "Agent", tabComponent: "permanentTab" },
  plan: { component: "plan", title: "Plan", tabComponent: "planTab" },
  changes: { component: "changes", title: "Changes", tabComponent: "changesTab" },
  files: { component: "files", title: "Files" },
  browser: { component: "browser", title: "Browser", params: { url: "" } },
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
    params: { terminalId: "shell-default" },
  },
  "pr-detail": { component: "pr-detail", title: "PR Details" },
  "mr-detail": { component: "mr-detail", title: "Merge Request" },
  todos: { component: "todos", title: "Todos" },
};

/** Create a LayoutPanel from the registry by ID. */
export function panel(id: string): LayoutPanel {
  const config = PANEL_REGISTRY[id];
  if (!config) throw new Error(`Unknown panel: ${id}`);
  return { id, ...config };
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
