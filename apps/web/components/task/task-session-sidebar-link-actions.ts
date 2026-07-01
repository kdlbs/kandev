"use client";

import { useCallback, useState } from "react";

export type SidebarLinkTarget = {
  id: string;
  title: string;
  repositoryId?: string;
  issueUrl?: string;
  issueNumber?: number;
  repositories?: Array<{ id?: string; repository_id: string; position?: number }>;
};

export function useSidebarLinkActions(taskById: ReadonlyMap<string, SidebarLinkTarget>) {
  const [linkingPullRequestTask, setLinkingPullRequestTask] = useState<SidebarLinkTarget | null>(
    null,
  );
  const [linkingIssueTask, setLinkingIssueTask] = useState<SidebarLinkTarget | null>(null);

  const getLinkTarget = useCallback(
    (taskId: string): SidebarLinkTarget => {
      const task = taskById.get(taskId);
      return {
        id: taskId,
        title: task?.title ?? "this task",
        repositoryId: task?.repositoryId,
        issueUrl: task?.issueUrl,
        issueNumber: task?.issueNumber,
        repositories: task?.repositories,
      };
    },
    [taskById],
  );

  const handleLinkPullRequestTask = useCallback(
    (taskId: string) => {
      setLinkingPullRequestTask(getLinkTarget(taskId));
    },
    [getLinkTarget],
  );

  const handleLinkIssueTask = useCallback(
    (taskId: string) => {
      setLinkingIssueTask(getLinkTarget(taskId));
    },
    [getLinkTarget],
  );

  return {
    linkingPullRequestTask,
    setLinkingPullRequestTask,
    handleLinkPullRequestTask,
    linkingIssueTask,
    setLinkingIssueTask,
    handleLinkIssueTask,
  };
}
