"use client";

import { createElement } from "react";
import { useTranslation } from "react-i18next";
import {
  IconBrandGitlab,
  IconBrandSentry,
  IconCircleDot,
  IconGitPullRequest,
  IconLink,
  IconTicket,
} from "@tabler/icons-react";
import {
  ContextMenuItem,
  ContextMenuSub,
  ContextMenuSubContent,
  ContextMenuSubTrigger,
} from "@kandev/ui/context-menu";
import { resolvePluginIcon } from "@/lib/plugins/icons";
import { useTaskPluginLinkActions } from "./task-session-sidebar-link-actions";
import type { TaskSwitcherItem } from "./task-switcher-types";

export type TaskLinkHandlers = {
  onLinkPullRequest?: (taskId: string, taskTitle?: string) => void;
  onLinkIssue?: (taskId: string, taskTitle?: string) => void;
  onLinkMergeRequest?: (taskId: string, taskTitle?: string) => void;
  onLinkJiraTicket?: (taskId: string, taskTitle?: string) => void;
  onLinkLinearIssue?: (taskId: string, taskTitle?: string) => void;
  onLinkSentryIssue?: (taskId: string, taskTitle?: string) => void;
};

export function createTaskLinkSelectAction(
  task: Pick<TaskSwitcherItem, "id" | "title">,
  handler: ((taskId: string, taskTitle?: string) => void) | undefined,
  closeMenu: () => void,
) {
  if (!handler) return undefined;
  return () => {
    closeMenu();
    handler(task.id, task.title);
  };
}

export function selectTaskLinkActions(
  task: Pick<TaskSwitcherItem, "id" | "title">,
  closeMenu: () => void,
  handlers: TaskLinkHandlers,
) {
  return {
    onLinkPullRequest: createTaskLinkSelectAction(task, handlers.onLinkPullRequest, closeMenu),
    onLinkIssue: createTaskLinkSelectAction(task, handlers.onLinkIssue, closeMenu),
    onLinkMergeRequest: createTaskLinkSelectAction(task, handlers.onLinkMergeRequest, closeMenu),
    onLinkJiraTicket: createTaskLinkSelectAction(task, handlers.onLinkJiraTicket, closeMenu),
    onLinkLinearIssue: createTaskLinkSelectAction(task, handlers.onLinkLinearIssue, closeMenu),
    onLinkSentryIssue: createTaskLinkSelectAction(task, handlers.onLinkSentryIssue, closeMenu),
  };
}

export function TaskPluginLinkMenu({
  task,
  disabled,
  closeMenu,
  linkActions,
}: {
  task: TaskSwitcherItem;
  disabled?: boolean;
  closeMenu: () => void;
  linkActions: ReturnType<typeof selectTaskLinkActions>;
}) {
  const pluginLinkActions = useTaskPluginLinkActions(task.id, task.repositoryLinks ?? []);
  return (
    <TaskLinkMenu
      disabled={disabled}
      pluginLinkActions={pluginLinkActions.map((action) => ({
        ...action,
        onSelect: () => {
          closeMenu();
          queueMicrotask(action.onSelect);
        },
      }))}
      {...linkActions}
    />
  );
}

function TaskLinkMenu({
  disabled,
  onLinkPullRequest,
  onLinkIssue,
  onLinkMergeRequest,
  onLinkJiraTicket,
  onLinkLinearIssue,
  onLinkSentryIssue,
  pluginLinkActions = [],
}: {
  disabled?: boolean;
  onLinkPullRequest?: () => void;
  onLinkIssue?: () => void;
  onLinkMergeRequest?: () => void;
  onLinkJiraTicket?: () => void;
  onLinkLinearIssue?: () => void;
  onLinkSentryIssue?: () => void;
  pluginLinkActions?: { id: string; label: string; icon?: string; onSelect: () => void }[];
}) {
  const { t } = useTranslation();
  if (
    !onLinkPullRequest &&
    !onLinkIssue &&
    !onLinkMergeRequest &&
    !onLinkJiraTicket &&
    !onLinkLinearIssue &&
    !onLinkSentryIssue &&
    pluginLinkActions.length === 0
  ) {
    return null;
  }
  return (
    <ContextMenuSub>
      <ContextMenuSubTrigger disabled={disabled}>
        <IconLink className="mr-2 h-4 w-4" />
        {t("task:link")}
      </ContextMenuSubTrigger>
      <ContextMenuSubContent className="w-56">
        {onLinkPullRequest ? (
          <ContextMenuItem disabled={disabled} onSelect={onLinkPullRequest}>
            <IconGitPullRequest className="mr-2 h-4 w-4" />
            {t("task:githubPullRequest")}
          </ContextMenuItem>
        ) : null}
        {onLinkIssue ? (
          <ContextMenuItem disabled={disabled} onSelect={onLinkIssue}>
            <IconCircleDot className="mr-2 h-4 w-4" />
            {t("task:githubIssue")}
          </ContextMenuItem>
        ) : null}
        {onLinkMergeRequest ? (
          <ContextMenuItem
            className="min-h-12! sm:min-h-7!"
            disabled={disabled}
            onSelect={onLinkMergeRequest}
          >
            <IconBrandGitlab className="mr-2 h-4 w-4" />
            {t("task:gitlabMergeRequest")}
          </ContextMenuItem>
        ) : null}
        {onLinkJiraTicket ? (
          <ContextMenuItem disabled={disabled} onSelect={onLinkJiraTicket}>
            <IconTicket className="mr-2 h-4 w-4" />
            {t("task:jiraTicket")}
          </ContextMenuItem>
        ) : null}
        {onLinkLinearIssue ? (
          <ContextMenuItem disabled={disabled} onSelect={onLinkLinearIssue}>
            <IconCircleDot className="mr-2 h-4 w-4" />
            {t("task:linearIssue")}
          </ContextMenuItem>
        ) : null}
        {onLinkSentryIssue ? (
          <ContextMenuItem disabled={disabled} onSelect={onLinkSentryIssue}>
            <IconBrandSentry className="mr-2 h-4 w-4" />
            {t("task:sentryIssue")}
          </ContextMenuItem>
        ) : null}
        {pluginLinkActions.map((action) => (
          <ContextMenuItem
            key={action.id}
            data-testid={`task-context-link-plugin-${action.id}`}
            disabled={disabled}
            onSelect={action.onSelect}
          >
            {createElement(resolvePluginIcon(action.icon), { className: "mr-2 h-4 w-4" })}
            {action.label}
          </ContextMenuItem>
        ))}
      </ContextMenuSubContent>
    </ContextMenuSub>
  );
}
