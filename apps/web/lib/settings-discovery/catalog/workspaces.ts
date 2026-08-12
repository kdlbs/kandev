import type { SettingsDiscoveryDefinition } from "../types";

export const WORKSPACES_SETTINGS_HREF = "/settings/workspaces";

export const WORKSPACE_DISCOVERY_DEFINITIONS: SettingsDiscoveryDefinition[] = [
  {
    id: "workspaces",
    kind: "page",
    labelKey: "common:workspaces",
    aliasesKey: "common:commandWorkspaceSettingsKeywords",
    groupId: "workspaces",
    href: WORKSPACES_SETTINGS_HREF,
    order: 200,
  },
];
