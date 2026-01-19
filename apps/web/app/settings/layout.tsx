import { SettingsLayoutClient } from '@/components/settings/settings-layout-client';
import { StateProvider } from '@/components/state-provider';
import {
  listAgentDiscovery,
  listAgents,
  listEnvironments,
  listExecutors,
  listWorkspaces,
  fetchUserSettings,
  listPrompts,
  listNotificationProviders,
  listEditors,
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
      userSettings,
      promptsResponse,
      notificationProviders,
      editorsResponse,
    ] = await Promise.all([
      listWorkspaces({ cache: 'no-store' }),
      listExecutors({ cache: 'no-store' }),
      listEnvironments({ cache: 'no-store' }),
      listAgents({ cache: 'no-store' }),
      listAgentDiscovery({ cache: 'no-store' }),
      fetchUserSettings({ cache: 'no-store' }).catch(() => null),
      listPrompts({ cache: 'no-store' }).catch(() => ({ prompts: [] })),
      listNotificationProviders({ cache: 'no-store' }).catch(() => ({
        providers: [],
        apprise_available: false,
        events: [],
      })),
      listEditors({ cache: 'no-store' }).catch(() => ({ editors: [] })),
    ]);
    const settings = userSettings?.settings;
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
      prompts: {
        items: promptsResponse.prompts ?? [],
        loaded: true,
        loading: false,
      },
      editors: {
        items: editorsResponse.editors ?? [],
        loaded: true,
        loading: false,
      },
      notificationProviders: {
        items: notificationProviders.providers ?? [],
        events: notificationProviders.events ?? [],
        appriseAvailable: notificationProviders.apprise_available ?? false,
        loaded: true,
        loading: false,
      },
      userSettings: {
        workspaceId: settings?.workspace_id ?? null,
        boardId: settings?.board_id ?? null,
        repositoryIds: settings?.repository_ids ?? [],
        preferredShell: settings?.preferred_shell ?? null,
        shellOptions: userSettings?.shell_options ?? [],
        defaultEditorId: settings?.default_editor_id ?? null,
        loaded: Boolean(settings),
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
