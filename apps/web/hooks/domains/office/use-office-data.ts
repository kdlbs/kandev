import { useEffect } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { qk } from "@/lib/query/keys";
import {
  officeActivityQueryOptions,
  officeAgentsQueryOptions,
  officeDashboardQueryOptions,
  officeInboxQueryOptions,
  officeMetaQueryOptions,
  officeProjectsQueryOptions,
  officeRoutinesQueryOptions,
  officeSkillsQueryOptions,
} from "@/lib/query/query-options/office";
import type {
  ActivityEntry,
  AgentProfile,
  DashboardData,
  InboxItem,
  OfficeMeta,
  Project,
  Routine,
  Skill,
} from "@/lib/state/slices/office/types";

export function useOfficeMetaData(initialMeta?: OfficeMeta | null) {
  const queryClient = useQueryClient();
  const query = useQuery({
    ...officeMetaQueryOptions(),
    initialData: initialMeta ?? undefined,
  });

  useEffect(() => {
    if (!initialMeta) return;
    queryClient.setQueryData(qk.office.meta(), initialMeta);
  }, [initialMeta, queryClient]);

  return query;
}

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

export function useOfficeAgentsData(workspaceId: string | null, initialAgents?: AgentProfile[]) {
  const queryClient = useQueryClient();
  const query = useQuery(officeAgentsQueryOptions(workspaceId ?? ""));

  useEffect(() => {
    if (!workspaceId || initialAgents === undefined) return;
    queryClient.setQueryData(qk.office.agents(workspaceId), { agents: initialAgents });
  }, [initialAgents, queryClient, workspaceId]);

  return query;
}

export function useOfficeProjectsData(workspaceId: string | null, initialProjects?: Project[]) {
  const queryClient = useQueryClient();
  const query = useQuery(officeProjectsQueryOptions(workspaceId ?? ""));

  useEffect(() => {
    if (!workspaceId || initialProjects === undefined) return;
    queryClient.setQueryData(qk.office.projects(workspaceId), { projects: initialProjects });
  }, [initialProjects, queryClient, workspaceId]);

  return query;
}

export function useOfficeRoutinesData(workspaceId: string | null, initialRoutines?: Routine[]) {
  const queryClient = useQueryClient();
  const query = useQuery(officeRoutinesQueryOptions(workspaceId ?? ""));

  useEffect(() => {
    if (!workspaceId || initialRoutines === undefined) return;
    queryClient.setQueryData(qk.office.routines(workspaceId), { routines: initialRoutines });
  }, [initialRoutines, queryClient, workspaceId]);

  return query;
}

export function useOfficeSkillsData(workspaceId: string | null, initialSkills?: Skill[]) {
  const queryClient = useQueryClient();
  const query = useQuery(officeSkillsQueryOptions(workspaceId ?? ""));

  useEffect(() => {
    if (!workspaceId || initialSkills === undefined) return;
    queryClient.setQueryData(qk.office.skills(workspaceId), { skills: initialSkills });
  }, [initialSkills, queryClient, workspaceId]);

  return query;
}

export function useOfficeInboxData(
  workspaceId: string | null,
  initialItems?: InboxItem[],
  initialCount?: number,
) {
  const queryClient = useQueryClient();
  const inboxQuery = useQuery(officeInboxQueryOptions(workspaceId ?? ""));
  const agentsQuery = useOfficeAgentsData(workspaceId);

  useEffect(() => {
    if (!workspaceId || initialItems === undefined) return;
    queryClient.setQueryData(qk.office.inbox(workspaceId), {
      items: initialItems,
      total_count: initialCount ?? initialItems.length,
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
  initialActivity?: ActivityEntry[],
) {
  const queryClient = useQueryClient();
  const query = useQuery(officeActivityQueryOptions(workspaceId ?? "", filterType));

  useEffect(() => {
    if (!workspaceId || filterType !== "all" || initialActivity === undefined) return;
    queryClient.setQueryData(qk.office.activity(workspaceId, filterType), {
      activity: initialActivity,
    });
  }, [filterType, initialActivity, queryClient, workspaceId]);

  return query;
}
