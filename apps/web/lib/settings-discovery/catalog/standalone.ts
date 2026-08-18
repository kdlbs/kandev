import type { SettingsDiscoveryDefinition } from "../types";

export const PROMPTS_SETTINGS_HREF = "/settings/prompts";
export const UTILITY_AGENTS_SETTINGS_HREF = "/settings/utility-agents";
export const EXTERNAL_MCP_SETTINGS_HREF = "/settings/external-mcp";
export const PLUGINS_SETTINGS_HREF = "/settings/plugins";
export const SECRETS_SETTINGS_HREF = "/settings/secrets";
const UTILITY_AGENTS_DISCOVERY_ID = "utility-agents";
export const SETTINGS_TARGETS = {
  utilityDefaultModel: "setting-utility-default-model",
  utilityActions: "setting-utility-actions",
  utilityCustomAgents: "setting-utility-custom-agents",
  externalMcpEndpoints: "setting-external-mcp-endpoints",
  externalMcpSnippets: "setting-external-mcp-snippets",
} as const;

export const STANDALONE_DISCOVERY_DEFINITIONS: SettingsDiscoveryDefinition[] = [
  {
    id: "prompts",
    kind: "page",
    labelKey: "common:prompts",
    aliasesKey: "common:commandPromptsSettingsKeywords",
    groupId: "agents",
    href: PROMPTS_SETTINGS_HREF,
    order: 500,
  },
  {
    id: UTILITY_AGENTS_DISCOVERY_ID,
    kind: "page",
    labelKey: "settings:utilityAgents",
    groupId: "agents",
    href: UTILITY_AGENTS_SETTINGS_HREF,
    order: 520,
  },
  {
    id: "utility-default-model",
    kind: "control",
    labelKey: "settings:utilityDefaultModelTitle",
    aliasesKey: "settings:discoveryAliasesUtilityDefaultModel",
    parentId: UTILITY_AGENTS_DISCOVERY_ID,
    groupId: "agents",
    href: UTILITY_AGENTS_SETTINGS_HREF,
    targetId: SETTINGS_TARGETS.utilityDefaultModel,
    order: 521,
  },
  {
    id: "utility-actions",
    kind: "section",
    labelKey: "settings:utilityActionsTitle",
    aliasesKey: "settings:discoveryAliasesUtilityActions",
    parentId: UTILITY_AGENTS_DISCOVERY_ID,
    groupId: "agents",
    href: UTILITY_AGENTS_SETTINGS_HREF,
    targetId: SETTINGS_TARGETS.utilityActions,
    order: 522,
  },
  {
    id: "utility-custom-agents",
    kind: "section",
    labelKey: "settings:utilityCustomAgentsTitle",
    parentId: UTILITY_AGENTS_DISCOVERY_ID,
    groupId: "agents",
    href: UTILITY_AGENTS_SETTINGS_HREF,
    targetId: SETTINGS_TARGETS.utilityCustomAgents,
    order: 523,
  },
  {
    id: "secrets",
    kind: "page",
    labelKey: "settings:globalSecrets",
    groupId: "workspaces",
    href: SECRETS_SETTINGS_HREF,
    order: 525,
  },
  {
    id: "external-mcp",
    kind: "page",
    labelKey: "common:externalMcp",
    groupId: "workspaces",
    href: EXTERNAL_MCP_SETTINGS_HREF,
    order: 530,
  },
  {
    id: "external-mcp-endpoints",
    kind: "section",
    labelKey: "settings:externalMcpEndpoints",
    aliasesKey: "settings:discoveryAliasesExternalMcpEndpoints",
    parentId: "external-mcp",
    groupId: "workspaces",
    href: EXTERNAL_MCP_SETTINGS_HREF,
    targetId: SETTINGS_TARGETS.externalMcpEndpoints,
    order: 531,
  },
  {
    id: "external-mcp-snippets",
    kind: "section",
    labelKey: "settings:externalMcpSnippets",
    parentId: "external-mcp",
    groupId: "workspaces",
    href: EXTERNAL_MCP_SETTINGS_HREF,
    targetId: SETTINGS_TARGETS.externalMcpSnippets,
    order: 532,
  },
  {
    id: "plugins",
    kind: "page",
    labelKey: "common:plugins",
    groupId: "workspaces",
    href: PLUGINS_SETTINGS_HREF,
    order: 540,
  },
];
