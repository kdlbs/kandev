import { useCallback, useEffect, useMemo, useSyncExternalStore } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { listAvailableAgents, listExecutors } from "@/lib/api";
import {
  agentListAuthScopeIdentity,
  getAgentListResourceScope,
  type AgentListSnapshot,
} from "./agent-list-resource";

const EMPTY_AGENT_LIST_SNAPSHOT: AgentListSnapshot = {
  response: null,
  profileVersion: -1,
  loading: false,
  loaded: false,
  error: null,
};

function useAgentListResource(enabled: boolean) {
  const store = useAppStoreApi();
  const identity = useAppStore(agentListAuthScopeIdentity);
  const scope = useMemo(
    () => (enabled ? getAgentListResourceScope(store) : null),
    [enabled, identity, store],
  );
  const subscribe = useCallback(
    (listener: () => void) => (enabled && scope ? scope.subscribe(listener) : () => {}),
    [enabled, scope],
  );
  const getSnapshot = useCallback(() => scope?.getSnapshot() ?? EMPTY_AGENT_LIST_SNAPSHOT, [scope]);
  const snapshot = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
  return { scope, snapshot };
}

export function useSettingsData(enabled = true) {
  const executors = useAppStore((state) => state.executors.items);
  const settingsAgents = useAppStore((state) => state.settingsAgents.items);
  const settingsData = useAppStore((state) => state.settingsData);
  const availableAgents = useAppStore((state) => state.availableAgents);
  const setExecutors = useAppStore((state) => state.setExecutors);
  const setSettingsAgents = useAppStore((state) => state.setSettingsAgents);
  const setAgentProfiles = useAppStore((state) => state.setAgentProfiles);
  const setAvailableAgents = useAppStore((state) => state.setAvailableAgents);
  const setAvailableAgentsLoading = useAppStore((state) => state.setAvailableAgentsLoading);
  const setSettingsData = useAppStore((state) => state.setSettingsData);
  const { scope: agentListResource, snapshot: agentListSnapshot } = useAgentListResource(enabled);

  useEffect(() => {
    if (!enabled || !agentListResource) return;
    if (settingsData.executorsLoaded) return;
    if (executors.length === 0) {
      listExecutors({ cache: "no-store" })
        .then((response) => setExecutors(response.executors))
        .catch(() => setExecutors([]))
        .finally(() => setSettingsData({ executorsLoaded: true }));
    } else {
      setSettingsData({ executorsLoaded: true });
    }
  }, [enabled, executors.length, setExecutors, setSettingsData, settingsData.executorsLoaded]);

  useEffect(() => {
    if (!enabled || !agentListResource) return;
    if (settingsData.agentsLoaded) return;
    if (settingsAgents.length === 0) {
      agentListResource
        .ensure()
        .catch(() => {
          if (!agentListResource.isActive) return;
          setSettingsAgents([]);
          setAgentProfiles([]);
        })
        .finally(() => {
          if (agentListResource.isActive) setSettingsData({ agentsLoaded: true });
        });
    } else {
      setSettingsData({ agentsLoaded: true });
    }
  }, [
    enabled,
    setAgentProfiles,
    setSettingsAgents,
    setSettingsData,
    agentListResource,
    agentListSnapshot.loaded,
    settingsAgents.length,
    settingsData.agentsLoaded,
  ]);

  // Capabilities probe — host-utility probes ACP agents in the background and
  // the backend reconciler renames profiles from "Claude" → "Claude Sonnet 4.6"
  // once results land. The DB write happens *after* listAgents has already
  // returned the seeded profile.name, so without re-fetching here, the create
  // dialog renders stale labels (agent name with no model badge) for as long as
  // the probe takes to finish. This effect kicks the probe off and gates
  // listAgents-staleness on its completion.
  useEffect(() => {
    if (!enabled || !agentListResource) return;
    if (availableAgents.loaded || availableAgents.loading) return;
    setAvailableAgentsLoading(true);
    listAvailableAgents({ cache: "no-store" })
      .then((response) => setAvailableAgents(response.agents, response.tools ?? []))
      .catch(() => setAvailableAgents([]));
  }, [
    enabled,
    availableAgents.loaded,
    availableAgents.loading,
    setAvailableAgents,
    setAvailableAgentsLoading,
  ]);

  // Re-fetch once after the shared host-utility probe settles. The resource
  // owns this gate so multiple settings consumers do not queue duplicate reads.
  useEffect(() => {
    if (!enabled || !agentListResource) return;
    if (!availableAgents.loaded) return;
    if (!settingsData.agentsLoaded) return; // Wait for the initial agents fetch first.
    agentListResource.refreshAfterAvailableAgents().catch(() => {
      // Best-effort reconcile; keep prior (possibly stale) profiles rather
      // than wiping the dialog state on a transient error.
    });
  }, [enabled, availableAgents.loaded, settingsData.agentsLoaded, agentListResource]);
}
