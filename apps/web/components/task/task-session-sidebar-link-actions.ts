"use client";

import { useCallback, useState } from "react";
import type { KanbanState } from "@/lib/state/slices";
import { findTaskInSnapshots } from "@/lib/kanban/find-task";
import type { ExternalLinkProvider } from "./task-external-link-dialog";

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

export type SidebarExternalLinkTarget = {
  provider: ExternalLinkProvider;
  task: SidebarLinkTarget;
};

export function useSidebarLinkActions(store: StoreApi) {
  const [linkingPullRequestTask, setLinkingPullRequestTask] = useState<SidebarLinkTarget | null>(
    null,
  );
  const [linkingIssueTask, setLinkingIssueTask] = useState<SidebarLinkTarget | null>(null);
  const [linkingExternalIssueTask, setLinkingExternalIssueTask] =
    useState<SidebarExternalLinkTarget | null>(null);

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

  const handleLinkExternalIssueTask = useCallback(
    (provider: ExternalLinkProvider, taskId: string) => {
      setLinkingExternalIssueTask({ provider, task: getLinkTarget(taskId) });
    },
    [getLinkTarget],
  );

  const handleLinkJiraTicketTask = useCallback(
    (taskId: string) => handleLinkExternalIssueTask("jira", taskId),
    [handleLinkExternalIssueTask],
  );

  const handleLinkLinearIssueTask = useCallback(
    (taskId: string) => handleLinkExternalIssueTask("linear", taskId),
    [handleLinkExternalIssueTask],
  );

  const handleLinkSentryIssueTask = useCallback(
    (taskId: string) => handleLinkExternalIssueTask("sentry", taskId),
    [handleLinkExternalIssueTask],
  );

  return {
    linkingPullRequestTask,
    setLinkingPullRequestTask,
    handleLinkPullRequestTask,
    linkingIssueTask,
    setLinkingIssueTask,
    handleLinkIssueTask,
    linkingExternalIssueTask,
    setLinkingExternalIssueTask,
    handleLinkJiraTicketTask,
    handleLinkLinearIssueTask,
    handleLinkSentryIssueTask,
  };
}
