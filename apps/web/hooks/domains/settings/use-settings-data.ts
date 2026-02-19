import { useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import { listAgents, listEnvironments, listExecutors } from "@/lib/api";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";

export function useSettingsData(enabled = true) {
  const executors = useAppStore((state) => state.executors.items);
  const environments = useAppStore((state) => state.environments.items);
  const settingsAgents = useAppStore((state) => state.settingsAgents.items);
  const settingsData = useAppStore((state) => state.settingsData);
  const setExecutors = useAppStore((state) => state.setExecutors);
  const setEnvironments = useAppStore((state) => state.setEnvironments);
  const setSettingsAgents = useAppStore((state) => state.setSettingsAgents);
  const setAgentProfiles = useAppStore((state) => state.setAgentProfiles);
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
    if (settingsData.environmentsLoaded) return;
    if (environments.length === 0) {
      listEnvironments({ cache: "no-store" })
        .then((response) => setEnvironments(response.environments))
        .catch(() => setEnvironments([]))
        .finally(() => setSettingsData({ environmentsLoaded: true }));
    } else {
      setSettingsData({ environmentsLoaded: true });
    }
  }, [
    enabled,
    environments.length,
    setEnvironments,
    setSettingsData,
    settingsData.environmentsLoaded,
  ]);

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
}
