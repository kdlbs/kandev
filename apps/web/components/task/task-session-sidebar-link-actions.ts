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
  const [linkingMergeRequestTask, setLinkingMergeRequestTask] = useState<SidebarLinkTarget | null>(
    null,
  );
  const [linkingExternalIssueTask, setLinkingExternalIssueTask] =
    useState<SidebarExternalLinkTarget | null>(null);

  const getLinkTarget = useCallback(
    (taskId: string, fallbackTitle?: string): SidebarLinkTarget => {
      const state = store.getState();
      const task = findTaskInSnapshots(taskId, state.kanbanMulti.snapshots, state.kanban.tasks);
      return {
        id: taskId,
        title: task?.title ?? fallbackTitle ?? "this task",
        repositoryId: task?.repositoryId,
        issueUrl: task?.issueUrl,
        issueNumber: task?.issueNumber,
        repositories: task?.repositories,
      };
    },
    [store],
  );

  const handleLinkPullRequestTask = useCallback(
    (taskId: string, fallbackTitle?: string) => {
      setLinkingPullRequestTask(getLinkTarget(taskId, fallbackTitle));
    },
    [getLinkTarget],
  );

  const handleLinkIssueTask = useCallback(
    (taskId: string, fallbackTitle?: string) => {
      setLinkingIssueTask(getLinkTarget(taskId, fallbackTitle));
    },
    [getLinkTarget],
  );

  const handleLinkMergeRequestTask = useCallback(
    (taskId: string, fallbackTitle?: string) => {
      setLinkingMergeRequestTask(getLinkTarget(taskId, fallbackTitle));
    },
    [getLinkTarget],
  );

  const handleLinkExternalIssueTask = useCallback(
    (provider: ExternalLinkProvider, taskId: string, fallbackTitle?: string) => {
      setLinkingExternalIssueTask({ provider, task: getLinkTarget(taskId, fallbackTitle) });
    },
    [getLinkTarget],
  );

  const handleLinkJiraTicketTask = useCallback(
    (taskId: string, fallbackTitle?: string) =>
      handleLinkExternalIssueTask("jira", taskId, fallbackTitle),
    [handleLinkExternalIssueTask],
  );

  const handleLinkLinearIssueTask = useCallback(
    (taskId: string, fallbackTitle?: string) =>
      handleLinkExternalIssueTask("linear", taskId, fallbackTitle),
    [handleLinkExternalIssueTask],
  );

  const handleLinkSentryIssueTask = useCallback(
    (taskId: string, fallbackTitle?: string) =>
      handleLinkExternalIssueTask("sentry", taskId, fallbackTitle),
    [handleLinkExternalIssueTask],
  );

  return {
    linkingPullRequestTask,
    setLinkingPullRequestTask,
    handleLinkPullRequestTask,
    linkingIssueTask,
    setLinkingIssueTask,
    handleLinkIssueTask,
    linkingMergeRequestTask,
    setLinkingMergeRequestTask,
    handleLinkMergeRequestTask,
    linkingExternalIssueTask,
    setLinkingExternalIssueTask,
    handleLinkJiraTicketTask,
    handleLinkLinearIssueTask,
    handleLinkSentryIssueTask,
  };
}
