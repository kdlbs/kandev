import type { LayoutPanel } from "./types";

// Layout sizing constants (single source of truth)
export const LAYOUT_SIDEBAR_RATIO = 2.5 / 10;
export const LAYOUT_RIGHT_RATIO = 2.5 / 10;
export const LAYOUT_SIDEBAR_MAX_PX = 350;
export const LAYOUT_RIGHT_MAX_PX = 400;

// Well-known group/panel IDs
export const SIDEBAR_GROUP = "group-sidebar";
export const CENTER_GROUP = "group-center";
export const RIGHT_TOP_GROUP = "group-right-top";
export const RIGHT_BOTTOM_GROUP = "group-right-bottom";
export const TERMINAL_DEFAULT_ID = "terminal-default";
export const SIDEBAR_LOCK = "no-drop-target" as const;

/** Fixed panel IDs that can be saved in layout configs. */
export const KNOWN_PANEL_IDS = new Set([
  "sidebar",
  "chat",
  "plan",
  TERMINAL_DEFAULT_ID,
  "browser",
  "vscode",
  "changes",
  "files",
]);

/** Components whose panels are structural and should survive filterEphemeral,
 *  even when the panel ID is dynamically generated. */
export const STRUCTURAL_COMPONENTS = new Set([
  "sidebar",
  "chat",
  "plan",
  "changes",
  "files",
  "terminal",
  "browser",
  "vscode",
]);

/** Default panel configurations for known panels. */
export const PANEL_REGISTRY: Record<string, Omit<LayoutPanel, "id">> = {
  sidebar: { component: "sidebar", title: "Sidebar" },
  chat: { component: "chat", title: "Agent", tabComponent: "permanentTab" },
  plan: { component: "plan", title: "Plan" },
  changes: { component: "changes", title: "Changes" },
  files: { component: "files", title: "Files" },
  browser: { component: "browser", title: "Browser", params: { url: "" } },
  vscode: { component: "vscode", title: "VS Code" },
  [TERMINAL_DEFAULT_ID]: {
    component: "terminal",
    title: "Terminal",
    params: { terminalId: "shell-default" },
  },
};

/** Create a LayoutPanel from the registry by ID. */
export function panel(id: string): LayoutPanel {
  const config = PANEL_REGISTRY[id];
  if (!config) throw new Error(`Unknown panel: ${id}`);
  return { id, ...config };
}
