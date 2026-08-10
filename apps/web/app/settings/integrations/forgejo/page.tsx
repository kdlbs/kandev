import { ForgejoIntegrationPage } from "@/components/forgejo/forgejo-settings";

export default function IntegrationsForgejoPage({ workspaceId }: { workspaceId?: string } = {}) {
  return <ForgejoIntegrationPage workspaceId={workspaceId} />;
}
