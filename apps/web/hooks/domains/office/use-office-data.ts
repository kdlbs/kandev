import { useEffect } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
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
  const query = useQuery(officeDashboardQueryOptions(workspaceId ?? ""));

  useEffect(() => {
    if (!workspaceId || !initialDashboard) return;
    queryClient.setQueryData(qk.office.dashboard(workspaceId), initialDashboard);
  }, [initialDashboard, queryClient, workspaceId]);

  return query;
}

export function useOfficeAgentsData(
  workspaceId: string | null,
  initialAgents: AgentProfile[] = [],
) {
  const queryClient = useQueryClient();
  const query = useQuery(officeAgentsQueryOptions(workspaceId ?? ""));

  useEffect(() => {
    if (!workspaceId || initialAgents.length === 0) return;
    queryClient.setQueryData(qk.office.agents(workspaceId), { agents: initialAgents });
  }, [initialAgents, queryClient, workspaceId]);

  return query;
}

export function useOfficeProjectsData(workspaceId: string | null, initialProjects: Project[] = []) {
  const queryClient = useQueryClient();
  const query = useQuery(officeProjectsQueryOptions(workspaceId ?? ""));

  useEffect(() => {
    if (!workspaceId || initialProjects.length === 0) return;
    queryClient.setQueryData(qk.office.projects(workspaceId), { projects: initialProjects });
  }, [initialProjects, queryClient, workspaceId]);

  return query;
}

export function useOfficeInboxData(
  workspaceId: string | null,
  initialItems: InboxItem[] = [],
  initialCount = 0,
) {
  const queryClient = useQueryClient();
  const inboxQuery = useQuery(officeInboxQueryOptions(workspaceId ?? ""));
  const agentsQuery = useOfficeAgentsData(workspaceId);

  useEffect(() => {
    if (!workspaceId || (initialItems.length === 0 && initialCount === 0)) return;
    queryClient.setQueryData(qk.office.inbox(workspaceId), {
      items: initialItems,
      total_count: initialCount || initialItems.length,
    });
  }, [initialCount, initialItems, queryClient, workspaceId]);

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
  const query = useQuery(officeActivityQueryOptions(workspaceId ?? "", filterType));

  useEffect(() => {
    if (!workspaceId || filterType !== "all" || initialActivity.length === 0) return;
    queryClient.setQueryData(qk.office.activity(workspaceId, filterType), {
      activity: initialActivity,
    });
  }, [filterType, initialActivity, queryClient, workspaceId]);

  return query;
}
