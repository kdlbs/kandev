import { useEffect, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { listAgents, listAvailableAgents, listExecutors } from "@/lib/api";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";

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
      listAgents({ cache: "no-store" })
        .then((response) => {
          setSettingsAgents(response.agents);
          setAgentProfiles(
            response.agents.flatMap((agent) =>
              agent.profiles.map((profile) => toAgentProfileOption(agent, profile)),
            ),
          );
        })
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
    listAgents({ cache: "no-store" })
      .then((response) => {
        setSettingsAgents(response.agents);
        setAgentProfiles(
          response.agents.flatMap((agent) =>
            agent.profiles.map((profile) => toAgentProfileOption(agent, profile)),
          ),
        );
      })
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
