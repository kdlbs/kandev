import { useEffect, useRef, type ReactNode } from "react";
import { Trans, useTranslation } from "react-i18next";

import AgentsSettingsPage from "@/app/settings/agents/page";
import AgentsBrowsePage from "@/app/settings/agents/browse/page";
import AgentSetupPage from "@/app/settings/agents/[agentId]/page";
import AgentProfileRoute from "@/app/settings/agents/[agentId]/profiles/[profileId]/page";
import ExecutorCreatePage from "@/app/settings/executor/new/page";
import ExecutorsPage from "@/app/settings/executors/page";
import ProfileEditPage from "@/app/settings/executors/[profileId]/page";
import CreateProfilePage from "@/app/settings/executors/new/[type]/page";
import SSHExecutorPage from "@/app/settings/executors/ssh/[executorId]/page";
import KubernetesExecutorPage from "@/app/settings/executors/k8s/[executorId]/page";
import ExternalMcpPage from "@/app/settings/external-mcp/page";
import PluginsSettingsPage from "@/app/settings/plugins/page";
import PluginDetailPage from "@/app/settings/plugins/[pluginId]/page";
import UtilityAgentsSettingsPage from "@/app/settings/utility-agents/page";
import AutomationsPage from "@/app/settings/workspace/[id]/automations/page";
import AutomationEditorPage from "@/app/settings/workspace/[id]/automations/[automationId]/page";
import NewAutomationPage from "@/app/settings/workspace/[id]/automations/new/page";
import WorkspaceEditPage from "@/app/settings/workspace/[id]/page";
import WorkspacesPage from "@/app/settings/workspace/page";
import Link from "@/components/routing/app-link";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import {
  AppearanceSettings,
  KeyboardShortcutsSettings,
} from "@/components/settings/general-settings";
import { SettingsIndex } from "@/components/settings/settings-index";
import { LegacyExecutorSettingsRoute } from "@/components/settings/legacy-executor-settings-route";
import { readLastSettingsPath } from "@/lib/settings/last-settings-page";
import { SettingsRedirect, useRememberSettingsPath } from "./settings-route-helpers";
import { NotificationsSettings } from "@/components/settings/notifications-settings";
import { LayoutSettings } from "@/components/settings/layouts/layout-settings";
import { PromptsSettings } from "@/components/settings/prompts-settings";
import { SecretsSettings } from "@/components/settings/secrets-settings";
import { SettingsLayoutClient } from "@/components/settings/settings-layout-client";
import { TaskBehaviorSettings } from "@/components/settings/task-behavior-settings";
import { TerminalEditorsSettings } from "@/components/settings/terminal-editors-settings";
import { AboutSettings } from "@/components/settings/system/about-settings";
import { ApiTokens } from "@/components/settings/account/api-tokens";
import { SecuritySettings } from "@/components/settings/account/security-settings";
import { UsersTable } from "@/components/settings/system/users-table";
import { DataStorageSettings } from "@/components/settings/system/data-storage-settings";
import { DiskUsageCard } from "@/components/settings/system/disk-usage-card";
import { FeatureTogglesRoute } from "@/components/settings/system/feature-toggles-route";
import { HealthIssuesCard } from "@/components/settings/system/health-issues-card";
import { SystemPageShell } from "@/components/settings/system/system-page-shell";
import { SystemRouteShell } from "@/components/settings/system/system-route-shell";
import { UIStateCard } from "@/components/settings/system/ui-state-card";
import { UpdatesCard } from "@/components/settings/system/updates-card";
import { VersionSummaryCard } from "@/components/settings/system/version-summary-card";
import {
  WorkspaceSettingsShell,
  type WorkspaceSettingsTab,
} from "@/components/settings/workspaces/workspace-settings-shell";
import licenses from "@/generated/licenses.json";
import {
  APPEARANCE_SETTINGS_HREF,
  KEYBOARD_SHORTCUTS_SETTINGS_HREF,
  LAYOUTS_SETTINGS_HREF,
  NOTIFICATIONS_SETTINGS_HREF,
  TASK_BEHAVIOR_SETTINGS_HREF,
  TERMINAL_EDITORS_SETTINGS_HREF,
} from "@/lib/settings-discovery/catalog/preferences";
import {
  EXECUTORS_SETTINGS_HREF,
  SECRETS_SETTINGS_HREF,
  SYSTEM_ABOUT_SETTINGS_HREF,
  SYSTEM_DATA_STORAGE_SETTINGS_HREF,
  WORKSPACES_SETTINGS_HREF,
} from "@/lib/settings-discovery/catalog";
import {
  PluginErrorBoundary,
  PluginRouteFallback,
} from "@/components/plugins/plugin-error-boundary";
import { pluginRegistry, usePluginRegistry } from "@/lib/plugins/registry";
import {
  fetchUserSettings,
  listAgentDiscovery,
  listAgents,
  listAvailableAgents,
  listExecutors,
} from "@/lib/api/domains/settings-api";
import { listWorkspaces } from "@/lib/api/domains/workspace-api";
import {
  matchSingle,
  matchDouble,
  normalizeSettingsPath,
  safeDecodePathSegment,
} from "@/lib/routing/path";
import {
  mapWorkspaceItem,
  promoteLegacyWorkspaceSelection,
  readActiveWorkspaceCookie,
  resolveSettingsActiveWorkspaceId,
} from "@/lib/routing/route-bootstrap";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import type { HydrationState } from "@/lib/state/store";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";
import type { ListWorkspacesResponse, UserSettingsResponse } from "@/lib/types/http";
import type { LicenseEntry } from "@/lib/types/system";
import { renderIntegrationSettingsRoute } from "./integration-settings-route";
import {
  WorkspaceRepositoriesRoute,
  WorkspaceWorkflowsRoute,
} from "./settings-routes.workspace-data";

type RouteRenderer = () => ReactNode;
type SettingsInitialStateData = {
  workspaces: ListWorkspacesResponse["workspaces"];
  executors: Awaited<ReturnType<typeof listExecutors>>["executors"];
  agents: Awaited<ReturnType<typeof listAgents>>["agents"];
  discoveryAgents: Awaited<ReturnType<typeof listAgentDiscovery>>["agents"];
  availableAgents: Awaited<ReturnType<typeof listAvailableAgents>>["agents"];
  availableTools: NonNullable<Awaited<ReturnType<typeof listAvailableAgents>>["tools"]>;
  userSettingsResponse: UserSettingsResponse | null;
};

const licenseEntries = licenses as LicenseEntry[];

const SETTINGS_ROUTES: Record<string, RouteRenderer> = {
  // The index resolves per surface: the tree as a page on a phone, a handoff to
  // the last visited settings page on desktop. See `SettingsIndex`.
  "/settings": () => <SettingsIndex restoreTo={readLastSettingsPath(SETTINGS_ROUTE_PATHS)} />,
  // Preferences pages (the former General group).
  "/settings/preferences": () => <SettingsRedirect to={APPEARANCE_SETTINGS_HREF} />,
  "/settings/preferences/appearance": () => <AppearanceSettings />,
  "/settings/preferences/keyboard-shortcuts": () => <KeyboardShortcutsSettings />,
  "/settings/preferences/layouts": () => <LayoutSettings />,
  "/settings/preferences/notifications": () => <NotificationsSettings />,
  "/settings/preferences/task-behavior": () => <TaskBehaviorSettings />,
  "/settings/preferences/terminal-editors": () => <TerminalEditorsSettings />,
  // Legacy /settings/general paths, one redirect per page that lived there.
  "/settings/general": () => <SettingsRedirect to={APPEARANCE_SETTINGS_HREF} />,
  "/settings/general/appearance": () => <SettingsRedirect to={APPEARANCE_SETTINGS_HREF} />,
  "/settings/general/changes-panel": () => <SettingsRedirect to={APPEARANCE_SETTINGS_HREF} />,
  "/settings/general/chat-input": () => <SettingsRedirect to={KEYBOARD_SHORTCUTS_SETTINGS_HREF} />,
  "/settings/general/editors": () => <SettingsRedirect to={TERMINAL_EDITORS_SETTINGS_HREF} />,
  "/settings/general/keyboard-shortcuts": () => (
    <SettingsRedirect to={KEYBOARD_SHORTCUTS_SETTINGS_HREF} />
  ),
  "/settings/general/layouts": () => <SettingsRedirect to={LAYOUTS_SETTINGS_HREF} />,
  "/settings/general/message-queue": () => <SettingsRedirect to={TASK_BEHAVIOR_SETTINGS_HREF} />,
  "/settings/general/notifications": () => <SettingsRedirect to={NOTIFICATIONS_SETTINGS_HREF} />,
  "/settings/general/resource-metrics": () => <SettingsRedirect to={APPEARANCE_SETTINGS_HREF} />,
  "/settings/general/secrets": () => <SettingsRedirect to={SECRETS_SETTINGS_HREF} />,
  "/settings/general/shell": () => <SettingsRedirect to={TERMINAL_EDITORS_SETTINGS_HREF} />,
  "/settings/general/sprites": () => <SettingsRedirect to={EXECUTORS_SETTINGS_HREF} />,
  "/settings/general/task-actions": () => <SettingsRedirect to={TASK_BEHAVIOR_SETTINGS_HREF} />,
  "/settings/general/terminal": () => <SettingsRedirect to={TERMINAL_EDITORS_SETTINGS_HREF} />,
  "/settings/workspaces": () => <WorkspacesPage />,
  "/settings/workspace": () => <SettingsRedirect to={WORKSPACES_SETTINGS_HREF} />,
  "/settings/secrets": () => <SecretsSettings />,
  "/settings/agents": () => <AgentsSettingsPage />,
  "/settings/agents/browse": () => <AgentsBrowsePage />,
  "/settings/automations": () => <ActiveWorkspaceSectionRedirect section="automations" />,
  "/settings/executors": () => <ExecutorsPage />,
  "/settings/executor/new": () => <ExecutorCreatePage />,
  "/settings/utility-agents": () => <UtilityAgentsSettingsPage />,
  "/settings/external-mcp": () => <ExternalMcpPage />,
  "/settings/prompts": () => <PromptsSettings />,
  "/settings/plugins": () => <PluginsSettingsPage />,
  // Integrations are per-workspace pages now; the old install-level paths
  // forward into the active workspace's Integrations tab.
  "/settings/integrations": () => <ActiveWorkspaceSectionRedirect section="integrations" />,
  "/settings/integrations/azure-devops": () => (
    <ActiveWorkspaceSectionRedirect section="integrations/azure-devops" />
  ),
  "/settings/integrations/github": () => (
    <ActiveWorkspaceSectionRedirect section="integrations/github" />
  ),
  "/settings/integrations/gitlab": () => (
    <ActiveWorkspaceSectionRedirect section="integrations/gitlab" />
  ),
  "/settings/integrations/jira": () => (
    <ActiveWorkspaceSectionRedirect section="integrations/jira" />
  ),
  "/settings/integrations/linear": () => (
    <ActiveWorkspaceSectionRedirect section="integrations/linear" />
  ),
  "/settings/integrations/sentry": () => (
    <ActiveWorkspaceSectionRedirect section="integrations/sentry" />
  ),
  "/settings/system": () => <SettingsRedirect to="/settings/system/status" />,
  "/settings/system/users": () => (
    <SystemRouteShell titleKey="system:navUsers" descriptionKey="system:usersPageDescription">
      <UsersTable />
    </SystemRouteShell>
  ),
  "/settings/account/security": () => <AccountSecurityRoute />,
  "/settings/account/tokens": () => <AccountTokensRoute />,
  "/settings/system/about": () => (
    <SystemRouteShell titleKey="system:navAbout" descriptionKey="system:aboutPageDescription">
      <AboutSettings licenses={licenseEntries} />
    </SystemRouteShell>
  ),
  "/settings/system/data-storage": () => (
    <SystemRouteShell
      titleKey="system:navDataStorage"
      descriptionKey="system:dataStoragePageDescription"
    >
      <DataStorageSettings />
    </SystemRouteShell>
  ),
  "/settings/system/backups": () => <SettingsRedirect to={SYSTEM_DATA_STORAGE_SETTINGS_HREF} />,
  "/settings/system/database": () => <SettingsRedirect to={SYSTEM_DATA_STORAGE_SETTINGS_HREF} />,
  "/settings/system/feature-toggles": () => (
    <SystemRouteShell
      titleKey="system:navFeatureToggles"
      descriptionKey="system:featureTogglesPageDescription"
    >
      <FeatureTogglesRoute />
    </SystemRouteShell>
  ),
  "/settings/system/licenses": () => <SettingsRedirect to={SYSTEM_ABOUT_SETTINGS_HREF} />,
  "/settings/system/logs": () => <SettingsRedirect to={SYSTEM_DATA_STORAGE_SETTINGS_HREF} />,
  "/settings/system/message-queue": () => <SettingsRedirect to={TASK_BEHAVIOR_SETTINGS_HREF} />,
  "/settings/system/status": () => (
    <SystemRouteShell titleKey="common:status" descriptionKey="system:statusPageDescription">
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <HealthIssuesCard />
        <VersionSummaryCard />
      </div>
      <DiskUsageCard />
      <UIStateCard />
    </SystemRouteShell>
  ),
  "/settings/system/storage": () => <SettingsRedirect to={SYSTEM_DATA_STORAGE_SETTINGS_HREF} />,
  "/settings/system/updates": renderUpdatesRoute,
  "/settings/changelog": () => <SettingsRedirect to="/settings/system/updates" />,
};

/**
 * Every static settings path. Bare `/settings` restores only into this set: the
 * shell renders — and would therefore record — any `/settings/*` path, including
 * ones that fall through to `SettingsRouteFallback`, and the dynamic routes
 * resolve against workspaces, agents and plugins that can be deleted. Derived
 * from the table above so a removed route leaves the set in the same commit.
 */
export const SETTINGS_ROUTE_PATHS: ReadonlySet<string> = new Set(Object.keys(SETTINGS_ROUTES));

export function SettingsRoutes({ pathname }: { pathname: string }) {
  const normalizedPathname = normalizeSettingsPath(pathname);
  // Subscribe so a plugin settings route registered after first paint
  // (async bundle load) re-resolves without requiring a navigation.
  usePluginRegistry();

  useRememberSettingsPath(normalizedPathname, SETTINGS_ROUTE_PATHS);

  return (
    <>
      <SettingsRouteBootstrap pathname={normalizedPathname} />
      <SettingsLayoutClient>{renderSettingsRoute(normalizedPathname)}</SettingsLayoutClient>
    </>
  );
}

export function settingsRouteKey(pathname: string): string {
  return normalizeSettingsPath(pathname);
}

export function renderSettingsRoute(pathname: string) {
  const dynamicRoute = renderDynamicSettingsRoute(pathname);
  if (dynamicRoute) return dynamicRoute;
  const staticRoute = SETTINGS_ROUTES[pathname]?.();
  if (staticRoute) return staticRoute;
  const pluginRoute = renderPluginSettingsRoute(pathname);
  if (pluginRoute) return pluginRoute;
  return <SettingsRouteFallback pathname={pathname} />;
}

/**
 * `/settings/plugins/{id}/...` routes registered by a plugin
 * (`registry.registerSettingsRoute(path, Component)`). Scoped to the
 * `/settings/plugins/` prefix so it never intercepts a first-party path.
 */
function renderPluginSettingsRoute(pathname: string) {
  if (!pathname.startsWith("/settings/plugins/")) return null;
  const match = pluginRegistry.getSettingsRoutes().find((route) => route.path === pathname);
  if (!match) return null;
  return (
    <PluginErrorBoundary
      context={`settings route "${pathname}"`}
      fallback={<PluginRouteFallback />}
    >
      <match.Component />
    </PluginErrorBoundary>
  );
}

function renderDynamicSettingsRoute(pathname: string) {
  const workspaceRoute = renderWorkspaceSettingsRoute(pathname);
  if (workspaceRoute) return workspaceRoute;

  const integrationId = matchSingle(pathname, /^\/settings\/integrations\/([^/]+)$/);
  if (integrationId && pluginRegistry.getIntegrationSetting(integrationId)) {
    const section = pathname.split("/").slice(2).join("/");
    return <ActiveWorkspaceSectionRedirect section={section} />;
  }

  const pluginId = matchSingle(pathname, /^\/settings\/plugins\/([^/]+)$/);
  if (pluginId) {
    // A plugin-authored settings route registered at exactly this path
    // (registry.registerSettingsRoute) wins over the first-party detail
    // page, so a plugin can fully replace its own settings surface.
    return renderPluginSettingsRoute(pathname) ?? <PluginDetailPage pluginId={pluginId} />;
  }

  const agentProfile = matchDouble(pathname, /^\/settings\/agents\/([^/]+)\/profiles\/([^/]+)$/);
  if (agentProfile) {
    return <AgentProfileRoute />;
  }

  const agentId = matchSingle(pathname, /^\/settings\/agents\/([^/]+)$/);
  // "browse" is the static install-catalogue route, not an agent name — same
  // guard shape as /settings/executor/new below.
  if (agentId && agentId !== "browse") {
    return <AgentSetupPage />;
  }

  const executorRoute = renderExecutorSettingsRoute(pathname);
  if (executorRoute) return executorRoute;

  return null;
}

function renderExecutorSettingsRoute(pathname: string): ReactNode {
  const executorProfile = matchDouble(
    pathname,
    /^\/settings\/executor\/([^/]+)\/profile\/([^/]+)$/,
  );
  if (executorProfile) {
    const [id, profileId] = executorProfile;
    return <LegacyExecutorSettingsRoute executorId={id} profileId={profileId} />;
  }

  const executorId = matchSingle(pathname, /^\/settings\/executor\/([^/]+)$/);
  if (executorId && executorId !== "new") {
    return <LegacyExecutorSettingsRoute executorId={executorId} />;
  }
  const profileId = matchSingle(pathname, /^\/settings\/executors\/([^/]+)$/);
  if (profileId) return <ProfileEditPage profileId={profileId} />;
  const executorType = matchSingle(pathname, /^\/settings\/executors\/new\/([^/]+)$/);
  if (executorType) return <CreateProfilePage executorType={executorType} />;
  const sshExecutorId = matchSingle(pathname, /^\/settings\/executors\/ssh\/([^/]+)$/);
  if (sshExecutorId) return <SSHExecutorPage executorId={sshExecutorId} />;
  const kubernetesExecutorId = matchSingle(pathname, /^\/settings\/executors\/k8s\/([^/]+)$/);
  return kubernetesExecutorId ? <KubernetesExecutorPage executorId={kubernetesExecutorId} /> : null;
}

// One component per workspace sub-page tab. A lookup rather than a ternary
// chain: the nested version pushed the enclosing matcher over both the
// cyclomatic and cognitive complexity limits.
// Keep in step with the alternation in the sub-page pattern below.
type WorkspaceSubpageSection = "repositories" | "workflows" | "automations" | "secrets";

const WORKSPACE_SUBPAGE_PAGES: Record<WorkspaceSubpageSection, (id: string) => ReactNode> = {
  repositories: (id) => <WorkspaceRepositoriesRoute workspaceId={id} />,
  workflows: (id) => <WorkspaceWorkflowsRoute workspaceId={id} />,
  automations: (id) => <AutomationsPage workspaceId={id} />,
  secrets: (id) => <SecretsSettings scope="workspace" workspaceId={id} />,
};

function renderWorkspaceIntegrationRoute(match: RegExpMatchArray): ReactNode {
  const workspaceId = safeDecodePathSegment(match[1]);
  const section = match[2] ? safeDecodePathSegment(match[2]) : null;
  if (!workspaceId || (match[2] && !section)) return null;
  const integrationPage = renderIntegrationSettingsRoute(section, workspaceId);
  if (!integrationPage) return null;
  return (
    <WorkspaceSettingsShell workspaceId={workspaceId} activeTab="integrations">
      {integrationPage}
    </WorkspaceSettingsShell>
  );
}

function renderWorkspaceAutomationRoute(id: string, automationId: string): ReactNode {
  const editor =
    automationId === "new" ? (
      <NewAutomationPage workspaceId={id} />
    ) : (
      <AutomationEditorPage workspaceId={id} automationId={automationId} />
    );
  return (
    <WorkspaceSettingsShell workspaceId={id} activeTab="automations">
      {editor}
    </WorkspaceSettingsShell>
  );
}

function renderWorkspaceSettingsRoute(pathname: string): ReactNode {
  // Legacy /settings/workspace/<id>... paths forward to /settings/workspaces/<id>...
  if (pathname.startsWith("/settings/workspace/")) {
    return (
      <SettingsRedirect
        to={pathname.replace("/settings/workspace/", `${WORKSPACES_SETTINGS_HREF}/`)}
      />
    );
  }

  const workspaceIntegration = pathname.match(
    /^\/settings\/workspaces\/([^/]+)\/integrations(?:\/([^/]+))?$/,
  );
  if (workspaceIntegration?.[1]) {
    return renderWorkspaceIntegrationRoute(workspaceIntegration);
  }

  const workspaceAutomation = matchDouble(
    pathname,
    /^\/settings\/workspaces\/([^/]+)\/automations\/([^/]+)$/,
  );
  if (workspaceAutomation) {
    return renderWorkspaceAutomationRoute(workspaceAutomation[0], workspaceAutomation[1]);
  }

  const workspaceSubpage = matchDouble(
    pathname,
    /^\/settings\/workspaces\/([^/]+)\/(repositories|workflows|automations|secrets)$/,
  );
  if (workspaceSubpage) {
    const [id, section] = workspaceSubpage;
    const tab = section as WorkspaceSubpageSection;
    return (
      <WorkspaceSettingsShell workspaceId={id} activeTab={tab as WorkspaceSettingsTab}>
        {WORKSPACE_SUBPAGE_PAGES[tab](id)}
      </WorkspaceSettingsShell>
    );
  }

  const workspaceId = matchSingle(pathname, /^\/settings\/workspaces\/([^/]+)$/);
  if (workspaceId) {
    return (
      <WorkspaceSettingsShell workspaceId={workspaceId} activeTab="overview">
        <WorkspaceEditPage workspaceId={workspaceId} />
      </WorkspaceSettingsShell>
    );
  }

  return null;
}

/**
 * The install-level automations/integrations paths are gone from the menu;
 * anything still linking to them lands on the active workspace's tab. Falls
 * back to the Workspaces list when no workspace exists (or none is active yet).
 */
function ActiveWorkspaceSectionRedirect({ section }: { section: string }) {
  const workspaces = useAppStore((s) => s.workspaces.items);
  const activeId = useAppStore((s) => s.workspaces.activeId);
  // Workspaces hydrate asynchronously (SettingsRouteBootstrap); this flag flips
  // in the same hydrate call, so it separates "still loading" from "none exist".
  const hydrated = useAppStore((s) => s.settingsData.executorsLoaded);
  const workspaceId =
    (activeId && workspaces.some((workspace) => workspace.id === activeId) ? activeId : null) ??
    workspaces[0]?.id ??
    null;
  if (workspaceId === null) {
    if (!hydrated) return null;
    // No workspace exists: the list page owns the "create one" flow.
    return <SettingsRedirect to={WORKSPACES_SETTINGS_HREF} />;
  }
  return (
    <SettingsRedirect
      to={`${WORKSPACES_SETTINGS_HREF}/${encodeURIComponent(workspaceId)}/${section}`}
    />
  );
}

// Components rather than inline JSX so `t()` resolves at render — a t() call
// inside SETTINGS_ROUTES would run at module load and freeze at the boot locale.
function AccountSecurityRoute() {
  const { t } = useTranslation();
  return (
    <SystemPageShell
      title={t("account:securityPageTitle")}
      description={t("account:securityPageDescription")}
    >
      <SecuritySettings />
    </SystemPageShell>
  );
}

function AccountTokensRoute() {
  const { t } = useTranslation();
  return (
    <SystemPageShell
      title={t("account:tokensPageTitle")}
      description={t("account:tokensPageDescription")}
    >
      <ApiTokens />
    </SystemPageShell>
  );
}

function renderUpdatesRoute() {
  return <UpdatesRoute />;
}

function UpdatesRoute() {
  return (
    <SystemRouteShell titleKey="system:navUpdates" descriptionKey="system:updatesPageDescription">
      <p className="text-sm text-muted-foreground">
        <Trans i18nKey="system:updatesNotificationsHint">
          Notification preferences are managed in{" "}
          <Link className="cursor-pointer underline" href="/settings/preferences/notifications">
            Notifications
          </Link>
          .
        </Trans>
      </p>
      <UpdatesCard />
    </SystemRouteShell>
  );
}

function SettingsRouteBootstrap({ pathname }: { pathname: string }) {
  const store = useAppStoreApi();
  const bootstrappedRef = useRef(false);

  useEffect(() => {
    if (bootstrappedRef.current) return;
    bootstrappedRef.current = true;
    let cancelled = false;

    async function bootstrap() {
      const initialState = await loadSettingsInitialState();
      if (!cancelled && Object.keys(initialState).length > 0) {
        store.getState().hydrate(initialState);
      }
    }

    void bootstrap();
    return () => {
      cancelled = true;
      bootstrappedRef.current = false;
    };
  }, [pathname, store]);

  return null;
}

async function loadSettingsInitialState(): Promise<HydrationState> {
  const [workspaces, executors, agents, discovery, available, userSettingsResponse] =
    await Promise.all([
      listWorkspaces({ cache: "no-store" }).catch(() => ({ workspaces: [] })),
      listExecutors({ cache: "no-store" }).catch(() => ({ executors: [] })),
      listAgents({ cache: "no-store" }).catch(() => ({ agents: [] })),
      listAgentDiscovery({ cache: "no-store" }).catch(() => ({ agents: [] })),
      listAvailableAgents({ cache: "no-store" }).catch(() => ({ agents: [], tools: [] })),
      fetchUserSettings({ cache: "no-store" }).catch(() => null),
    ]);

  return buildSettingsInitialStateForRoute({
    workspaces: workspaces.workspaces,
    executors: executors.executors,
    agents: agents.agents,
    discoveryAgents: discovery.agents,
    availableAgents: available.agents,
    availableTools: available.tools ?? [],
    userSettingsResponse,
  });
}

export function buildSettingsInitialStateForRoute({
  workspaces,
  executors,
  agents,
  discoveryAgents,
  availableAgents,
  availableTools,
  userSettingsResponse,
}: SettingsInitialStateData): HydrationState {
  const workspaceItems = workspaces.map(mapWorkspaceItem);
  promoteLegacyWorkspaceSelection(workspaceItems);
  const activeWorkspaceId = resolveSettingsActiveWorkspaceId(
    workspaceItems,
    readActiveWorkspaceCookie(),
    userSettingsResponse?.settings?.workspace_id ?? null,
  );
  const mappedUserSettings = mapUserSettingsResponse(userSettingsResponse);

  return {
    workspaces: { items: workspaceItems, activeId: activeWorkspaceId },
    executors: { items: executors },
    agentProfiles: {
      items: agents.flatMap((agent) =>
        agent.profiles.map((profile) => toAgentProfileOption(agent, profile)),
      ),
      version: 0,
    },
    settingsAgents: { items: agents },
    agentDiscovery: { items: discoveryAgents, loading: false, loaded: true },
    availableAgents: {
      items: availableAgents,
      tools: availableTools,
      loading: false,
      loaded: true,
    },
    settingsData: { executorsLoaded: true, agentsLoaded: true },
    ...(mappedUserSettings.loaded
      ? {
          userSettings: {
            ...mappedUserSettings,
            workspaceId: activeWorkspaceId,
          },
        }
      : {}),
  };
}

function SettingsRouteFallback({ pathname }: { pathname: string }) {
  return (
    <div className="rounded-md border border-dashed p-6 text-sm text-muted-foreground">
      {/* `pathname` is a route string, never translated. */}
      <Trans i18nKey="system:settingsRouteNotPorted" values={{ pathname }}>
        This settings route is handled by the SPA shell, but its dedicated client page is still
        being ported: <span className="font-mono">{pathname}</span>
      </Trans>
    </div>
  );
}
