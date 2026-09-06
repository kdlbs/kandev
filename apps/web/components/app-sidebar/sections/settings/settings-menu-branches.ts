import type { ComponentType } from "react";
import { getExecutorIcon } from "@/lib/executor-icons";
import { AGENTS_SETTINGS_HREF } from "@/lib/settings-discovery/catalog/agents";
import { EXECUTORS_SETTINGS_HREF } from "@/lib/settings-discovery/catalog/executors";
import { WORKSPACE_INTEGRATIONS } from "@/lib/settings-discovery/catalog/integrations";
import { WORKSPACES_SETTINGS_HREF } from "@/lib/settings-discovery/catalog/workspaces";
import { INTEGRATION_ICONS } from "@/lib/settings/integration-icons";
import {
  executorConnectionSettingsPath,
  executorProfileSettingsPath,
} from "@/lib/settings/executor-settings-routes";
import {
  getWorkspaceSettingsTabs,
  workspaceSettingsHref,
} from "@/lib/settings/workspace-settings-tabs";
import { orderWorkspacesForDisplay } from "@/lib/settings/workspace-display-order";

/**
 * What the settings menu grows *underneath* a row when a tree mode is on.
 *
 * `settings-menu-sections.ts` stays the information architecture — which pages
 * exist and what they are called — and is what `flat` renders on its own. This
 * module answers a different question: for the three rows whose pages own
 * records (Workspaces, Agents, Executors), which records exist right now and
 * where does each one's sub-page live.
 *
 * Every href here is built from the same catalog constants and route tables the
 * pages themselves use (`workspace-settings-tabs.ts`, `WORKSPACE_INTEGRATIONS`),
 * never hand-spelled. A branch that invented its own paths would be a third
 * surface for the menu and the breadcrumb to disagree with, which is the exact
 * failure `settings-menu-sections.ts` exists to prevent.
 *
 * Pure data, no React: the builders take plain records so they can be tested
 * without a store, and labels stay unresolved (`{ key }` for catalog copy,
 * `{ text }` for a record name or brand) so `t()` runs at render.
 */

export type IntegrationSlug = (typeof WORKSPACE_INTEGRATIONS)[number][0];

export type SettingsMenuNodeLabel =
  /** A catalog key, translated at render. */
  | { key: string }
  /** A record name or brand — never translated. */
  | { text: string };

export type SettingsMenuNode = {
  /**
   * Stable identity for expansion state. Derived from record ids rather than
   * hrefs so a renamed record keeps its open branch, and so a stored key for a
   * deleted record is inert rather than matching something else.
   */
  key: string;
  /** The page this node opens, or null when it only discloses its children. */
  href: string | null;
  label: SettingsMenuNodeLabel;
  icon?: ComponentType<{ className?: string }>;
  /**
   * Render this agent's logo as the leading visual instead of `icon`. Carried
   * as a name rather than an element to keep this module free of JSX.
   */
  agentName?: string;
  /**
   * True when the node is something the user created: a workspace, or a profile
   * of an agent or an executor. Agents and executors themselves are **not**
   * records — `Claude` and `Local` ship with kandev and are the same in every
   * install; what the user authors is the profiles underneath them.
   *
   * Not a synonym for depth either: at depth 3 a workspace's name sits beside
   * `GitHub`, which comes from a fixed seven-item catalog. Only a per-node flag
   * separates the two, and the menu spends it on a lighter label weight.
   */
  isUserRecord?: boolean;
  /**
   * A qualifier the row's name cannot carry on its own: the workspace commands
   * act on, or a profile every picker will refuse to offer. One field rather
   * than a flag per badge — a row shows at most one, and the renderer should
   * not have to decide which flag outranks which.
   */
  badge?: "active" | "disabled" | "not-installed";
  /**
   * An integration row, by a catalog slug or a plugin registration id. Whether
   * it is connected is not known here — it takes a network probe or a plugin
   * state lookup — so the node names the integration and the renderer resolves
   * the badge.
   */
  integrationSlug?: IntegrationSlug | string;
  /**
   * Set on the Integrations row: its children need one shared probe of this
   * workspace, run only while the branch is open.
   */
  integrationsWorkspaceId?: string;
  /**
   * Routes this node owns beyond `href` and its `href/` descendants. Needed by
   * nodes with no page of their own (an agent, whose profile pages live under a
   * path the agent itself does not resolve to) and by branch roots, which
   * inherit their row's `activePrefixes` so an alternate spelling of a record
   * route still lights up the branch.
   */
  ownsPrefixes?: readonly string[];
  children?: SettingsMenuNode[];
};

/** The store shapes the builders need, narrowed to what they actually read. */
export type BranchWorkspace = { id: string; name: string };
export type BranchIntegrationContribution = {
  id: string;
  pluginId: string;
  label: string;
  icon: ComponentType<{ className?: string }>;
};
export type BranchAgent = {
  name: string;
  profiles: ReadonlyArray<{
    id: string;
    name: string;
    agentDisplayName?: string;
    /** Absent on older payloads, hence `!== false` rather than `=== true`. */
    enabled?: boolean;
  }>;
};
export type BranchExecutor = {
  id: string;
  name: string;
  type: string;
  profiles?: ReadonlyArray<{ id: string; name: string }>;
};

export type WorkspaceBranchOptions = {
  pluginIntegrationEnabled?: (integrationId: string, workspaceId: string) => boolean | undefined;
  canvasesEnabled?: boolean;
};

/** The menu rows that grow a branch. */
export const BRANCHED_SETTINGS_HREFS = [
  WORKSPACES_SETTINGS_HREF,
  AGENTS_SETTINGS_HREF,
  EXECUTORS_SETTINGS_HREF,
] as const;

/** Expansion key for a branched menu row. Namespaced so it cannot collide
 *  with a record key, which is what makes one flat forest of keys safe. */
export function settingsMenuRowKey(href: string): string {
  return `row:${href}`;
}

/**
 * A branched menu row as a node, so the row and its records live in one tree.
 *
 * Without this the row would need its own parallel notion of "open" and "active"
 * — and the key of a row could not be resolved by the same walk that resolves
 * everything below it. Inheriting `activePrefixes` matters for the alternate
 * route spellings a row already claims: `/settings/executor/<id>` is not under
 * `/settings/executors`, but it is still the Executors row's page.
 */
export function buildBranchRoot(
  item: { href: string; labelKey: string; activePrefixes?: readonly string[] },
  children: SettingsMenuNode[],
): SettingsMenuNode {
  return {
    key: settingsMenuRowKey(item.href),
    href: item.href,
    label: { key: item.labelKey },
    ownsPrefixes: item.activePrefixes ?? [],
    children,
  };
}

function integrationNodes(
  workspaceId: string,
  integrationsHref: string,
  visibleSlugs?: ReadonlySet<IntegrationSlug>,
  contributions: ReadonlyArray<BranchIntegrationContribution> = [],
  pluginEnabled?: (integrationId: string, workspaceId: string) => boolean | undefined,
): SettingsMenuNode[] {
  // Configured status gates the badge only, never the row itself — the branch
  // always lists every integration regardless of credentials, so the filter
  // here hides only disabled ones (intentionally looser than
  // useNavAvailability's `configured && (!hideDisabled || enabled)`).
  const builtIns = WORKSPACE_INTEGRATIONS.filter(
    ([slug]) => !visibleSlugs || visibleSlugs.has(slug),
  ).map(([slug, label]) => ({
    key: `workspace:${workspaceId}:integrations:${slug}`,
    href: `${integrationsHref}/${slug}`,
    label: { text: label },
    icon: INTEGRATION_ICONS[slug],
    integrationSlug: slug,
  }));
  const registered = contributions
    .filter(({ id }) => pluginEnabled?.(id, workspaceId) !== false)
    .map(({ id, label, icon }) => ({
      key: `workspace:${workspaceId}:integrations:${id}`,
      href: `${integrationsHref}/${id}`,
      label: { text: label } as const,
      icon,
      integrationSlug: id,
    }));
  return [...builtIns, ...registered];
}

/**
 * One node per workspace, each holding its settings tabs — and, under
 * Integrations, one node per provider. That is the deepest branch the menu has:
 * Workspaces › a workspace › Integrations › GitHub, which is exactly the
 * breadcrumb those pages already render.
 *
 * Overview is not listed as a tab: it *is* the workspace node's own page, so a
 * child for it would be a row that navigates to its own parent.
 */
export function buildWorkspacesBranch(
  workspaces: ReadonlyArray<BranchWorkspace>,
  activeWorkspaceId?: string | null,
  /**
   * Which integrations a given workspace's Integrations branch may list.
   * Returning `undefined` — the default — lists all of them; a set comes back
   * only while "Hide disabled integrations from left panel navigation" is on.
   * Resolved per workspace because the enable toggles are per workspace: one
   * workspace hiding GitHub must not strip it from its siblings' branches.
   */
  visibleIntegrationSlugsFor?: (workspaceId: string) => ReadonlySet<IntegrationSlug> | undefined,
  integrationContributions: ReadonlyArray<BranchIntegrationContribution> = [],
  { pluginIntegrationEnabled, canvasesEnabled = false }: WorkspaceBranchOptions = {},
): SettingsMenuNode[] {
  return orderWorkspacesForDisplay(workspaces, activeWorkspaceId).map((workspace) => {
    const integrationsHref = workspaceSettingsHref(workspace.id, "integrations");
    const workspaceTabs = getWorkspaceSettingsTabs(canvasesEnabled)
      .filter(({ tab }) => tab !== "overview")
      .map(({ tab, labelKey, icon }) => ({
        key: `workspace:${workspace.id}:${tab}`,
        href: workspaceSettingsHref(workspace.id, tab),
        label: { key: labelKey },
        icon,
        ...(tab === "integrations"
          ? {
              children: integrationNodes(
                workspace.id,
                integrationsHref,
                visibleIntegrationSlugsFor?.(workspace.id),
                integrationContributions,
                pluginIntegrationEnabled,
              ),
              integrationsWorkspaceId: workspace.id,
            }
          : {}),
      }));
    return {
      key: `workspace:${workspace.id}`,
      href: workspaceSettingsHref(workspace.id, "overview"),
      label: { text: workspace.name },
      // No glyph: a folder on every workspace was one repeated mark down a
      // column that already says "workspace" by where it sits. The row still
      // keeps the glyph's width, empty, so its label lines up with its peers.
      isUserRecord: true,
      // Same badge the workspace list and the workspace switcher show, so the
      // menu does not leave you guessing which one commands land in.
      ...(workspace.id === activeWorkspaceId ? { badge: "active" as const } : {}),
      children: [...workspaceTabs],
    };
  });
}

/**
 * One node per installed agent, holding that agent's profiles.
 *
 * The agent node has no `href`: `/settings/agents/<name>` redirects back to the
 * Agents index for an already-saved agent, and a menu row that bounces you
 * somewhere else is worse than one that only expands. `ownsPrefixes` is what
 * still lets a profile route light up its agent's branch.
 *
 * An agent with no profiles is dropped rather than rendered as a node that can
 * neither navigate nor expand.
 */
export function buildAgentsBranch(
  agents: ReadonlyArray<BranchAgent>,
  /**
   * Names the CLI scan currently finds. Omitted while the scan has not run:
   * "not detected yet" is not "not installed", and badging every agent for the
   * moment before discovery lands would be worse than saying nothing.
   */
  detectedNames?: ReadonlySet<string>,
  /**
   * When on ("Hide disabled agent profiles from left panel navigation"),
   * profiles with `enabled === false` are omitted from the branch. `undefined`
   * (the setting off) skips filtering entirely; profiles whose `enabled` is
   * absent are legacy and always stay listed.
   */
  hideDisabled?: boolean,
): SettingsMenuNode[] {
  return agents
    .filter((agent) => agent.profiles.length > 0)
    .map((agent) => {
      const encodedAgent = encodeURIComponent(agent.name);
      const agentHref = `${AGENTS_SETTINGS_HREF}/${encodedAgent}`;
      return {
        key: `agent:${agent.name}`,
        href: null,
        label: { text: agent.profiles[0].agentDisplayName || agent.name },
        agentName: agent.name,
        ownsPrefixes: [`${agentHref}/`],
        ...(detectedNames && !detectedNames.has(agent.name)
          ? { badge: "not-installed" as const }
          : {}),
        // The agent is kandev's; only the profiles under it are the user's.
        children: agent.profiles
          .filter((profile) => !hideDisabled || (profile.enabled ?? true))
          .map((profile) => ({
            key: `agent:${agent.name}:profile:${profile.id}`,
            href: `${agentHref}/profiles/${profile.id}`,
            label: { text: profile.name },
            isUserRecord: true,
            ...(profile.enabled === false ? { badge: "disabled" as const } : {}),
          })),
      };
    });
}

/**
 * One node per executor, holding its profiles.
 *
 * Most executors use the scoped legacy spellings so their executor breadcrumb
 * matches the branch. A configured Kubernetes executor only discloses its
 * profile children: connection, diagnostics, sessions and workload settings
 * all live on that profile page. Its standalone connection route remains
 * reachable only when there is no profile to recover through.
 */
export function buildExecutorsBranch(executors: ReadonlyArray<BranchExecutor>): SettingsMenuNode[] {
  return executors.map((executor) => {
    const profiles = executor.profiles ?? [];
    const executorHref =
      executor.type === "k8s" && profiles.length > 0
        ? null
        : executorConnectionSettingsPath(executor);
    return {
      key: `executor:${executor.id}`,
      href: executorHref,
      label: { text: executor.name },
      icon: getExecutorIcon(executor.type),
      // Same split as agents: the executor ships with kandev, the profiles do not.
      children: profiles.map((profile) => ({
        key: `executor:${executor.id}:profile:${profile.id}`,
        href: executorProfileSettingsPath(executor, profile.id),
        label: { text: profile.name },
        isUserRecord: true,
      })),
    };
  });
}

/** True when `pathname` is this node's page or lives underneath it. */
function nodeOwns(node: SettingsMenuNode, pathname: string): boolean {
  if (node.href && (pathname === node.href || pathname.startsWith(`${node.href}/`))) return true;
  if ((node.ownsPrefixes ?? []).some((prefix) => pathname.startsWith(prefix))) return true;
  return (node.children ?? []).some((child) => nodeOwns(child, pathname));
}

/**
 * The chain of node keys from the outermost branch down to the deepest node
 * that owns `pathname`, or an empty array when the branch owns none of it.
 *
 * Deepest-wins is the whole point: on
 * `/settings/workspaces/ws-1/integrations/github` the workspace node, the
 * Integrations node and the GitHub node all *contain* the route, but only
 * GitHub is the page you are on. The ancestors are the open path; the last key
 * is the one row allowed to mark itself active.
 */
export function findActiveNodePath(
  nodes: ReadonlyArray<SettingsMenuNode>,
  pathname: string,
): string[] {
  for (const node of nodes) {
    if (!nodeOwns(node, pathname)) continue;
    const deeper = findActiveNodePath(node.children ?? [], pathname);
    return [node.key, ...deeper];
  }
  return [];
}

/**
 * The chain of keys from the outermost branch down to `key`, inclusive, or an
 * empty array when no node carries it. Expanding a node has to expand its
 * ancestors too — otherwise the row it reveals has nowhere to appear.
 */
export function findNodeKeyPath(nodes: ReadonlyArray<SettingsMenuNode>, key: string): string[] {
  for (const node of nodes) {
    if (node.key === key) return [node.key];
    const deeper = findNodeKeyPath(node.children ?? [], key);
    if (deeper.length > 0) return [node.key, ...deeper];
  }
  return [];
}

function subtreeKeys(node: SettingsMenuNode): string[] {
  return [node.key, ...(node.children ?? []).flatMap(subtreeKeys)];
}

/**
 * `key` plus every key beneath it, or an empty array when no node carries it.
 * Collapsing a branch drops its descendants from the persistent set as well —
 * they are hidden either way, and leaving them stored reopens a sub-branch the
 * user last saw closed.
 */
export function findSubtreeKeys(nodes: ReadonlyArray<SettingsMenuNode>, key: string): string[] {
  for (const node of nodes) {
    if (node.key === key) return subtreeKeys(node);
    const deeper = findSubtreeKeys(node.children ?? [], key);
    if (deeper.length > 0) return deeper;
  }
  return [];
}
