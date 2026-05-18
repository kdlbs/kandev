import { SettingsLayoutClient } from "@/components/settings/settings-layout-client";
import { StateHydrator } from "@/components/state-hydrator";
import {
  listAgentDiscovery,
  listAgents,
  listAvailableAgents,
  listExecutors,
  listWorkspaces,
} from "@/lib/api";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";

export default function SettingsLayout({ children }: { children: React.ReactNode }) {
  return <SettingsLayoutServer>{children}</SettingsLayoutServer>;
}

async function SettingsLayoutServer({ children }: { children: React.ReactNode }) {
  let initialState = {};
  try {
    // Fetch discovery + available agents alongside the DB-backed list so a
    // hard refresh of /settings/agents/[name] (where no profile exists yet)
    // can still render the agent from the discovered set.
    const [workspaces, executors, agents, discovery, available] = await Promise.all([
      listWorkspaces({ cache: "no-store" }),
      listExecutors({ cache: "no-store" }),
      listAgents({ cache: "no-store" }),
      listAgentDiscovery({ cache: "no-store" }),
      listAvailableAgents({ cache: "no-store" }),
    ]);
    initialState = {
      workspaces: {
        items: workspaces.workspaces.map((workspace) => ({
          id: workspace.id,
          name: workspace.name,
          default_executor_id: workspace.default_executor_id ?? null,
          default_environment_id: workspace.default_environment_id ?? null,
          default_agent_profile_id: workspace.default_agent_profile_id ?? null,
          default_config_agent_profile_id: workspace.default_config_agent_profile_id ?? null,
        })),
        activeId: workspaces.workspaces[0]?.id ?? null,
      },
      executors: {
        items: executors.executors,
      },
      agentProfiles: {
        items: agents.agents.flatMap((agent) =>
          agent.profiles.map((profile) => toAgentProfileOption(agent, profile)),
        ),
      },
      settingsAgents: {
        items: agents.agents,
      },
      agentDiscovery: {
        items: discovery.agents,
        loading: false,
        loaded: true,
      },
      availableAgents: {
        items: available.agents,
        tools: available.tools ?? [],
        loading: false,
        loaded: true,
      },
      settingsData: {
        executorsLoaded: true,
        agentsLoaded: true,
      },
    };
  } catch {
    initialState = {};
  }

  return (
    <>
      <StateHydrator initialState={initialState} />
      <SettingsLayoutClient>{children}</SettingsLayoutClient>
    </>
  );
}
