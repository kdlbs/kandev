import { SettingsLayoutClient } from "@/components/settings/settings-layout-client";
import { StateHydrator } from "@/components/state-hydrator";
import {
  fetchUserSettings,
  listAgentDiscovery,
  listAgents,
  listAvailableAgents,
  listExecutors,
  listWorkspaces,
} from "@/lib/api";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";

export default function SettingsLayout({ children }: { children: React.ReactNode }) {
  return <SettingsLayoutServer>{children}</SettingsLayoutServer>;
}

async function SettingsLayoutServer({ children }: { children: React.ReactNode }) {
  let initialState = {};
  try {
    // Fetch discovery + available agents alongside the DB-backed list so a
    // hard refresh of /settings/agents/[name] (where no profile exists yet)
    // can still render the agent from the discovered set.
    const [workspaces, executors, agents, discovery, available, userSettingsResponse] =
      await Promise.all([
        listWorkspaces({ cache: "no-store" }),
        listExecutors({ cache: "no-store" }),
        listAgents({ cache: "no-store" }),
        listAgentDiscovery({ cache: "no-store" }),
        listAvailableAgents({ cache: "no-store" }),
        fetchUserSettings({ cache: "no-store" }).catch(() => null),
      ]);
    // Hydrate userSettings into the ROOT store so app-global, override-driven
    // shortcuts (TOGGLE_SIDEBAR, Quick Chat) work on settings routes too. The
    // settings/general page mounts its own nested store for editing; that store
    // is invisible to the root-mounted GlobalCommands/useAppShortcuts.
    const mappedUserSettings = mapUserSettingsResponse(userSettingsResponse);
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
      ...(mappedUserSettings.loaded
        ? {
            userSettings: {
              ...mappedUserSettings,
              workspaceId: workspaces.workspaces[0]?.id ?? null,
            },
          }
        : {}),
    };
  } catch {
    // If any non-settings fetch (workspaces, executors, agents, …) throws, we
    // render with empty initial state — losing the userSettings overrides too.
    // That's acceptable: the page is already degraded (no executors/agents), so
    // override-driven shortcuts being inactive until client hydration is fine.
    initialState = {};
  }

  return (
    <>
      <StateHydrator initialState={initialState} />
      <SettingsLayoutClient>{children}</SettingsLayoutClient>
    </>
  );
}
