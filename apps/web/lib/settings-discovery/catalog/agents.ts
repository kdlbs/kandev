import type { SettingsDiscoveryDefinition } from "../types";

export const AGENTS_SETTINGS_HREF = "/settings/agents";
export const AGENTS_BROWSE_SETTINGS_HREF = `${AGENTS_SETTINGS_HREF}/browse`;

export const AGENT_DISCOVERY_DEFINITIONS: SettingsDiscoveryDefinition[] = [
  {
    id: "agents",
    kind: "page",
    labelKey: "common:agents",
    aliasesKey: "common:commandAgentsSettingsKeywords",
    groupId: "agents",
    href: AGENTS_SETTINGS_HREF,
    order: 300,
  },
  {
    id: "agents-browse",
    kind: "page",
    labelKey: "agents:browseAvailableAgents",
    parentId: "agents",
    groupId: "agents",
    href: AGENTS_BROWSE_SETTINGS_HREF,
    order: 310,
  },
];
