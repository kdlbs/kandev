import {
  fetchUserSettings,
  listAgentDiscovery,
  listAgents,
  listAvailableAgents,
  listExecutors,
} from "@/lib/api/domains/settings-api";
import { listWorkspaces } from "@/lib/api/domains/workspace-api";
import {
  mapWorkspaceItem,
  promoteLegacyWorkspaceSelection,
  readActiveWorkspaceCookie,
  resolveSettingsActiveWorkspaceId,
} from "@/lib/routing/route-bootstrap";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import type { HydrationState } from "@/lib/state/store";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";
import type {
  ListAgentDiscoveryResponse,
  ListAgentsResponse,
  ListAvailableAgentsResponse,
  ListExecutorsResponse,
  ListWorkspacesResponse,
  UserSettingsResponse,
} from "@/lib/types/http";

type SettingsInitialStateData = {
  workspaces: ListWorkspacesResponse["workspaces"];
  executors: ListExecutorsResponse["executors"];
  agents: ListAgentsResponse["agents"];
  discoveryAgents: ListAgentDiscoveryResponse["agents"];
  availableAgents: ListAvailableAgentsResponse["agents"];
  availableTools: NonNullable<ListAvailableAgentsResponse["tools"]>;
  userSettingsResponse: UserSettingsResponse | null;
  agentProfilesVersion?: number;
  /** The workspace this tab already has active; it outranks the shared cookie. */
  currentWorkspaceId?: string | null;
};

export async function loadSettingsInitialState(
  getAgentProfilesVersion: () => number,
  getCurrentWorkspaceId: () => string | null = () => null,
): Promise<HydrationState> {
  // A profile event can arrive while any of these requests are in flight. Do
  // not publish a partial snapshot as loaded; repeat the complete read until
  // it was captured at one stable local generation.
  for (;;) {
    const agentProfilesVersion = getAgentProfilesVersion();
    const [workspaces, executors, agents, discovery, available, userSettingsResponse] =
      await Promise.all([
        listWorkspaces({ cache: "no-store" }).catch(() => ({ workspaces: [] })),
        listExecutors({ cache: "no-store" }).catch(() => ({ executors: [] })),
        listAgents({ cache: "no-store" }).catch(() => ({ agents: [] })),
        listAgentDiscovery({ cache: "no-store" }).catch(() => ({ agents: [] })),
        listAvailableAgents({ cache: "no-store" }).catch(() => ({ agents: [], tools: [] })),
        fetchUserSettings({ cache: "no-store" }).catch(() => null),
      ]);

    const initialState = buildSettingsInitialStateForRoute({
      workspaces: workspaces.workspaces,
      executors: executors.executors,
      agents: agents.agents,
      discoveryAgents: discovery.agents,
      availableAgents: available.agents,
      availableTools: available.tools ?? [],
      userSettingsResponse,
      agentProfilesVersion,
      currentWorkspaceId: getCurrentWorkspaceId(),
    });
    if (getAgentProfilesVersion() === agentProfilesVersion) return initialState;
  }
}

export function buildSettingsInitialStateForRoute({
  workspaces,
  executors,
  agents,
  discoveryAgents,
  availableAgents,
  availableTools,
  userSettingsResponse,
  agentProfilesVersion = 0,
  currentWorkspaceId = null,
}: SettingsInitialStateData): HydrationState {
  const workspaceItems = workspaces.map(mapWorkspaceItem);
  promoteLegacyWorkspaceSelection(workspaceItems);
  const activeWorkspaceId = resolveSettingsActiveWorkspaceId(
    workspaceItems,
    readActiveWorkspaceCookie(),
    userSettingsResponse?.settings?.workspace_id ?? null,
    currentWorkspaceId,
  );
  const mappedUserSettings = mapUserSettingsResponse(userSettingsResponse);

  return {
    workspaces: { items: workspaceItems, activeId: activeWorkspaceId },
    executors: { items: executors },
    agentProfiles: {
      items: agents.flatMap((agent) =>
        agent.profiles.map((profile) => toAgentProfileOption(agent, profile)),
      ),
      version: agentProfilesVersion,
    },
    settingsAgents: { items: agents },
    agentDiscovery: { items: discoveryAgents, loading: false, loaded: true },
    availableAgents: {
      items: availableAgents,
      tools: availableTools,
      loading: false,
      loaded: true,
    },
    settingsData: { executorsLoaded: true, agentsLoaded: true },
    ...(mappedUserSettings.loaded
      ? {
          userSettings: {
            ...mappedUserSettings,
            workspaceId: activeWorkspaceId,
          },
        }
      : {}),
  };
}
