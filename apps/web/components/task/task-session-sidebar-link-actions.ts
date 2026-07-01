"use client";

import { useCallback, useState } from "react";
import type { ExternalLinkProvider } from "./task-external-link-dialog";

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

export function useSidebarLinkActions(taskById: ReadonlyMap<string, SidebarLinkTarget>) {
  const [linkingPullRequestTask, setLinkingPullRequestTask] = useState<SidebarLinkTarget | null>(
    null,
  );
  const [linkingIssueTask, setLinkingIssueTask] = useState<SidebarLinkTarget | null>(null);
  const [linkingExternalIssueTask, setLinkingExternalIssueTask] =
    useState<SidebarExternalLinkTarget | null>(null);

  const getLinkTarget = useCallback(
    (taskId: string, fallbackTitle?: string): SidebarLinkTarget => {
      const task = taskById.get(taskId);
      return {
        id: taskId,
        title: task?.title ?? fallbackTitle ?? "this task",
        repositoryId: task?.repositoryId,
        issueUrl: task?.issueUrl,
        issueNumber: task?.issueNumber,
        repositories: task?.repositories,
      };
    },
    [taskById],
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
    linkingExternalIssueTask,
    setLinkingExternalIssueTask,
    handleLinkJiraTicketTask,
    handleLinkLinearIssueTask,
    handleLinkSentryIssueTask,
  };
}
