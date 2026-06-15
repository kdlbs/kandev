import AgentsSettingsPage from "@/app/settings/agents/page";
import ExternalMcpPage from "@/app/settings/external-mcp/page";
import IntegrationsIndexPage from "@/app/settings/integrations/page";
import IntegrationsGitLabPage from "@/app/settings/integrations/gitlab/page";
import IntegrationsJiraPage from "@/app/settings/integrations/jira/page";
import IntegrationsLinearPage from "@/app/settings/integrations/linear/page";
import IntegrationsSentryPage from "@/app/settings/integrations/sentry/page";
import IntegrationsSlackPage from "@/app/settings/integrations/slack/page";
import UtilityAgentsSettingsPage from "@/app/settings/utility-agents/page";
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
  return SETTINGS_ROUTES[pathname]?.() ?? <SettingsRouteFallback pathname={pathname} />;
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

function normalizeSettingsPath(pathname: string): string {
  if (!pathname || pathname === "/settings/") return "/settings";
  return pathname.length > 1 && pathname.endsWith("/") ? pathname.slice(0, -1) : pathname;
}
