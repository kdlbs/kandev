import type {
  ResolvedSettingsDiscoveryItem,
  SettingsDiscoveryContext,
  SettingsDiscoveryDefinition,
} from "./types";
import { SETTINGS_DISCOVERY_DEFINITIONS } from "./catalog";
import { SETTINGS_DISCOVERY_GROUPS } from "./catalog/groups";
import { INTEGRATION_SETTINGS_TARGETS, WORKSPACE_INTEGRATIONS } from "./catalog/integrations";
import {
  agentProfileDiscoveryTarget,
  executorProfileDiscoveryTarget,
  workspaceDiscoveryTarget,
} from "./dynamic-targets";

function workspaceDefinitions(
  workspaces: SettingsDiscoveryContext["workspaces"],
): SettingsDiscoveryDefinition[] {
  const entries: SettingsDiscoveryDefinition[] = [];
  for (const [workspaceIndex, workspace] of workspaces.entries()) {
    const workspaceId = `workspace:${workspace.id}`;
    const href = `/settings/workspaces/${encodeURIComponent(workspace.id)}`;
    const order = 10_000 + workspaceIndex * 100;
    entries.push({
      id: workspaceId,
      kind: "page",
      label: workspace.name,
      parentId: "workspaces",
      groupId: "workspaces",
      href,
      order,
    });
    for (const [index, [suffix, labelKey]] of [
      ["name", "workspaces:name"],
      ["default-executor", "workspaces:defaultExecutor"],
      ["default-agent-profile", "workspaces:defaultAgentProfile"],
    ].entries()) {
      entries.push({
        id: `${workspaceId}:${suffix}`,
        kind: "control",
        labelKey,
        parentId: workspaceId,
        groupId: "workspaces",
        href,
        targetId: workspaceDiscoveryTarget(
          workspace.id,
          suffix as "name" | "default-executor" | "default-agent-profile",
        ),
        order: order + index + 1,
      });
    }
    for (const [index, [suffix, labelKey]] of [
      ["repositories", "sidebar:repositories"],
      ["workflows", "workflows:workflows"],
      ["automations", "common:automations"],
      ["integrations", "common:integrations"],
      ["secrets", "settings:secrets"],
    ].entries()) {
      entries.push({
        id: `${workspaceId}:${suffix}`,
        kind: "page",
        labelKey,
        parentId: workspaceId,
        groupId: "workspaces",
        href: `${href}/${suffix}`,
        order: order + index + 10,
      });
    }
    // One entry per integration service so search reads "<workspace> › Integrations › Jira".
    for (const [index, [slug, label]] of WORKSPACE_INTEGRATIONS.entries()) {
      const integrationId = `${workspaceId}:integration-${slug}`;
      const integrationHref = `${href}/integrations/${slug}`;
      entries.push({
        id: integrationId,
        kind: "page",
        label,
        parentId: `${workspaceId}:integrations`,
        groupId: "workspaces",
        href: integrationHref,
        order: order + index + 20,
      });
      entries.push({
        id: `${integrationId}-connection`,
        kind: "section",
        labelKey: "settings:connection",
        parentId: integrationId,
        groupId: "workspaces",
        href: integrationHref,
        targetId: INTEGRATION_SETTINGS_TARGETS[slug],
        order: order + index + 21,
      });
    }
  }
  return entries;
}

function agentProfileDefinitions(
  agents: SettingsDiscoveryContext["agents"],
): SettingsDiscoveryDefinition[] {
  const entries: SettingsDiscoveryDefinition[] = [];
  for (const [agentIndex, agent] of agents.entries()) {
    for (const [profileIndex, profile] of agent.profiles.entries()) {
      const profileId = `agent-profile:${profile.id}`;
      const href = `/settings/agents/${encodeURIComponent(agent.name)}/profiles/${encodeURIComponent(profile.id)}`;
      const order = 20_000 + agentIndex * 100 + profileIndex * 10;
      entries.push({
        id: profileId,
        kind: "page",
        label: `${profile.agentDisplayName || agent.name} • ${profile.name}`,
        parentId: "agents",
        groupId: "agents",
        href,
        order,
      });
      for (const [index, [suffix, labelKey]] of [
        ["profile-settings", "agents:profileSettings"],
        ["cli-flags", "agents:agentCliFlags"],
        ["environment-variables", "executors:environmentVariables"],
        ["command-preview", "agents:commandPreview"],
      ].entries()) {
        entries.push({
          id: `${profileId}:${suffix}`,
          kind: "section",
          labelKey,
          parentId: profileId,
          groupId: "agents",
          href,
          targetId: agentProfileDiscoveryTarget(
            profile.id,
            suffix as
              | "profile-settings"
              | "cli-flags"
              | "environment-variables"
              | "command-preview",
          ),
          order: order + index + 1,
        });
      }
    }
  }
  return entries;
}

function executorProfileDefinitions(
  executors: SettingsDiscoveryContext["executors"],
): SettingsDiscoveryDefinition[] {
  const entries: SettingsDiscoveryDefinition[] = [];
  for (const [executorIndex, executor] of executors.entries()) {
    for (const [profileIndex, profile] of (executor.profiles ?? []).entries()) {
      const profileId = `executor-profile:${profile.id}`;
      const href = `/settings/executors/${encodeURIComponent(profile.id)}`;
      const order = 30_000 + executorIndex * 100 + profileIndex * 10;
      entries.push({
        id: profileId,
        kind: "page",
        label: profile.name,
        parentId: "executors",
        groupId: "agents",
        href,
        order,
      });
      for (const [index, [suffix, labelKey]] of [
        ["profile-details", "executors:profileDetails"],
        ["environment-variables", "executors:environmentVariables"],
        ["prepare-script", "executors:prepareScript"],
        ["mcp-policy", "executors:mcpPolicy"],
      ].entries()) {
        entries.push({
          id: `${profileId}:${suffix}`,
          kind: "section",
          labelKey,
          parentId: profileId,
          groupId: "agents",
          href,
          targetId: executorProfileDiscoveryTarget(
            profile.id,
            suffix as "profile-details" | "environment-variables" | "prepare-script" | "mcp-policy",
          ),
          order: order + index + 1,
        });
      }
    }
  }
  return entries;
}

function dynamicDefinitions(context: SettingsDiscoveryContext): SettingsDiscoveryDefinition[] {
  return [
    ...workspaceDefinitions(context.workspaces),
    ...agentProfileDefinitions(context.agents),
    ...executorProfileDefinitions(context.executors),
  ];
}

function isVisible(entry: SettingsDiscoveryDefinition, context: SettingsDiscoveryContext) {
  if (entry.requires === "account") return context.showAccount;
  if (entry.requires === "users") return context.showUsers;
  if (entry.requires === "organizations") return context.showOrganizations;
  if (entry.requires === "workspace") return context.workspaces.length > 0;
  return true;
}

function aliasesFor(entry: SettingsDiscoveryDefinition, context: SettingsDiscoveryContext) {
  if (!entry.aliasesKey) return [];
  return context
    .t(entry.aliasesKey)
    .split(",")
    .map((alias) => alias.trim())
    .filter(Boolean);
}

export function resolveSettingsDiscovery(
  context: SettingsDiscoveryContext,
): ResolvedSettingsDiscoveryItem[] {
  const definitions = [...SETTINGS_DISCOVERY_DEFINITIONS, ...dynamicDefinitions(context)].filter(
    (entry) => isVisible(entry, context),
  );
  const byId = new Map(definitions.map((entry) => [entry.id, entry]));
  const groups = new Map(SETTINGS_DISCOVERY_GROUPS.map((group) => [group.id, group]));
  const labelFor = (entry: SettingsDiscoveryDefinition) =>
    entry.label ?? context.t(entry.labelKey ?? entry.id);

  return definitions.map((entry) => {
    const breadcrumb: string[] = [];
    let parent = entry.parentId ? byId.get(entry.parentId) : undefined;
    while (parent) {
      breadcrumb.unshift(labelFor(parent));
      parent = parent.parentId ? byId.get(parent.parentId) : undefined;
    }
    const group = groups.get(entry.groupId);
    const href = entry.targetId
      ? `${entry.href}#${encodeURIComponent(entry.targetId)}`
      : entry.href;
    return {
      id: entry.id,
      kind: entry.kind,
      label: labelFor(entry),
      aliases: aliasesFor(entry, context),
      breadcrumb,
      groupId: entry.groupId,
      groupLabel: context.t(group?.labelKey ?? entry.groupId),
      href,
      targetId: entry.targetId,
      order: (group?.order ?? 99) * 100_000 + entry.order,
    };
  });
}
