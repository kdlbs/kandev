import { useEffect, useRef } from "react";

import { useAppStoreApi } from "@/components/state-provider";
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
import type { ListWorkspacesResponse, UserSettingsResponse } from "@/lib/types/http";

type SettingsInitialStateData = {
  workspaces: ListWorkspacesResponse["workspaces"];
  executors: Awaited<ReturnType<typeof listExecutors>>["executors"];
  agents: Awaited<ReturnType<typeof listAgents>>["agents"];
  discoveryAgents: Awaited<ReturnType<typeof listAgentDiscovery>>["agents"];
  availableAgents: Awaited<ReturnType<typeof listAvailableAgents>>["agents"];
  availableTools: NonNullable<Awaited<ReturnType<typeof listAvailableAgents>>["tools"]>;
  userSettingsResponse: UserSettingsResponse | null;
  agentProfilesVersion?: number;
};

export function SettingsRouteBootstrap({ pathname }: { pathname: string }) {
  const store = useAppStoreApi();
  const bootstrappedRef = useRef(false);

  useEffect(() => {
    if (bootstrappedRef.current) return;
    bootstrappedRef.current = true;
    let cancelled = false;

    async function bootstrap() {
      const initialState = await loadSettingsInitialState(
        () => store.getState().agentProfiles.version,
      );
      if (cancelled || Object.keys(initialState).length === 0) return;
      const desiredWorkspaceId = initialState.workspaces?.activeId ?? null;
      const workspaceBeforeHydration = store.getState().workspaces.activeId;
      store.getState().hydrate(
        initialState.workspaces
          ? {
              ...initialState,
              workspaces: { ...initialState.workspaces, activeId: workspaceBeforeHydration },
            }
          : initialState,
      );
      if (initialState.workspaces && desiredWorkspaceId !== workspaceBeforeHydration) {
        store.getState().setActiveWorkspace(desiredWorkspaceId);
      }
    }

    void bootstrap();
    return () => {
      cancelled = true;
      bootstrappedRef.current = false;
    };
  }, [pathname, store]);

  return null;
}

export async function loadSettingsInitialState(
  getAgentProfilesVersion: () => number,
): Promise<HydrationState> {
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
}: SettingsInitialStateData): HydrationState {
  const workspaceItems = workspaces.map(mapWorkspaceItem);
  promoteLegacyWorkspaceSelection(workspaceItems);
  const activeWorkspaceId = resolveSettingsActiveWorkspaceId(
    workspaceItems,
    readActiveWorkspaceCookie(),
    userSettingsResponse?.settings?.workspace_id ?? null,
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
