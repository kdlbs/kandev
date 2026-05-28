"use client";

import { useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { settingsQueryOptions } from "@/lib/query/query-options/settings";
import { useAppStore } from "@/components/state-provider";

/**
 * Loads executors, agents (+ agent profiles), and available agents.
 * In the TQ world, each data type is its own query — dedup and stale-time
 * handle the "load once" semantics that the old Zustand loading flags provided.
 *
 * Many consumers (task-create dialog, mode/model selectors, sessions dropdown,
 * preview tabs, agent status, jira/linear watch dialogs, etc.) still read from
 * the Zustand slices directly, so this hook also mirrors the TQ data back into
 * the slices. The mirror can be removed once every consumer reads through TQ.
 *
 * `enabled` prop is preserved for callers that conditionally activate loading.
 */
export function useSettingsData(enabled = true) {
  const executorsQuery = useQuery({ ...settingsQueryOptions.executors(), enabled });
  const agentsQuery = useQuery({ ...settingsQueryOptions.agents(), enabled });
  const agentProfilesQuery = useQuery({ ...settingsQueryOptions.agentProfiles(), enabled });
  const availableAgentsQuery = useQuery({ ...settingsQueryOptions.availableAgents(), enabled });

  const setExecutors = useAppStore((s) => s.setExecutors);
  const setSettingsAgents = useAppStore((s) => s.setSettingsAgents);
  const setAgentProfiles = useAppStore((s) => s.setAgentProfiles);
  const setAvailableAgents = useAppStore((s) => s.setAvailableAgents);
  const setSettingsData = useAppStore((s) => s.setSettingsData);

  useEffect(() => {
    if (executorsQuery.data) setExecutors(executorsQuery.data);
  }, [executorsQuery.data, setExecutors]);
  useEffect(() => {
    if (agentsQuery.data) setSettingsAgents(agentsQuery.data);
  }, [agentsQuery.data, setSettingsAgents]);
  useEffect(() => {
    if (agentProfilesQuery.data) setAgentProfiles(agentProfilesQuery.data);
  }, [agentProfilesQuery.data, setAgentProfiles]);
  useEffect(() => {
    const data = availableAgentsQuery.data;
    if (data) setAvailableAgents(data.agents, data.tools);
  }, [availableAgentsQuery.data, setAvailableAgents]);

  // settingsData.{agentsLoaded, executorsLoaded} drive the loading gates in
  // the task-create dialog — without them the selectors stay disabled and
  // tests can't click them. Flip the flags once each query has finished.
  useEffect(() => {
    if (agentsQuery.isSuccess) setSettingsData({ agentsLoaded: true });
  }, [agentsQuery.isSuccess, setSettingsData]);
  useEffect(() => {
    if (executorsQuery.isSuccess) setSettingsData({ executorsLoaded: true });
  }, [executorsQuery.isSuccess, setSettingsData]);
}
