"use client";

import { useMemo } from "react";
import type { TaskIssueLink } from "@/lib/types/github";
import { useWorkspaceTaskIssues } from "./use-task-issues";

export function issueKey(owner: string, repo: string, issueNumber: number): string {
  return `${owner}/${repo}#${issueNumber}`;
}

export function useIssueKeyToTasks(workspaceId: string | null): Map<string, TaskIssueLink[]> {
  const { data: taskIssues = {} } = useWorkspaceTaskIssues(workspaceId);

  return useMemo(() => {
    const map = new Map<string, TaskIssueLink[]>();
    for (const link of Object.values(taskIssues)) {
      const key = issueKey(link.owner, link.repo, link.issue_number);
      const existing = map.get(key) ?? [];
      existing.push(link);
      map.set(key, existing);
    }
    return map;
  }, [taskIssues]);
}
