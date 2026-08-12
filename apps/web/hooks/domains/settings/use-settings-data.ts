import { useEffect, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { listAgents, listAvailableAgents, listExecutors } from "@/lib/api";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";

const AGENT_LIST_RETRY_DELAYS_MS = [100, 250, 500, 1_000] as const;

type AgentListResponse = Awaited<ReturnType<typeof listAgents>>;

function hasAgentProfiles(response: AgentListResponse): boolean {
  return response.agents.some((agent) => (agent.profiles?.length ?? 0) > 0);
}

function waitForAgentListRetry(delayMs: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, delayMs));
}

/**
 * Agent discovery and profile persistence complete before the backend health
 * endpoint becomes ready, but a freshly started client can still observe an
 * empty list while that state is settling. Keep the settings surface loading
 * briefly instead of treating that transient response as the authoritative
 * "no agents" state.
 */
export async function listAgentsUntilSettled(): Promise<AgentListResponse> {
  let lastResponse: AgentListResponse | undefined;
  let lastError: unknown;

  for (let attempt = 0; attempt <= AGENT_LIST_RETRY_DELAYS_MS.length; attempt++) {
    try {
      const response = await listAgents({ cache: "no-store" });
      lastResponse = response;
      if (hasAgentProfiles(response) || attempt === AGENT_LIST_RETRY_DELAYS_MS.length) {
        return response;
      }
    } catch (error) {
      lastError = error;
      if (attempt === AGENT_LIST_RETRY_DELAYS_MS.length) throw error;
    }

    await waitForAgentListRetry(AGENT_LIST_RETRY_DELAYS_MS[attempt]);
  }

  if (lastResponse) return lastResponse;
  throw lastError ?? new Error("Unable to load agent profiles");
}

function applyAgentList(
  response: AgentListResponse,
  setSettingsAgents: (agents: AgentListResponse["agents"]) => void,
  setAgentProfiles: (profiles: ReturnType<typeof toAgentProfileOption>[]) => void,
): void {
  setSettingsAgents(response.agents);
  setAgentProfiles(
    response.agents.flatMap((agent) =>
      agent.profiles.map((profile) => toAgentProfileOption(agent, profile)),
    ),
  );
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

  useEffect(() => {
    if (!enabled) return;
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
    if (!enabled) return;
    if (settingsData.agentsLoaded) return;
    if (settingsAgents.length === 0) {
      listAgentsUntilSettled()
        .then((response) => applyAgentList(response, setSettingsAgents, setAgentProfiles))
        .catch(() => {
          setSettingsAgents([]);
          setAgentProfiles([]);
        })
        .finally(() => setSettingsData({ agentsLoaded: true }));
    } else {
      setSettingsData({ agentsLoaded: true });
    }
  }, [
    enabled,
    setAgentProfiles,
    setSettingsAgents,
    setSettingsData,
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
    if (!enabled) return;
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

  // Re-fetch agent profiles once the host-utility probe completes. Use a ref
  // gate so we re-fetch exactly once per `availableAgents.loaded` transition,
  // not on every render after.
  const reconciledRef = useRef(false);
  useEffect(() => {
    if (!enabled) return;
    if (!availableAgents.loaded) return;
    if (!settingsData.agentsLoaded) return; // Wait for the initial agents fetch first.
    if (reconciledRef.current) return;
    reconciledRef.current = true;
    listAgentsUntilSettled()
      .then((response) => applyAgentList(response, setSettingsAgents, setAgentProfiles))
      .catch(() => {
        // Best-effort reconcile; keep prior (possibly stale) profiles rather
        // than wiping the dialog state on a transient error.
      });
  }, [
    enabled,
    availableAgents.loaded,
    settingsData.agentsLoaded,
    setAgentProfiles,
    setSettingsAgents,
  ]);
}
