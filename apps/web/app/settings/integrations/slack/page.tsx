import { SlackIntegrationPage } from "@/components/slack/slack-settings";

type IntegrationsSlackPageProps = {
  workspaceId?: string;
};

export default function IntegrationsSlackPage({ workspaceId }: IntegrationsSlackPageProps = {}) {
  return <SlackIntegrationPage workspaceId={workspaceId} />;
}
