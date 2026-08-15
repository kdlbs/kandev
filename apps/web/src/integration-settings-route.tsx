import IntegrationsIndexPage from "@/app/settings/integrations/page";
import IntegrationsAzureDevOpsPage from "@/app/settings/integrations/azure-devops/page";
import IntegrationsGitHubPage from "@/app/settings/integrations/github/page";
import IntegrationsGitLabPage from "@/app/settings/integrations/gitlab/page";
import IntegrationsJiraPage from "@/app/settings/integrations/jira/page";
import IntegrationsLinearPage from "@/app/settings/integrations/linear/page";
import IntegrationsSentryPage from "@/app/settings/integrations/sentry/page";
import { IntegrationsIndexPage as IntegrationsIndexPageClient } from "@/components/integrations/integrations-index-page";
import { renderPluginIntegrationSettings } from "./plugin-integration-settings-route";

export function renderIntegrationSettingsRoute(section: string | null, workspaceId?: string) {
  switch (section) {
    case null:
      return workspaceId ? (
        <IntegrationsIndexPageClient workspaceId={workspaceId} />
      ) : (
        <IntegrationsIndexPage />
      );
    case "azure-devops":
      return <IntegrationsAzureDevOpsPage workspaceId={workspaceId} />;
    case "github":
      return <IntegrationsGitHubPage workspaceId={workspaceId} />;
    case "gitlab":
      return <IntegrationsGitLabPage workspaceId={workspaceId} />;
    case "jira":
      return <IntegrationsJiraPage workspaceId={workspaceId} />;
    case "linear":
      return <IntegrationsLinearPage workspaceId={workspaceId} />;
    case "sentry":
      return <IntegrationsSentryPage workspaceId={workspaceId} />;
    default:
      return renderPluginIntegrationSettings(section, workspaceId);
  }
}
