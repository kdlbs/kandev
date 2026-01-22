import { StateHydrator } from '@/components/state-hydrator';
import { listAvailableAgents, getAgentProfileMcpConfig } from '@/lib/api';
import type { AgentProfileMcpConfig } from '@/lib/types/http';
import { AgentProfilePage } from '@/components/settings/agent-profile-page';

export default async function AgentProfileRoute({
  params,
}: {
  params: Promise<{ agentId: string; profileId: string }>;
}) {
  const { profileId } = await params;
  let initialState = {};
  let initialMcpConfig: AgentProfileMcpConfig | null | undefined = undefined;

  try {
    const [availableAgentsResult, mcpConfigResult] = await Promise.allSettled([
      listAvailableAgents({
        cache: 'no-store',
      }),
      getAgentProfileMcpConfig(profileId, {
        cache: 'no-store',
      }),
    ]);
    const availableAgents =
      availableAgentsResult.status === 'fulfilled'
        ? availableAgentsResult.value
        : { agents: [] };
    const mcpConfig =
      mcpConfigResult.status === 'fulfilled' ? mcpConfigResult.value : null;
    initialState = {
      availableAgents: {
        items: availableAgents.agents ?? [],
        loaded: availableAgentsResult.status === 'fulfilled',
        loading: false,
      },
    };
    initialMcpConfig = mcpConfig;
  } catch {
    initialState = {};
    initialMcpConfig = undefined;
  }

  return (
    <>
      {Object.keys(initialState).length ? <StateHydrator initialState={initialState} /> : null}
      <AgentProfilePage initialMcpConfig={initialMcpConfig} />
    </>
  );
}
