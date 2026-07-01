"use client";

import { useQuery } from "@tanstack/react-query";
import { workspaceTaskIssuesQueryOptions } from "@/lib/query/query-options/github";

export function useWorkspaceTaskIssues(workspaceId: string | null) {
  return useQuery({
    ...workspaceTaskIssuesQueryOptions(workspaceId ?? ""),
    enabled: Boolean(workspaceId),
  });
}
