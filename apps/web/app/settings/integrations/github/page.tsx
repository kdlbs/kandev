import { GitHubIntegrationPage } from "@/components/github/github-settings";

type IntegrationsGitHubPageProps = {
  workspaceId?: string;
};

export default function IntegrationsGitHubPage({ workspaceId }: IntegrationsGitHubPageProps = {}) {
  return <GitHubIntegrationPage workspaceId={workspaceId} />;
}
