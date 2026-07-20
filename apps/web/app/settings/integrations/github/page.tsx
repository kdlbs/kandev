import { GitHubIntegrationPage } from "@/components/github/github-settings";
import { fetchGitHubStatus } from "@/lib/api/domains/github-api";

type IntegrationsGitHubPageProps = {
  workspaceId?: string;
};

export default async function IntegrationsGitHubPage({
  workspaceId,
}: IntegrationsGitHubPageProps = {}) {
  const status = await fetchGitHubStatus({ cache: "no-store" }).catch(() => null);
  return <GitHubIntegrationPage workspaceId={workspaceId} initialStatus={status} />;
}
