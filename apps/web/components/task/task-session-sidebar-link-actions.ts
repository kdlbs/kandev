"use client";

import { useCallback, useState } from "react";
import type { KanbanState } from "@/lib/state/slices";
import { findTaskInSnapshots } from "@/lib/kanban/find-task";

type StoreApi = {
  getState: () => {
    kanbanMulti: { snapshots: Record<string, { tasks: KanbanState["tasks"] }> };
    kanban: { tasks: KanbanState["tasks"] };
  };
};

export type SidebarLinkTarget = {
  id: string;
  title: string;
  repositoryId?: string;
  issueUrl?: string;
  issueNumber?: number;
  repositories?: Array<{ id?: string; repository_id: string; position?: number }>;
};

export function useSidebarLinkActions(store: StoreApi) {
  const [linkingPullRequestTask, setLinkingPullRequestTask] = useState<SidebarLinkTarget | null>(
    null,
  );
  const [linkingIssueTask, setLinkingIssueTask] = useState<SidebarLinkTarget | null>(null);

  const getLinkTarget = useCallback(
    (taskId: string): SidebarLinkTarget => {
      const state = store.getState();
      const task = findTaskInSnapshots(taskId, state.kanbanMulti.snapshots, state.kanban.tasks);
      return {
        id: taskId,
        title: task?.title ?? "this task",
        repositoryId: task?.repositoryId,
        issueUrl: task?.issueUrl,
        issueNumber: task?.issueNumber,
        repositories: task?.repositories,
      };
    },
    [store],
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
