import { useEffect } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { qk } from "@/lib/query/keys";
import {
  officeActivityQueryOptions,
  officeAgentsQueryOptions,
  officeDashboardQueryOptions,
  officeInboxQueryOptions,
  officeProjectsQueryOptions,
} from "@/lib/query/query-options/office";
import type {
  ActivityEntry,
  AgentProfile,
  DashboardData,
  InboxItem,
  Project,
} from "@/lib/state/slices/office/types";

export function useOfficeDashboardData(
  workspaceId: string | null,
  initialDashboard?: DashboardData | null,
) {
  const queryClient = useQueryClient();
  const setDashboard = useAppStore((state) => state.setDashboard);
  const query = useQuery(officeDashboardQueryOptions(workspaceId ?? ""));

  useEffect(() => {
    if (!workspaceId || !initialDashboard) return;
    queryClient.setQueryData(qk.office.dashboard(workspaceId), initialDashboard);
    setDashboard(initialDashboard);
  }, [initialDashboard, queryClient, setDashboard, workspaceId]);

  useEffect(() => {
    if (!query.data) return;
    setDashboard(query.data);
  }, [query.data, setDashboard]);

  return query;
}

export function useOfficeAgentsData(
  workspaceId: string | null,
  initialAgents: AgentProfile[] = [],
) {
  const queryClient = useQueryClient();
  const setAgents = useAppStore((state) => state.setOfficeAgentProfiles);
  const query = useQuery(officeAgentsQueryOptions(workspaceId ?? ""));

  useEffect(() => {
    if (!workspaceId || initialAgents.length === 0) return;
    queryClient.setQueryData(qk.office.agents(workspaceId), { agents: initialAgents });
    setAgents(initialAgents);
  }, [initialAgents, queryClient, setAgents, workspaceId]);

  useEffect(() => {
    if (!query.data) return;
    setAgents(query.data.agents ?? []);
  }, [query.data, setAgents]);

  return query;
}

export function useOfficeProjectsData(workspaceId: string | null, initialProjects: Project[] = []) {
  const queryClient = useQueryClient();
  const setProjects = useAppStore((state) => state.setProjects);
  const query = useQuery(officeProjectsQueryOptions(workspaceId ?? ""));

  useEffect(() => {
    if (!workspaceId || initialProjects.length === 0) return;
    queryClient.setQueryData(qk.office.projects(workspaceId), { projects: initialProjects });
    setProjects(initialProjects);
  }, [initialProjects, queryClient, setProjects, workspaceId]);

  useEffect(() => {
    if (!query.data) return;
    setProjects(query.data.projects ?? []);
  }, [query.data, setProjects]);

  return query;
}

export function useOfficeInboxData(
  workspaceId: string | null,
  initialItems: InboxItem[] = [],
  initialCount = 0,
) {
  const queryClient = useQueryClient();
  const setItems = useAppStore((state) => state.setInboxItems);
  const setCount = useAppStore((state) => state.setInboxCount);
  const inboxQuery = useQuery(officeInboxQueryOptions(workspaceId ?? ""));
  const agentsQuery = useOfficeAgentsData(workspaceId);

  useEffect(() => {
    if (!workspaceId || (initialItems.length === 0 && initialCount === 0)) return;
    queryClient.setQueryData(qk.office.inbox(workspaceId), {
      items: initialItems,
      total_count: initialCount || initialItems.length,
    });
    setItems(initialItems);
    setCount(initialCount || initialItems.length);
  }, [initialCount, initialItems, queryClient, setCount, setItems, workspaceId]);

  useEffect(() => {
    if (!inboxQuery.data) return;
    const items = inboxQuery.data.items ?? [];
    setItems(items);
    setCount(inboxQuery.data.total_count ?? items.length);
  }, [inboxQuery.data, setCount, setItems]);

  return {
    ...inboxQuery,
    refetchAll: async () => {
      await Promise.all([inboxQuery.refetch(), agentsQuery.refetch()]);
    },
  };
}

export function useOfficeActivityData(
  workspaceId: string | null,
  filterType = "all",
  initialActivity: ActivityEntry[] = [],
) {
  const queryClient = useQueryClient();
  const setActivity = useAppStore((state) => state.setActivity);
  const query = useQuery(officeActivityQueryOptions(workspaceId ?? "", filterType));

  useEffect(() => {
    if (!workspaceId || filterType !== "all" || initialActivity.length === 0) return;
    queryClient.setQueryData(qk.office.activity(workspaceId, filterType), {
      activity: initialActivity,
    });
    setActivity(initialActivity);
  }, [filterType, initialActivity, queryClient, setActivity, workspaceId]);

  useEffect(() => {
    if (!query.data) return;
    setActivity(query.data.activity ?? []);
  }, [query.data, setActivity]);

  return query;
}
