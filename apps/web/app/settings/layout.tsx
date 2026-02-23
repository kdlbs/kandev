import { SettingsLayoutClient } from "@/components/settings/settings-layout-client";
import { StateProvider } from "@/components/state-provider";
import { listAgents, listExecutors, listWorkspaces } from "@/lib/api";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";

export default function SettingsLayout({ children }: { children: React.ReactNode }) {
  return <SettingsLayoutServer>{children}</SettingsLayoutServer>;
}

async function SettingsLayoutServer({ children }: { children: React.ReactNode }) {
  let initialState = {};
  try {
    const [workspaces, executors, agents] = await Promise.all([
      listWorkspaces({ cache: "no-store" }),
      listExecutors({ cache: "no-store" }),
      listAgents({ cache: "no-store" }),
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
      agentProfiles: {
        items: agents.agents.flatMap((agent) =>
          agent.profiles.map((profile) => toAgentProfileOption(agent, profile)),
        ),
      },
      settingsAgents: {
        items: agents.agents,
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
    <StateProvider initialState={initialState}>
      <SettingsLayoutClient>{children}</SettingsLayoutClient>
    </StateProvider>
  );
}
