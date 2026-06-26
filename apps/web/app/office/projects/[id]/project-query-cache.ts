"use client";

import { useCallback } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { qk } from "@/lib/query/keys";
import type { Project } from "@/lib/state/slices/office/types";

export function useSyncOfficeProjectCache() {
  const queryClient = useQueryClient();

  return useCallback(
    (project: Project) => {
      queryClient.setQueryData(qk.office.project(project.id), project);
      queryClient.setQueryData<{ projects: Project[] }>(
        qk.office.projects(project.workspaceId),
        (current) => {
          const projects = current?.projects ?? [];
          if (projects.some((item) => item.id === project.id)) {
            return {
              projects: projects.map((item) => (item.id === project.id ? project : item)),
            };
          }
          return { projects: [...projects, project] };
        },
      );
    },
    [queryClient],
  );
}
