import type { SettingsDiscoveryDefinition } from "../types";

export const AGENTS_SETTINGS_HREF = "/settings/agents";

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
];
