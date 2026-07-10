import { JiraIntegrationPage } from "@/components/jira/jira-settings";

type IntegrationsJiraPageProps = {
  workspaceId?: string;
};

export default function IntegrationsJiraPage({ workspaceId }: IntegrationsJiraPageProps = {}) {
  return <JiraIntegrationPage workspaceId={workspaceId} />;
}
