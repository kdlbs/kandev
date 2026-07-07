import { copySlackConfig } from "@/lib/api/domains/slack-api";
import { copyJiraConfig } from "@/lib/api/domains/jira-api";
import { copyLinearConfig } from "@/lib/api/domains/linear-api";
import { copySentryConfig } from "@/lib/api/domains/sentry-api";
import { copyGitHubWorkspaceSettings } from "@/lib/api/domains/github-api";

// IntegrationSlug is the set of integration settings pages that support copying
// their per-workspace config to another workspace.
export type IntegrationSlug = "slack" | "jira" | "linear" | "sentry" | "github";

// integrationLabels maps each slug to how the rest of the app spells it.
const integrationLabels: Record<IntegrationSlug, string> = {
  slack: "Slack",
  jira: "Jira",
  linear: "Linear",
  sentry: "Sentry",
  github: "GitHub",
};

// integrationFromPathname returns the integration slug for a settings pathname
// like /settings/integrations/slack, or null when the current page is not a
// copyable integration page.
export function integrationFromPathname(pathname: string): IntegrationSlug | null {
  const match = pathname.match(/^\/settings\/integrations\/([^/]+)/);
  const slug = match?.[1];
  if (slug && slug in integrationLabels) {
    return slug as IntegrationSlug;
  }
  return null;
}

// integrationLabel returns the display name for a slug.
export function integrationLabel(slug: IntegrationSlug): string {
  return integrationLabels[slug];
}

// copyIntegrationConfig copies the config for the given integration from
// sourceWorkspaceId to targetWorkspaceId. GitHub copies settings only (its auth
// is install-wide); every other integration copies config + credentials.
export async function copyIntegrationConfig(
  slug: IntegrationSlug,
  sourceWorkspaceId: string,
  targetWorkspaceId: string,
): Promise<void> {
  const options = { workspaceId: sourceWorkspaceId };
  switch (slug) {
    case "slack":
      await copySlackConfig(targetWorkspaceId, options);
      return;
    case "jira":
      await copyJiraConfig(targetWorkspaceId, options);
      return;
    case "linear":
      await copyLinearConfig(targetWorkspaceId, options);
      return;
    case "sentry":
      await copySentryConfig(targetWorkspaceId, options);
      return;
    case "github":
      await copyGitHubWorkspaceSettings(sourceWorkspaceId, targetWorkspaceId);
      return;
  }
}
