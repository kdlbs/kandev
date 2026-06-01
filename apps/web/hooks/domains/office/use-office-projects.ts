"use client";

import { useQuery } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { officeQueryOptions } from "@/lib/query/query-options/office";
import type { Project } from "@/lib/state/slices/office/types";

const EMPTY_PROJECTS: Project[] = [];

/**
 * Projects for the active workspace, read from TanStack Query.
 *
 * Replaces the legacy `useAppStore(s => s.office.projects)` mirror read.
 */
export function useOfficeProjects(): Project[] {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const { data } = useQuery({
    ...officeQueryOptions.projects(workspaceId ?? ""),
    enabled: !!workspaceId,
  });
  return data ?? EMPTY_PROJECTS;
}
