import { AzureDevOpsIntegrationPage } from "@/components/azure-devops/azure-devops-settings";

export default function IntegrationsAzureDevOpsPage({
  workspaceId,
}: { workspaceId?: string } = {}) {
  return <AzureDevOpsIntegrationPage workspaceId={workspaceId} />;
}
