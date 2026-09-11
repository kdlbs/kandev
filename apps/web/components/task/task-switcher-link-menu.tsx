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
import { ContextMenuItem, ContextMenuSub, ContextMenuSubTrigger } from "@kandev/ui/context-menu";
import { TaskContextMenuSubContent as ContextMenuSubContent } from "./task-context-menu-sub-content";
import { resolvePluginIcon } from "@/lib/plugins/icons";
import type { PluginLinkMenuAction } from "./task-session-sidebar-link-actions";
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

export function taskLinkMenuOptions(actions: Partial<ReturnType<typeof selectTaskLinkActions>>) {
  return [
    {
      id: "pull-request",
      labelKey: "task:githubPullRequest",
      Icon: IconGitPullRequest,
      onSelect: actions.onLinkPullRequest,
    },
    {
      id: "issue",
      labelKey: "task:githubIssue",
      Icon: IconCircleDot,
      onSelect: actions.onLinkIssue,
    },
    {
      id: "merge-request",
      labelKey: "task:gitlabMergeRequest",
      Icon: IconBrandGitlab,
      onSelect: actions.onLinkMergeRequest,
    },
    {
      id: "jira",
      labelKey: "task:jiraTicket",
      Icon: IconTicket,
      onSelect: actions.onLinkJiraTicket,
    },
    {
      id: "linear",
      labelKey: "task:linearIssue",
      Icon: IconCircleDot,
      onSelect: actions.onLinkLinearIssue,
    },
    {
      id: "sentry",
      labelKey: "task:sentryIssue",
      Icon: IconBrandSentry,
      onSelect: actions.onLinkSentryIssue,
    },
  ].filter((option) => option.onSelect !== undefined);
}

export function TaskPluginLinkMenu({
  task,
  disabled,
  closeMenu,
  linkActions,
  pluginLinkActions: suppliedPluginLinkActions,
}: {
  task?: TaskSwitcherItem;
  disabled?: boolean;
  closeMenu: () => void;
  linkActions: Partial<ReturnType<typeof selectTaskLinkActions>>;
  pluginLinkActions?: PluginLinkMenuAction[];
}) {
  if (suppliedPluginLinkActions) {
    return (
      <TaskLinkMenuWithActions
        disabled={disabled}
        closeMenu={closeMenu}
        linkActions={linkActions}
        pluginLinkActions={suppliedPluginLinkActions}
      />
    );
  }
  if (!task) return null;
  return (
    <TaskLinkMenuWithTask
      task={task}
      disabled={disabled}
      closeMenu={closeMenu}
      linkActions={linkActions}
    />
  );
}

function TaskLinkMenuWithTask({
  task,
  disabled,
  closeMenu,
  linkActions,
}: {
  task: TaskSwitcherItem;
  disabled?: boolean;
  closeMenu: () => void;
  linkActions: Partial<ReturnType<typeof selectTaskLinkActions>>;
}) {
  const pluginLinkActions = useTaskPluginLinkActions(task.id, task.repositoryLinks ?? []);
  return (
    <TaskLinkMenuWithActions
      disabled={disabled}
      closeMenu={closeMenu}
      linkActions={linkActions}
      pluginLinkActions={pluginLinkActions}
    />
  );
}

function TaskLinkMenuWithActions({
  disabled,
  closeMenu,
  linkActions,
  pluginLinkActions,
}: {
  disabled?: boolean;
  closeMenu: () => void;
  linkActions: Partial<ReturnType<typeof selectTaskLinkActions>>;
  pluginLinkActions: PluginLinkMenuAction[];
}) {
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
  pluginLinkActions?: PluginLinkMenuAction[];
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
        {taskLinkMenuOptions({
          onLinkPullRequest,
          onLinkIssue,
          onLinkMergeRequest,
          onLinkJiraTicket,
          onLinkLinearIssue,
          onLinkSentryIssue,
        }).map(({ id, labelKey, Icon, onSelect }) => (
          <ContextMenuItem
            key={id}
            className={id === "merge-request" ? "min-h-12! sm:min-h-7!" : undefined}
            disabled={disabled}
            onSelect={onSelect}
          >
            <Icon className="mr-2 h-4 w-4" />
            {t(labelKey)}
          </ContextMenuItem>
        ))}
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
