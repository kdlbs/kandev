import {
  IconBrandGitlab,
  IconBrandSentry,
  IconCircleDot,
  IconGitPullRequest,
  IconLink,
  IconTicket,
} from "@tabler/icons-react";
import { t } from "@/lib/i18n";
import type { KanbanCardMenuEntry } from "./kanban-card-menu-items";

function buildGitLabMergeRequestLinkEntry({
  disabled,
  onLinkMergeRequest,
}: {
  disabled?: boolean;
  onLinkMergeRequest?: () => void;
}): KanbanCardMenuEntry | null {
  if (!onLinkMergeRequest) return null;
  return {
    kind: "item",
    key: "link-gitlab-merge-request",
    testId: "task-context-link-gitlab-merge-request",
    icon: <IconBrandGitlab className="mr-2 h-4 w-4" />,
    label: t("kanban:gitlabMergeRequest"),
    disabled,
    onSelect: onLinkMergeRequest,
  };
}

export function buildLinkSubmenu({
  disabled,
  onLinkPullRequest,
  onLinkIssue,
  onLinkMergeRequest,
  onLinkJiraTicket,
  onLinkLinearIssue,
  onLinkSentryIssue,
}: {
  disabled?: boolean;
  onLinkPullRequest?: () => void;
  onLinkIssue?: () => void;
  onLinkMergeRequest?: () => void;
  onLinkJiraTicket?: () => void;
  onLinkLinearIssue?: () => void;
  onLinkSentryIssue?: () => void;
}): KanbanCardMenuEntry | null {
  if (
    !onLinkPullRequest &&
    !onLinkIssue &&
    !onLinkMergeRequest &&
    !onLinkJiraTicket &&
    !onLinkLinearIssue &&
    !onLinkSentryIssue
  ) {
    return null;
  }
  const children: KanbanCardMenuEntry[] = [];
  if (onLinkPullRequest) {
    children.push({
      kind: "item",
      key: "link-github-pull-request",
      testId: "task-context-link-github-pull-request",
      icon: <IconGitPullRequest className="mr-2 h-4 w-4" />,
      label: t("kanban:githubPullRequest"),
      disabled,
      onSelect: onLinkPullRequest,
    });
  }
  if (onLinkIssue) {
    children.push({
      kind: "item",
      key: "link-github-issue",
      testId: "task-context-link-github-issue",
      icon: <IconCircleDot className="mr-2 h-4 w-4" />,
      label: t("kanban:githubIssue"),
      disabled,
      onSelect: onLinkIssue,
    });
  }
  const gitLabEntry = buildGitLabMergeRequestLinkEntry({ disabled, onLinkMergeRequest });
  if (gitLabEntry) children.push(gitLabEntry);
  if (onLinkJiraTicket) {
    children.push({
      kind: "item",
      key: "link-jira-ticket",
      testId: "task-context-link-jira-ticket",
      icon: <IconTicket className="mr-2 h-4 w-4" />,
      label: t("kanban:jiraTicket"),
      disabled,
      onSelect: onLinkJiraTicket,
    });
  }
  if (onLinkLinearIssue) {
    children.push({
      kind: "item",
      key: "link-linear-issue",
      testId: "task-context-link-linear-issue",
      icon: <IconCircleDot className="mr-2 h-4 w-4" />,
      label: t("kanban:linearIssue"),
      disabled,
      onSelect: onLinkLinearIssue,
    });
  }
  if (onLinkSentryIssue) {
    children.push({
      kind: "item",
      key: "link-sentry-issue",
      testId: "task-context-link-sentry-issue",
      icon: <IconBrandSentry className="mr-2 h-4 w-4" />,
      label: t("kanban:sentryIssue"),
      disabled,
      onSelect: onLinkSentryIssue,
    });
  }
  return {
    kind: "submenu",
    key: "link",
    testId: "task-context-link",
    icon: <IconLink className="mr-2 h-4 w-4" />,
    label: t("kanban:link"),
    disabled,
    className: "w-56",
    children,
  };
}
