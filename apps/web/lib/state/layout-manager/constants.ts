import type { LayoutPanel } from "./types";

// Layout sizing constants (single source of truth).
// Pinned column max width is computed at runtime — see ./caps.ts.
//
// Ratio applied to dockview width to compute initial defaults when no user
// override exists. The left app sidebar now sits outside dockview, so the
// right pane needs a larger dockview-relative ratio to preserve the old
// laptop proportions after the app sidebar consumes horizontal space.
export const LAYOUT_SIDEBAR_RATIO = 2.5 / 10;
export const LAYOUT_RIGHT_RATIO = 1 / 3;

// Well-known group/panel IDs
export const SIDEBAR_GROUP = "group-sidebar";
export const CENTER_GROUP = "group-center";
export const RIGHT_TOP_GROUP = "group-right-top";
export const RIGHT_BOTTOM_GROUP = "group-right-bottom";
export const TERMINAL_DEFAULT_ID = "terminal-default";
export const SIDEBAR_LOCK = "no-drop-target" as const;

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
  "pr-detail": { component: "pr-detail", title: "Pull Request" },
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
