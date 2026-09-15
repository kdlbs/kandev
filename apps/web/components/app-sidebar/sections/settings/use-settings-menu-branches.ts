"use client";

import { useMemo } from "react";

import { useAppStore } from "@/components/state-provider";
import { useHideDisabledIntegrationsInNav } from "@/hooks/domains/integrations/use-hide-disabled-integrations-in-nav";
import { useIntegrationEnabledReader } from "@/hooks/domains/integrations/use-integration-enabled";
import { useHideDisabledAgentProfilesInNav } from "@/hooks/domains/settings/use-hide-disabled-agent-profiles-in-nav";
import {
  INTEGRATION_ENABLED_KEYS,
  INTEGRATION_ENABLED_SYNC_EVENTS,
} from "@/lib/integrations/integration-enabled-keys";
import { AGENTS_SETTINGS_HREF } from "@/lib/settings-discovery/catalog/agents";
import { EXECUTORS_SETTINGS_HREF } from "@/lib/settings-discovery/catalog/executors";
import { WORKSPACE_INTEGRATIONS } from "@/lib/settings-discovery/catalog/integrations";
import { WORKSPACES_SETTINGS_HREF } from "@/lib/settings-discovery/catalog/workspaces";
import { detectedAgents, orderAgentsForDisplay } from "@/lib/settings/agent-display-order";
import { resolvePluginIcon } from "@/lib/plugins/icons";
import { usePluginRegistry } from "@/lib/plugins/registry";
import { isTreeSettingsMenuMode, type SettingsMenuMode } from "@/lib/settings/settings-menu-mode";
import {
  buildAgentsBranch,
  buildBranchRoot,
  buildExecutorsBranch,
  buildWorkspacesBranch,
  type BranchIntegrationContribution,
  type IntegrationSlug,
  type SettingsMenuNode,
} from "./settings-menu-branches";
import { SETTINGS_MENU_SECTIONS, type SettingsMenuItem } from "./settings-menu-sections";

/** Branch root per menu row href. Rows with no entry render as plain leaves. */
export type SettingsMenuBranches = Record<string, SettingsMenuNode>;

const NO_BRANCHES: SettingsMenuBranches = {};

const MENU_ITEMS_BY_HREF: ReadonlyMap<string, SettingsMenuItem> = new Map(
  SETTINGS_MENU_SECTIONS.flatMap((section) =>
    section.items.map((item) => [item.href, item] as const),
  ),
);

/**
 * The live branches for the current mode, keyed by the row they hang under.
 *
 * `flat` returns a shared empty object, so the menu does no branch work at all
 * and every row renders exactly as it did before the tree modes existed — the
 * default path stays the cheap one.
 *
 * The three record lists come from the settings bootstrap
 * (`SettingsRouteBootstrap`), which is also what fills the rows' count badges,
 * so a branch never lists records its own row's badge disagrees with.
 */
export function useSettingsMenuBranches(mode: SettingsMenuMode): SettingsMenuBranches {
  const pluginRegistry = usePluginRegistry();
  const isTree = isTreeSettingsMenuMode(mode);
  const workspaces = useAppStore((s) => s.workspaces.items);
  const agents = useAppStore((s) => s.settingsAgents.items);
  const executors = useAppStore((s) => s.executors.items);
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);
  const canvasesEnabled = useAppStore((s) => s.features?.canvases ?? false);
  // The Agents page groups detected agents ahead of configured-but-undetected
  // ones; the branch lists the same agents and so must land them in the same
  // order. Before the scan hydrates this is empty and the saved order stands.
  const agentDiscovery = useAppStore((s) => s.agentDiscovery.items);
  const discoveryLoaded = useAppStore((s) => s.agentDiscovery.loaded);
  const visibleIntegrationsFor = useVisibleIntegrationSlugsFor();
  const { hideDisabled: hideDisabledIntegrations } = useHideDisabledIntegrationsInNav();
  const integrationContributions: BranchIntegrationContribution[] = pluginRegistry
    .getIntegrationSettings()
    .map((integration) => ({
      id: integration.id,
      pluginId: integration.pluginId,
      label: integration.label,
      icon: resolvePluginIcon(integration.icon),
    }));
  const pluginIntegrationEnabled = useMemo(
    () =>
      hideDisabledIntegrations
        ? (integrationId: string, workspaceId: string) =>
            pluginRegistry.getIntegrationEnabled(integrationId, workspaceId)
        : undefined,
    [hideDisabledIntegrations, pluginRegistry],
  );
  const { hideDisabled: hideDisabledAgentProfiles } = useHideDisabledAgentProfilesInNav();

  return useMemo(() => {
    if (!isTree) return NO_BRANCHES;
    const orderedAgents = orderAgentsForDisplay(agentDiscovery, agents);
    // Only once the scan has actually reported: before that, "not in the list"
    // means "not looked yet".
    const detectedNames = discoveryLoaded
      ? new Set(detectedAgents(agentDiscovery).map((agent) => agent.name))
      : undefined;
    return {
      ...branchEntry(
        WORKSPACES_SETTINGS_HREF,
        buildWorkspacesBranch(
          workspaces,
          activeWorkspaceId,
          visibleIntegrationsFor,
          integrationContributions,
          { pluginIntegrationEnabled, canvasesEnabled },
        ),
      ),
      ...branchEntry(
        AGENTS_SETTINGS_HREF,
        buildAgentsBranch(orderedAgents, detectedNames, hideDisabledAgentProfiles),
      ),
      ...branchEntry(EXECUTORS_SETTINGS_HREF, buildExecutorsBranch(executors)),
    };
  }, [
    isTree,
    workspaces,
    activeWorkspaceId,
    canvasesEnabled,
    agents,
    executors,
    agentDiscovery,
    discoveryLoaded,
    visibleIntegrationsFor,
    integrationContributions,
    pluginIntegrationEnabled,
    hideDisabledIntegrations,
    hideDisabledAgentProfiles,
  ]);
}

/**
 * Which integrations a workspace's Integrations branch may list, or `undefined`
 * for all of them.
 *
 * The per-integration enable toggles gate **row visibility**, and only while
 * "Hide disabled integrations from left panel navigation" is on (off by
 * default). Whether an integration is *configured* stays a separate question
 * that gates only the row's badge (`integration-enabled.tsx`) — which makes
 * this deliberately looser than `useNavAvailability`'s
 * `configured && (!hideDisabled || enabled)`: the tree lists an integration you
 * have never connected, the sidebar nav does not.
 *
 * The branch lists every workspace, and the toggles are per workspace, so this
 * resolves them per workspace rather than once — rules of hooks forbid calling
 * `useXEnabled` in a loop, hence the subscribed reader over the key catalog.
 * Every toggle is still a `localStorage` read, so this costs no requests;
 * returning `undefined` on the default path keeps the builder from filtering at
 * all.
 */
function useVisibleIntegrationSlugsFor(): (
  workspaceId: string,
) => ReadonlySet<IntegrationSlug> | undefined {
  const { hideDisabled } = useHideDisabledIntegrationsInNav();
  const readEnabled = useIntegrationEnabledReader(INTEGRATION_ENABLED_SYNC_EVENTS);

  return useMemo(() => {
    if (!hideDisabled) return () => undefined;
    return (workspaceId: string) =>
      new Set(
        WORKSPACE_INTEGRATIONS.map(([slug]) => slug).filter((slug) =>
          readEnabled(INTEGRATION_ENABLED_KEYS[slug], workspaceId),
        ),
      );
  }, [hideDisabled, readEnabled]);
}

/**
 * A branch keyed by its row, or nothing when the row has no records yet. An
 * empty branch would render a chevron that opens onto an empty list, which
 * reads as a loading failure rather than as "you have no workspaces".
 */
function branchEntry(href: string, children: SettingsMenuNode[]): SettingsMenuBranches {
  const item = MENU_ITEMS_BY_HREF.get(href);
  if (!item || children.length === 0) return {};
  return { [href]: buildBranchRoot(item, children) };
}

/** Every branch in one forest — what expansion and active state are keyed over. */
export function useSettingsMenuForest(branches: SettingsMenuBranches): SettingsMenuNode[] {
  return useMemo(() => Object.values(branches), [branches]);
}
