import { GitLabIntegrationPage } from "@/components/gitlab/gitlab-settings";

type IntegrationsGitLabPageProps = {
  workspaceId?: string;
};

export default function IntegrationsGitLabPage({ workspaceId }: IntegrationsGitLabPageProps = {}) {
  return <GitLabIntegrationPage workspaceId={workspaceId} />;
}
