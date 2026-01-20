import { SettingsLayoutClient } from '@/components/settings/settings-layout-client';
import { StateProvider } from '@/components/state-provider';
import {
  listAgentDiscovery,
  listAgents,
  listEnvironments,
  listExecutors,
  listWorkspaces,
} from '@/lib/http';

export default function SettingsLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <SettingsLayoutServer>{children}</SettingsLayoutServer>
  );
}

async function SettingsLayoutServer({ children }: { children: React.ReactNode }) {
  let initialState = {};
  try {
    const [
      workspaces,
      executors,
      environments,
      agents,
      discovery,
    ] = await Promise.all([
      listWorkspaces({ cache: 'no-store' }),
      listExecutors({ cache: 'no-store' }),
      listEnvironments({ cache: 'no-store' }),
      listAgents({ cache: 'no-store' }),
      listAgentDiscovery({ cache: 'no-store' }),
    ]);
    initialState = {
      workspaces: {
        items: workspaces.workspaces.map((workspace) => ({
          id: workspace.id,
          name: workspace.name,
          default_executor_id: workspace.default_executor_id ?? null,
          default_environment_id: workspace.default_environment_id ?? null,
          default_agent_profile_id: workspace.default_agent_profile_id ?? null,
        })),
        activeId: workspaces.workspaces[0]?.id ?? null,
      },
      executors: {
        items: executors.executors,
      },
      environments: {
        items: environments.environments,
      },
      agentProfiles: {
        items: agents.agents.flatMap((agent) =>
          agent.profiles.map((profile) => ({
            id: profile.id,
            label: `${profile.agent_display_name} • ${profile.name}`,
            agent_id: agent.id,
          }))
        ),
      },
      settingsAgents: {
        items: agents.agents,
      },
      agentDiscovery: {
        items: discovery.agents,
      },
      settingsData: {
        executorsLoaded: true,
        environmentsLoaded: true,
        agentsLoaded: true,
      },
    };
  } catch {
    initialState = {};
  }

  return (
    <StateProvider initialState={initialState}>
      <SettingsLayoutClient>{children}</SettingsLayoutClient>
    </StateProvider>
  );
}
