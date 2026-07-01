"use client";

import { useAppStore } from "@/components/state-provider";
import { useOfficeAgentsData, useOfficeProjectsData } from "@/hooks/domains/office/use-office-data";

export function useActiveOfficeAgents() {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  return useOfficeAgentsData(workspaceId).data?.agents ?? [];
}

export function useActiveOfficeProjects() {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  return useOfficeProjectsData(workspaceId).data?.projects ?? [];
}
