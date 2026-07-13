import { LinearIntegrationPage } from "@/components/linear/linear-settings";

type IntegrationsLinearPageProps = {
  workspaceId?: string;
};

export default function IntegrationsLinearPage({ workspaceId }: IntegrationsLinearPageProps = {}) {
  return <LinearIntegrationPage workspaceId={workspaceId} />;
}
