import { useEffect, useMemo, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  agentsQueryOptions,
  availableAgentsQueryOptions,
  executorsQueryOptions,
} from "@/lib/query/query-options/settings";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";

export function useSettingsData(enabled = true) {
  const executorsQuery = useQuery({ ...executorsQueryOptions(), enabled });
  const agentsQuery = useQuery({ ...agentsQueryOptions(), enabled });
  const availableAgentsQuery = useQuery({ ...availableAgentsQueryOptions(), enabled });
  const refetchAgents = agentsQuery.refetch;
  const settingsAgents = agentsQuery.data?.agents ?? [];
  const agentProfiles = useMemo(
    () =>
      settingsAgents.flatMap((agent) =>
        agent.profiles.map((profile) => toAgentProfileOption(agent, profile)),
      ),
    [settingsAgents],
  );

  // Host-utility probes ACP agents in the background and the backend reconciler
  // can rename profiles once results land. Re-fetch agent profiles once the
  // capability probe completes so settings pickers don't render stale labels.
  const reconciledRef = useRef(false);
  useEffect(() => {
    if (!enabled) return;
    if (!availableAgentsQuery.isSuccess) return;
    if (!agentsQuery.isSuccess) return; // Wait for the initial agents fetch first.
    if (reconciledRef.current) return;
    reconciledRef.current = true;
    void refetchAgents();
  }, [agentsQuery.isSuccess, availableAgentsQuery.isSuccess, enabled, refetchAgents]);

  return {
    executors: executorsQuery.data?.executors ?? [],
    settingsAgents,
    agentProfiles,
    availableAgents: availableAgentsQuery.data?.agents ?? [],
    availableTools: availableAgentsQuery.data?.tools ?? [],
    settingsData: {
      agentsLoaded: agentsQuery.isSuccess || agentsQuery.isError,
      capabilitiesLoaded: availableAgentsQuery.isSuccess || availableAgentsQuery.isError,
      executorsLoaded: executorsQuery.isSuccess || executorsQuery.isError,
    },
  };
}
