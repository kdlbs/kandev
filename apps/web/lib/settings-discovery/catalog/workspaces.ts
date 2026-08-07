import type { SettingsDiscoveryDefinition } from "../types";

export const WORKSPACES_SETTINGS_HREF = "/settings/workspace";

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
  {
    id: "automations",
    kind: "page",
    labelKey: "common:automations",
    parentId: "workspaces",
    groupId: "workspaces",
    href: "/settings/automations",
    order: 210,
  },
];
