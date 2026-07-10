"use client";

import { useJiraAvailable } from "@/hooks/domains/jira/use-jira-availability";
import { useLinearAvailable } from "@/hooks/domains/linear/use-linear-availability";
import { useSentryAvailable } from "@/hooks/domains/sentry/use-sentry-availability";
import type { useSidebarActions } from "./task-session-sidebar";

type SidebarActions = Pick<
  ReturnType<typeof useSidebarActions>,
  | "handleLinkPullRequestTask"
  | "handleLinkIssueTask"
  | "handleLinkJiraTicketTask"
  | "handleLinkLinearIssueTask"
  | "handleLinkSentryIssueTask"
>;

export function useSidebarTaskLinking(workspaceId: string | null, actions: SidebarActions) {
  const jiraAvailable = useJiraAvailable(workspaceId);
  const linearAvailable = useLinearAvailable(workspaceId);
  const sentryAvailable = useSentryAvailable(workspaceId);

  return {
    onLinkPullRequest: actions.handleLinkPullRequestTask,
    onLinkIssue: actions.handleLinkIssueTask,
    onLinkJiraTicket: jiraAvailable ? actions.handleLinkJiraTicketTask : undefined,
    onLinkLinearIssue: linearAvailable ? actions.handleLinkLinearIssueTask : undefined,
    onLinkSentryIssue: sentryAvailable ? actions.handleLinkSentryIssueTask : undefined,
  };
}
