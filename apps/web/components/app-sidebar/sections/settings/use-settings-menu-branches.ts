"use client";

import { useMemo } from "react";

import { useAppStore } from "@/components/state-provider";
import { useAzureDevOpsEnabled } from "@/hooks/domains/azure-devops/use-azure-devops-enabled";
import { useGitHubEnabled } from "@/hooks/domains/github/use-github-enabled";
import { useGitLabEnabled } from "@/hooks/domains/gitlab/use-gitlab-enabled";
import { useHideDisabledIntegrationsInNav } from "@/hooks/domains/integrations/use-hide-disabled-integrations-in-nav";
import { useHideDisabledAgentProfilesInNav } from "@/hooks/domains/settings/use-hide-disabled-agent-profiles-in-nav";
import { useJiraEnabled } from "@/hooks/domains/jira/use-jira-enabled";
import { useLinearEnabled } from "@/hooks/domains/linear/use-linear-enabled";
import { useSentryEnabled } from "@/hooks/domains/sentry/use-sentry-enabled";
import { AGENTS_SETTINGS_HREF } from "@/lib/settings-discovery/catalog/agents";
import { EXECUTORS_SETTINGS_HREF } from "@/lib/settings-discovery/catalog/executors";
import { WORKSPACE_INTEGRATIONS } from "@/lib/settings-discovery/catalog/integrations";
import { WORKSPACES_SETTINGS_HREF } from "@/lib/settings-discovery/catalog/workspaces";
import { detectedAgents, orderAgentsForDisplay } from "@/lib/settings/agent-display-order";
import { isTreeSettingsMenuMode, type SettingsMenuMode } from "@/lib/settings/settings-menu-mode";
import {
  buildAgentsBranch,
  buildBranchRoot,
  buildExecutorsBranch,
  buildWorkspacesBranch,
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
  const isTree = isTreeSettingsMenuMode(mode);
  const workspaces = useAppStore((s) => s.workspaces.items);
  const agents = useAppStore((s) => s.settingsAgents.items);
  const executors = useAppStore((s) => s.executors.items);
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);
  // The Agents page groups detected agents ahead of configured-but-undetected
  // ones; the branch lists the same agents and so must land them in the same
  // order. Before the scan hydrates this is empty and the saved order stands.
  const agentDiscovery = useAppStore((s) => s.agentDiscovery.items);
  const discoveryLoaded = useAppStore((s) => s.agentDiscovery.loaded);
  const visibleIntegrations = useVisibleIntegrationSlugs();
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
        buildWorkspacesBranch(workspaces, activeWorkspaceId, visibleIntegrations),
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
    agents,
    executors,
    agentDiscovery,
    discoveryLoaded,
    visibleIntegrations,
    hideDisabledAgentProfiles,
  ]);
}

/**
 * Which integrations the Integrations branches may list, or `undefined` for all
 * of them.
 *
 * The per-integration enable toggles gate **row visibility**, and only while
 * "Hide disabled integrations from left panel navigation" is on (off by
 * default). Whether an integration is *configured* stays a separate question
 * that gates only the row's badge (`integration-enabled.tsx`) — which makes
 * this deliberately looser than `useNavAvailability`'s
 * `configured && (!hideDisabled || enabled)`: the tree lists an integration you
 * have never connected, the sidebar nav does not.
 *
 * Every toggle is a `localStorage`-backed `useSyncExternalStore` read, so this
 * costs no requests; returning `undefined` on the default path keeps the
 * builder from filtering at all.
 */
function useVisibleIntegrationSlugs(): ReadonlySet<IntegrationSlug> | undefined {
  const { enabled: azureDevOpsEnabled } = useAzureDevOpsEnabled();
  const { enabled: githubEnabled } = useGitHubEnabled();
  const { enabled: gitlabEnabled } = useGitLabEnabled();
  const { enabled: jiraEnabled } = useJiraEnabled();
  const { enabled: linearEnabled } = useLinearEnabled();
  const { enabled: sentryEnabled } = useSentryEnabled();
  const { hideDisabled } = useHideDisabledIntegrationsInNav();

  return useMemo(() => {
    if (!hideDisabled) return undefined;
    // A `Record` over the slug union rather than a list of spreads: a seventh
    // integration then fails to compile here until it declares its toggle,
    // instead of silently vanishing from the tree.
    const enabled: Record<IntegrationSlug, boolean> = {
      "azure-devops": azureDevOpsEnabled,
      github: githubEnabled,
      gitlab: gitlabEnabled,
      jira: jiraEnabled,
      linear: linearEnabled,
      sentry: sentryEnabled,
    };
    return new Set(WORKSPACE_INTEGRATIONS.map(([slug]) => slug).filter((slug) => enabled[slug]));
  }, [
    hideDisabled,
    azureDevOpsEnabled,
    githubEnabled,
    gitlabEnabled,
    jiraEnabled,
    linearEnabled,
    sentryEnabled,
  ]);
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
