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
import IntegrationsGitLabPage from "@/app/settings/integrations/gitlab/page";
import IntegrationsJiraPage from "@/app/settings/integrations/jira/page";
import IntegrationsLinearPage from "@/app/settings/integrations/linear/page";
import IntegrationsSentryPage from "@/app/settings/integrations/sentry/page";
import IntegrationsSlackPage from "@/app/settings/integrations/slack/page";
import UtilityAgentsSettingsPage from "@/app/settings/utility-agents/page";
import AutomationsPage from "@/app/settings/workspace/[id]/automations/page";
import AutomationEditorPage from "@/app/settings/workspace/[id]/automations/[automationId]/page";
import NewAutomationPage from "@/app/settings/workspace/[id]/automations/new/page";
import WorkspaceEditPage from "@/app/settings/workspace/[id]/page";
import WorkspaceRepositoriesPage from "@/app/settings/workspace/[id]/repositories/page";
import WorkspaceWorkflowsPage from "@/app/settings/workspace/[id]/workflows/page";
import WorkspacesPage from "@/app/settings/workspace/page";
import { GitHubIntegrationPage } from "@/components/github/github-settings";
import { EditorsSettings } from "@/components/settings/editors-settings";
import { GeneralSettings } from "@/components/settings/general-settings";
import { NotificationsSettings } from "@/components/settings/notifications-settings";
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
import { SystemPageShell } from "@/components/settings/system/system-page-shell";
import { UIStateCard } from "@/components/settings/system/ui-state-card";
import { UpdatesCard } from "@/components/settings/system/updates-card";
import { VersionSummaryCard } from "@/components/settings/system/version-summary-card";
import { VoiceModeSettings } from "@/components/settings/voice-mode-settings";

type RouteRenderer = () => React.ReactNode;

const SETTINGS_ROUTES: Record<string, RouteRenderer> = {
  "/settings": () => <GeneralSettings />,
  "/settings/general": () => <GeneralSettings />,
  "/settings/general/editors": () => <EditorsSettings />,
  "/settings/general/notifications": () => <NotificationsSettings />,
  "/settings/general/secrets": () => <SecretsSettings />,
  "/settings/general/sprites": () => <SpritesSettings />,
  "/settings/workspace": () => <WorkspacesPage />,
  "/settings/agents": () => <AgentsSettingsPage />,
  "/settings/automations": () => <AutomationsTopLevelPage />,
  "/settings/executors": () => <ExecutorsPage />,
  "/settings/executor/new": () => <ExecutorCreatePage />,
  "/settings/utility-agents": () => <UtilityAgentsSettingsPage />,
  "/settings/external-mcp": () => <ExternalMcpPage />,
  "/settings/prompts": () => <PromptsSettings />,
  "/settings/voice-mode": () => <VoiceModeSettings />,
  "/settings/integrations": () => <IntegrationsIndexPage />,
  "/settings/integrations/github": () => <GitHubIntegrationPage />,
  "/settings/integrations/gitlab": () => <IntegrationsGitLabPage />,
  "/settings/integrations/jira": () => <IntegrationsJiraPage />,
  "/settings/integrations/linear": () => <IntegrationsLinearPage />,
  "/settings/integrations/sentry": () => <IntegrationsSentryPage />,
  "/settings/integrations/slack": () => <IntegrationsSlackPage />,
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
      description="SQLite path and size, plus VACUUM, optimize, and factory reset."
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
  "/settings/system/updates": renderUpdatesRoute,
  "/settings/changelog": renderUpdatesRoute,
};

export function SettingsRoutes({ pathname }: { pathname: string }) {
  return (
    <SettingsLayoutClient>
      {renderSettingsRoute(normalizeSettingsPath(pathname))}
    </SettingsLayoutClient>
  );
}

export function settingsRouteKey(pathname: string): string {
  return normalizeSettingsPath(pathname);
}

function renderSettingsRoute(pathname: string) {
  const dynamicRoute = renderDynamicSettingsRoute(pathname);
  if (dynamicRoute) return dynamicRoute;
  return SETTINGS_ROUTES[pathname]?.() ?? <SettingsRouteFallback pathname={pathname} />;
}

function renderDynamicSettingsRoute(pathname: string) {
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
      return <WorkspaceRepositoriesPage params={Promise.resolve({ id })} />;
    }
    if (section === "workflows") {
      return <WorkspaceWorkflowsPage params={Promise.resolve({ id })} />;
    }
    return <AutomationsPage params={Promise.resolve({ id })} />;
  }

  const workspaceId = matchSingle(pathname, /^\/settings\/workspace\/([^/]+)$/);
  if (workspaceId) {
    return <WorkspaceEditPage params={Promise.resolve({ id: workspaceId })} />;
  }

  const agentProfile = matchDouble(pathname, /^\/settings\/agents\/([^/]+)\/profiles\/([^/]+)$/);
  if (agentProfile) {
    const [agentId, profileId] = agentProfile;
    return <AgentProfileRoute params={Promise.resolve({ agentId, profileId })} />;
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
  return match?.[1] ? decodeURIComponent(match[1]) : null;
}

function matchDouble(pathname: string, pattern: RegExp): [string, string] | null {
  const match = pathname.match(pattern);
  if (!match?.[1] || !match[2]) return null;
  return [decodeURIComponent(match[1]), decodeURIComponent(match[2])];
}

function normalizeSettingsPath(pathname: string): string {
  if (!pathname || pathname === "/settings/") return "/settings";
  return pathname.length > 1 && pathname.endsWith("/") ? pathname.slice(0, -1) : pathname;
}
