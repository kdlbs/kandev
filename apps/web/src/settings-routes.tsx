import { useEffect, useRef, useState, type ReactNode } from "react";

import AgentsSettingsPage from "@/app/settings/agents/page";
import AgentSetupPage from "@/app/settings/agents/[agentId]/page";
import AgentProfileRoute from "@/app/settings/agents/[agentId]/profiles/[profileId]/page";
import AutomationsTopLevelPage from "@/app/settings/automations/page";
import ExecutorEditPage from "@/app/settings/executor/[id]/page";
import ProfileDetailPage from "@/app/settings/executor/[id]/profile/[profileId]/page";
import ExecutorCreatePage from "@/app/settings/executor/new/page";
import ExecutorsPage from "@/app/settings/executors/page";
import ProfileEditPage from "@/app/settings/executors/[profileId]/page";
import CreateProfilePage from "@/app/settings/executors/new/[type]/page";
import SSHExecutorPage from "@/app/settings/executors/ssh/[executorId]/page";
import ExternalMcpPage from "@/app/settings/external-mcp/page";
import IntegrationsIndexPage from "@/app/settings/integrations/page";
import IntegrationsAzureDevOpsPage from "@/app/settings/integrations/azure-devops/page";
import IntegrationsGitLabPage from "@/app/settings/integrations/gitlab/page";
import IntegrationsJiraPage from "@/app/settings/integrations/jira/page";
import IntegrationsLinearPage from "@/app/settings/integrations/linear/page";
import IntegrationsSentryPage from "@/app/settings/integrations/sentry/page";
import IntegrationsSlackPage from "@/app/settings/integrations/slack/page";
import PluginsSettingsPage from "@/app/settings/plugins/page";
import PluginDetailPage from "@/app/settings/plugins/[pluginId]/page";
import StoragePage from "@/app/settings/system/storage/page";
import UtilityAgentsSettingsPage from "@/app/settings/utility-agents/page";
import AutomationsPage from "@/app/settings/workspace/[id]/automations/page";
import AutomationEditorPage from "@/app/settings/workspace/[id]/automations/[automationId]/page";
import NewAutomationPage from "@/app/settings/workspace/[id]/automations/new/page";
import WorkspaceEditPage from "@/app/settings/workspace/[id]/page";
import { WorkspaceRepositoriesClient } from "@/app/settings/workspace/workspace-repositories-client";
import { WorkspaceWorkflowsClient } from "@/app/settings/workspace/workspace-workflows-client";
import WorkspacesPage from "@/app/settings/workspace/page";
import { GitHubIntegrationPage } from "@/components/github/github-settings";
import { useAppStoreApi } from "@/components/state-provider";
import { EditorsSettings } from "@/components/settings/editors-settings";
import {
  AppearanceSettings,
  GeneralSettings,
  KeyboardShortcutsSettings,
  TaskActionsSettings,
} from "@/components/settings/general-settings";
import { NotificationsSettings } from "@/components/settings/notifications-settings";
import { LayoutSettings } from "@/components/settings/layouts/layout-settings";
import { PromptsSettings } from "@/components/settings/prompts-settings";
import { SecretsSettings } from "@/components/settings/secrets-settings";
import { SettingsLayoutClient } from "@/components/settings/settings-layout-client";
import { SpritesSettings } from "@/components/settings/sprites-settings";
import { AboutCard } from "@/components/settings/system/about-card";
import { BackupsTable } from "@/components/settings/system/backups-table";
import { DatabaseStatsCard } from "@/components/settings/system/database-stats-card";
import { DiskUsageCard } from "@/components/settings/system/disk-usage-card";
import { FeatureTogglesSettings } from "@/components/settings/system/feature-toggles-settings";
import { HealthIssuesCard } from "@/components/settings/system/health-issues-card";
import { LicensesList } from "@/components/settings/system/licenses-list";
import { LogViewer } from "@/components/settings/system/log-viewer";
import { SystemPageShell } from "@/components/settings/system/system-page-shell";
import { UIStateCard } from "@/components/settings/system/ui-state-card";
import { UpdatesCard } from "@/components/settings/system/updates-card";
import { VersionSummaryCard } from "@/components/settings/system/version-summary-card";
import { TerminalSettings } from "@/components/settings/terminal-settings";
import { VoiceModeSettings } from "@/components/settings/voice-mode-settings";
import licenses from "@/generated/licenses.json";
import { fetchJson } from "@/lib/api/client";
import {
  PluginErrorBoundary,
  PluginRouteFallback,
} from "@/components/plugins/plugin-error-boundary";
import { pluginRegistry, usePluginRegistry } from "@/lib/plugins/registry";
import { listWorkflows } from "@/lib/api/domains/kanban-api";
import {
  fetchUserSettings,
  listAgentDiscovery,
  listAgents,
  listAvailableAgents,
  listExecutors,
} from "@/lib/api/domains/settings-api";
import { listWorkflowTemplates } from "@/lib/api/domains/workflow-api";
import { listRepositories, listWorkspaces } from "@/lib/api/domains/workspace-api";
import { useRouter } from "@/lib/routing/client-router";
import { safeDecodePathSegment } from "@/lib/routing/path";
import {
  mapWorkspaceItem,
  readActiveWorkspaceCookie,
  resolveSettingsActiveWorkspaceId,
} from "@/lib/routing/route-bootstrap";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import type { AppState } from "@/lib/state/store";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";
import type {
  ListWorkspacesResponse,
  Repository,
  RepositoryScript,
  UserSettingsResponse,
  Workflow,
  WorkflowTemplate,
  Workspace,
} from "@/lib/types/http";
import type { LicenseEntry } from "@/lib/types/system";

type RouteRenderer = () => ReactNode;
type RepositoryWithScripts = Repository & { scripts: RepositoryScript[] };
type WorkspaceRepositoriesRouteState = {
  workspace: Workspace | null;
  repositories: RepositoryWithScripts[];
};
type WorkspaceWorkflowsRouteState = {
  workspace: Workspace | null;
  workflows: Workflow[];
  workflowTemplates: WorkflowTemplate[];
};
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
  "/settings": () => <GeneralSettings />,
  "/settings/general": () => <GeneralSettings />,
  "/settings/general/appearance": () => <AppearanceSettings />,
  "/settings/general/changes-panel": () => <SettingsRedirect to="/settings/general/appearance" />,
  "/settings/general/chat-input": () => (
    <SettingsRedirect to="/settings/general/keyboard-shortcuts" />
  ),
  "/settings/general/editors": () => <EditorsSettings />,
  "/settings/general/keyboard-shortcuts": () => <KeyboardShortcutsSettings />,
  "/settings/general/layouts": () => <LayoutSettings />,
  "/settings/general/notifications": () => <NotificationsSettings />,
  "/settings/general/resource-metrics": () => (
    <SettingsRedirect to="/settings/general/appearance" />
  ),
  "/settings/general/secrets": () => <SecretsSettings />,
  "/settings/general/shell": () => <SettingsRedirect to="/settings/general/terminal" />,
  "/settings/general/sprites": () => <SpritesSettings />,
  "/settings/general/task-actions": () => <TaskActionsSettings />,
  "/settings/general/terminal": () => <TerminalSettings />,
  "/settings/workspace": () => <WorkspacesPage />,
  "/settings/agents": () => <AgentsSettingsPage />,
  "/settings/automations": () => <AutomationsTopLevelPage />,
  "/settings/executors": () => <ExecutorsPage />,
  "/settings/executor/new": () => <ExecutorCreatePage />,
  "/settings/utility-agents": () => <UtilityAgentsSettingsPage />,
  "/settings/external-mcp": () => <ExternalMcpPage />,
  "/settings/prompts": () => <PromptsSettings />,
  "/settings/voice-mode": () => <VoiceModeSettings />,
  "/settings/plugins": () => <PluginsSettingsPage />,
  "/settings/integrations": () => renderIntegrationSettingsRoute(null),
  "/settings/integrations/azure-devops": () => renderIntegrationSettingsRoute("azure-devops"),
  "/settings/integrations/github": () => renderIntegrationSettingsRoute("github"),
  "/settings/integrations/gitlab": () => renderIntegrationSettingsRoute("gitlab"),
  "/settings/integrations/jira": () => renderIntegrationSettingsRoute("jira"),
  "/settings/integrations/linear": () => renderIntegrationSettingsRoute("linear"),
  "/settings/integrations/sentry": () => renderIntegrationSettingsRoute("sentry"),
  "/settings/integrations/slack": () => renderIntegrationSettingsRoute("slack"),
  "/settings/system": () => <SettingsRedirect to="/settings/system/status" />,
  "/settings/system/about": () => (
    <SystemPageShell title="About" description="Version, build metadata, and links.">
      <AboutCard />
    </SystemPageShell>
  ),
  "/settings/system/backups": () => (
    <SystemPageShell
      title="Backups"
      description="VACUUM INTO snapshots stored under <data-dir>/backups/."
    >
      <BackupsTable />
    </SystemPageShell>
  ),
  "/settings/system/database": () => (
    <SystemPageShell
      title="Database"
      description="Database driver, size, and available maintenance controls."
    >
      <DatabaseStatsCard />
    </SystemPageShell>
  ),
  "/settings/system/feature-toggles": () => (
    <SystemPageShell
      title="Feature Toggles"
      description="Enable or disable experimental and diagnostic Kandev features."
    >
      <FeatureTogglesSettings initialFlags={[]} restartCapability={null} />
    </SystemPageShell>
  ),
  "/settings/system/licenses": () => (
    <SystemPageShell
      title="Licenses"
      description="Open-source licenses for every npm and Go dependency shipped with kandev."
    >
      <LicensesList entries={licenseEntries} />
    </SystemPageShell>
  ),
  "/settings/system/logs": () => (
    <SystemPageShell
      title="Logs"
      description="Recent backend log output and downloadable log files."
    >
      <LogViewer />
    </SystemPageShell>
  ),
  "/settings/system/status": () => (
    <SystemPageShell title="Status" description="Health checks, disk usage, and version summary.">
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <HealthIssuesCard />
        <VersionSummaryCard />
      </div>
      <DiskUsageCard />
      <UIStateCard />
    </SystemPageShell>
  ),
  "/settings/system/storage": () => <StoragePage />,
  "/settings/system/updates": renderUpdatesRoute,
  "/settings/changelog": () => <SettingsRedirect to="/settings/system/updates" />,
};

export function SettingsRoutes({ pathname }: { pathname: string }) {
  const normalizedPathname = normalizeSettingsPath(pathname);
  // Subscribe so a plugin settings route registered after first paint
  // (async bundle load) re-resolves without requiring a navigation.
  usePluginRegistry();

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

  const pluginId = matchSingle(pathname, /^\/settings\/plugins\/([^/]+)$/);
  if (pluginId) {
    // A plugin-authored settings route registered at exactly this path
    // (registry.registerSettingsRoute) wins over the first-party detail
    // page, so a plugin can fully replace its own settings surface.
    return (
      renderPluginSettingsRoute(pathname) ?? (
        <PluginDetailPage params={Promise.resolve({ pluginId })} />
      )
    );
  }

  const agentProfile = matchDouble(pathname, /^\/settings\/agents\/([^/]+)\/profiles\/([^/]+)$/);
  if (agentProfile) {
    return <AgentProfileRoute />;
  }

  const agentId = matchSingle(pathname, /^\/settings\/agents\/([^/]+)$/);
  if (agentId) {
    return <AgentSetupPage />;
  }

  const executorProfile = matchDouble(
    pathname,
    /^\/settings\/executor\/([^/]+)\/profile\/([^/]+)$/,
  );
  if (executorProfile) {
    const [id, profileId] = executorProfile;
    return <ProfileDetailPage params={Promise.resolve({ id, profileId })} />;
  }

  const executorId = matchSingle(pathname, /^\/settings\/executor\/([^/]+)$/);
  if (executorId) {
    return <ExecutorEditPage params={Promise.resolve({ id: executorId })} />;
  }

  const profileId = matchSingle(pathname, /^\/settings\/executors\/([^/]+)$/);
  if (profileId) {
    return <ProfileEditPage params={Promise.resolve({ profileId })} />;
  }

  const executorType = matchSingle(pathname, /^\/settings\/executors\/new\/([^/]+)$/);
  if (executorType) {
    return <CreateProfilePage params={Promise.resolve({ type: executorType })} />;
  }

  const sshExecutorId = matchSingle(pathname, /^\/settings\/executors\/ssh\/([^/]+)$/);
  if (sshExecutorId) {
    return <SSHExecutorPage params={Promise.resolve({ executorId: sshExecutorId })} />;
  }

  return null;
}

function renderWorkspaceSettingsRoute(pathname: string) {
  const workspaceIntegration = pathname.match(
    /^\/settings\/workspace\/([^/]+)\/integrations(?:\/([^/]+))?$/,
  );
  if (workspaceIntegration?.[1]) {
    const workspaceId = safeDecodePathSegment(workspaceIntegration[1]);
    const section = workspaceIntegration[2] ? safeDecodePathSegment(workspaceIntegration[2]) : null;
    if (!workspaceId || (workspaceIntegration[2] && !section)) return null;
    return renderIntegrationSettingsRoute(section, workspaceId);
  }

  const workspaceAutomation = matchDouble(
    pathname,
    /^\/settings\/workspace\/([^/]+)\/automations\/([^/]+)$/,
  );
  if (workspaceAutomation) {
    const [id, automationId] = workspaceAutomation;
    if (automationId === "new") {
      return <NewAutomationPage params={Promise.resolve({ id })} />;
    }
    return <AutomationEditorPage params={Promise.resolve({ id, automationId })} />;
  }

  const workspaceSubpage = matchDouble(
    pathname,
    /^\/settings\/workspace\/([^/]+)\/(repositories|workflows|automations)$/,
  );
  if (workspaceSubpage) {
    const [id, section] = workspaceSubpage;
    if (section === "repositories") {
      return <WorkspaceRepositoriesRoute workspaceId={id} />;
    }
    if (section === "workflows") {
      return <WorkspaceWorkflowsRoute workspaceId={id} />;
    }
    return <AutomationsPage workspaceId={id} />;
  }

  const workspaceId = matchSingle(pathname, /^\/settings\/workspace\/([^/]+)$/);
  if (workspaceId) {
    return <WorkspaceEditPage params={Promise.resolve({ id: workspaceId })} />;
  }

  return null;
}

function renderIntegrationSettingsRoute(section: string | null, workspaceId?: string) {
  switch (section) {
    case null:
      return <IntegrationsIndexPage workspaceId={workspaceId} />;
    case "azure-devops":
      return <IntegrationsAzureDevOpsPage workspaceId={workspaceId} />;
    case "github":
      return <GitHubIntegrationPage workspaceId={workspaceId} />;
    case "gitlab":
      return <IntegrationsGitLabPage workspaceId={workspaceId} />;
    case "jira":
      return <IntegrationsJiraPage workspaceId={workspaceId} />;
    case "linear":
      return <IntegrationsLinearPage workspaceId={workspaceId} />;
    case "sentry":
      return <IntegrationsSentryPage workspaceId={workspaceId} />;
    case "slack":
      return <IntegrationsSlackPage workspaceId={workspaceId} />;
    default:
      return null;
  }
}

function renderUpdatesRoute() {
  return (
    <SystemPageShell
      title="Updates"
      description="Current vs latest release plus the full kandev changelog."
    >
      <UpdatesCard />
    </SystemPageShell>
  );
}

function SettingsRedirect({ to }: { to: string }) {
  const router = useRouter();

  useEffect(() => {
    router.replace(to);
  }, [router, to]);

  return null;
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

async function loadSettingsInitialState(): Promise<Partial<AppState>> {
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
}: SettingsInitialStateData): Partial<AppState> {
  const workspaceItems = workspaces.map(mapWorkspaceItem);
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

function WorkspaceRepositoriesRoute({ workspaceId }: { workspaceId: string }) {
  const [state, setState] = useState<WorkspaceRepositoriesRouteState | null>(null);

  useEffect(() => {
    let cancelled = false;
    setState(null);

    loadWorkspaceRepositoriesRoute(workspaceId)
      .catch(() => ({ workspace: null, repositories: [] }))
      .then((nextState) => {
        if (!cancelled) setState(nextState);
      });

    return () => {
      cancelled = true;
    };
  }, [workspaceId]);

  if (!state) return null;
  return (
    <WorkspaceRepositoriesClient workspace={state.workspace} repositories={state.repositories} />
  );
}

function WorkspaceWorkflowsRoute({ workspaceId }: { workspaceId: string }) {
  const [state, setState] = useState<WorkspaceWorkflowsRouteState | null>(null);

  useEffect(() => {
    let cancelled = false;
    setState(null);

    loadWorkspaceWorkflowsRoute(workspaceId)
      .catch(() => ({ workspace: null, workflows: [], workflowTemplates: [] }))
      .then((nextState) => {
        if (!cancelled) setState(nextState);
      });

    return () => {
      cancelled = true;
    };
  }, [workspaceId]);

  if (!state) return null;
  return (
    <WorkspaceWorkflowsClient
      workspace={state.workspace}
      workflows={state.workflows}
      workflowTemplates={state.workflowTemplates}
    />
  );
}

async function loadWorkspaceRepositoriesRoute(
  workspaceId: string,
): Promise<WorkspaceRepositoriesRouteState> {
  const [workspace, repoResponse] = await Promise.all([
    fetchJson<Workspace>(`/api/v1/workspaces/${workspaceId}`, { cache: "no-store" }),
    listRepositories(workspaceId, { includeScripts: true }, { cache: "no-store" }),
  ]);

  return {
    workspace,
    repositories: repoResponse.repositories.map((repository) => ({
      ...repository,
      scripts: repository.scripts ?? [],
    })),
  };
}

async function loadWorkspaceWorkflowsRoute(
  workspaceId: string,
): Promise<WorkspaceWorkflowsRouteState> {
  const [workspace, workflowResponse, templateResponse] = await Promise.all([
    fetchJson<Workspace>(`/api/v1/workspaces/${workspaceId}`, { cache: "no-store" }),
    listWorkflows(workspaceId, { cache: "no-store" }),
    listWorkflowTemplates({ cache: "no-store" }),
  ]);

  return {
    workspace,
    workflows: workflowResponse.workflows ?? [],
    workflowTemplates: templateResponse.templates ?? [],
  };
}

function SettingsRouteFallback({ pathname }: { pathname: string }) {
  return (
    <div className="rounded-md border border-dashed p-6 text-sm text-muted-foreground">
      This settings route is handled by the SPA shell, but its dedicated client page is still being
      ported: <span className="font-mono">{pathname}</span>
    </div>
  );
}

function matchSingle(pathname: string, pattern: RegExp): string | null {
  const match = pathname.match(pattern);
  return safeDecodePathSegment(match?.[1]);
}

function matchDouble(pathname: string, pattern: RegExp): [string, string] | null {
  const match = pathname.match(pattern);
  if (!match?.[1] || !match[2]) return null;
  const first = safeDecodePathSegment(match[1]);
  const second = safeDecodePathSegment(match[2]);
  return first && second ? [first, second] : null;
}

function normalizeSettingsPath(pathname: string): string {
  if (!pathname || pathname === "/settings/") return "/settings";
  return pathname.length > 1 && pathname.endsWith("/") ? pathname.slice(0, -1) : pathname;
}
