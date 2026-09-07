import type { ComponentType } from "react";
import {
  IconArrowsShuffle,
  IconApps,
  IconBolt,
  IconGitBranch,
  IconKey,
  IconLayoutGrid,
  IconPlugConnected,
} from "@tabler/icons-react";

import { WORKSPACES_SETTINGS_HREF } from "@/lib/settings-discovery/catalog/workspaces";

/**
 * The sections of a workspace's settings, as data.
 *
 * Extracted from `WorkspaceSettingsShell` so a second surface can read it
 * without pulling in the shell's hooks: the settings menu's Workspaces branch
 * lists exactly these tabs when the tree modes are on. Two surfaces naming the
 * same sections is precisely how the menu and the page drift apart, so there is
 * one table — the same reason `settings-menu-sections.ts` is data-only.
 */

export type WorkspaceSettingsTab =
  | "overview"
  | "repositories"
  | "workflows"
  | "canvases"
  | "integrations"
  | "automations"
  | "secrets";

export function workspaceSettingsHref(workspaceId: string, tab: WorkspaceSettingsTab): string {
  const base = `${WORKSPACES_SETTINGS_HREF}/${encodeURIComponent(workspaceId)}`;
  return tab === "overview" ? base : `${base}/${tab}`;
}

export type WorkspaceTabSpec = {
  tab: WorkspaceSettingsTab;
  labelKey: string;
  icon: ComponentType<{ className?: string }>;
};

/**
 * `icon` is carried here rather than at the call sites that draw it, so no two
 * surfaces can pick different marks for the same section. Four read this table:
 * the tab strip, each tab's own page heading, the workspace list's section
 * links, and the settings menu's Workspaces branch.
 */
export const WORKSPACE_SETTINGS_TABS: ReadonlyArray<WorkspaceTabSpec> = [
  { tab: "overview", labelKey: "workspaces:overview", icon: IconLayoutGrid },
  { tab: "repositories", labelKey: "sidebar:repositories", icon: IconGitBranch },
  { tab: "workflows", labelKey: "workflows:workflows", icon: IconArrowsShuffle },
  { tab: "canvases", labelKey: "canvases:canvases", icon: IconApps },
  { tab: "integrations", labelKey: "common:integrations", icon: IconPlugConnected },
  { tab: "automations", labelKey: "common:automations", icon: IconBolt },
  { tab: "secrets", labelKey: "settings:secrets", icon: IconKey },
];

/**
 * The workspace settings catalog with optional canvas entries. Keeping the
 * filter here means every settings surface shares the same route, label, and
 * icon definitions while the disabled feature stays absent.
 */
export function getWorkspaceSettingsTabs(
  canvasesEnabled: boolean,
): ReadonlyArray<WorkspaceTabSpec> {
  return canvasesEnabled
    ? WORKSPACE_SETTINGS_TABS
    : WORKSPACE_SETTINGS_TABS.filter(({ tab }) => tab !== "canvases");
}

/** The name and mark for a tab, for the surfaces that render one tab at a time. */
export function workspaceSettingsTabSpec(tab: WorkspaceSettingsTab): WorkspaceTabSpec {
  // Every member of the union has a row, so the fallback is unreachable; it
  // exists so a future tab cannot crash a page before its row is added.
  return WORKSPACE_SETTINGS_TABS.find((entry) => entry.tab === tab) ?? WORKSPACE_SETTINGS_TABS[0];
}
