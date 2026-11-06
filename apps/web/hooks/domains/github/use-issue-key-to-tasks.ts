"use client";

import { githubResourceKey } from "@/lib/github/host";
import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import type { TaskIssueLink } from "@/lib/types/github";
import { useWorkspaceTaskIssues } from "./use-task-issues";

export function issueKey(
  owner: string,
  repo: string,
  issueNumber: number,
  host = "github.com",
): string {
  return githubResourceKey(owner, repo, issueNumber, host);
}

export function useIssueKeyToTasks(workspaceId: string | null): Map<string, TaskIssueLink[]> {
  useWorkspaceTaskIssues(workspaceId);
  const taskIssues = useAppStore((state) => state.taskIssues);

  return useMemo(() => {
    const map = new Map<string, TaskIssueLink[]>();
    if (taskIssues.workspaceId !== workspaceId) return map;
    for (const link of Object.values(taskIssues.byTaskId)) {
      const key = issueKey(link.owner, link.repo, link.issue_number, link.github_host);
      const existing = map.get(key) ?? [];
      existing.push(link);
      map.set(key, existing);
    }
    return map;
  }, [taskIssues, workspaceId]);
}
