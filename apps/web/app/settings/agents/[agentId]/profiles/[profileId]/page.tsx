import { getAgentProfileMcpConfig } from "@/lib/api";
import type { AgentProfileMcpConfig } from "@/lib/types/http";
import { AgentProfilePage } from "@/components/settings/agent-profile-page";

export default async function AgentProfileRoute({
  params,
}: {
  params: Promise<{ agentId: string; profileId: string }>;
}) {
  const { profileId } = await params;
  let initialMcpConfig: AgentProfileMcpConfig | null | undefined = undefined;

  try {
    initialMcpConfig = await getAgentProfileMcpConfig(profileId, {
      cache: "no-store",
    });
  } catch {
    initialMcpConfig = undefined;
  }

  return <AgentProfilePage initialMcpConfig={initialMcpConfig} />;
}
