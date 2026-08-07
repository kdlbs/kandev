import type { ReactNode } from "react";
import {
  IconBrandGitlab,
  IconBrandSentry,
  IconCircleDot,
  IconGitPullRequest,
  IconLink,
  IconTicket,
} from "@tabler/icons-react";
import { resolvePluginIcon } from "@/lib/plugins/icons";
import type { KanbanCardMenuEntry, KanbanPluginLinkAction } from "./kanban-card-menu-items";

type LinkSubmenuArgs = {
  disabled?: boolean;
  onLinkPullRequest?: () => void;
  onLinkIssue?: () => void;
  onLinkMergeRequest?: () => void;
  onLinkJiraTicket?: () => void;
  onLinkLinearIssue?: () => void;
  onLinkSentryIssue?: () => void;
  pluginLinkActions?: KanbanPluginLinkAction[];
};

type LinkItem = Pick<LinkSubmenuArgs, "disabled"> & {
  key: string;
  testId: string;
  icon: ReactNode;
  label: string;
  onSelect?: () => void;
};

type LinkMenuItem = Extract<KanbanCardMenuEntry, { kind: "item" }>;

function buildLinkItem({
  disabled,
  key,
  testId,
  icon,
  label,
  onSelect,
}: LinkItem): LinkMenuItem | null {
  if (!onSelect) return null;
  return { kind: "item" as const, key, testId, icon, label, disabled, onSelect };
}

function builtInLinkItems(args: LinkSubmenuArgs): KanbanCardMenuEntry[] {
  const { disabled } = args;
  return [
    buildLinkItem({
      disabled,
      key: "link-github-pull-request",
      testId: "task-context-link-github-pull-request",
      icon: <IconGitPullRequest className="mr-2 h-4 w-4" />,
      label: "GitHub Pull Request",
      onSelect: args.onLinkPullRequest,
    }),
    buildLinkItem({
      disabled,
      key: "link-github-issue",
      testId: "task-context-link-github-issue",
      icon: <IconCircleDot className="mr-2 h-4 w-4" />,
      label: "GitHub Issue",
      onSelect: args.onLinkIssue,
    }),
    buildLinkItem({
      disabled,
      key: "link-gitlab-merge-request",
      testId: "task-context-link-gitlab-merge-request",
      icon: <IconBrandGitlab className="mr-2 h-4 w-4" />,
      label: "GitLab Merge Request",
      onSelect: args.onLinkMergeRequest,
    }),
    buildLinkItem({
      disabled,
      key: "link-jira-ticket",
      testId: "task-context-link-jira-ticket",
      icon: <IconTicket className="mr-2 h-4 w-4" />,
      label: "Jira Ticket",
      onSelect: args.onLinkJiraTicket,
    }),
    buildLinkItem({
      disabled,
      key: "link-linear-issue",
      testId: "task-context-link-linear-issue",
      icon: <IconCircleDot className="mr-2 h-4 w-4" />,
      label: "Linear Issue",
      onSelect: args.onLinkLinearIssue,
    }),
    buildLinkItem({
      disabled,
      key: "link-sentry-issue",
      testId: "task-context-link-sentry-issue",
      icon: <IconBrandSentry className="mr-2 h-4 w-4" />,
      label: "Sentry Issue",
      onSelect: args.onLinkSentryIssue,
    }),
  ].filter((item): item is LinkMenuItem => item !== null);
}

function pluginLinkItems(actions: KanbanPluginLinkAction[], disabled: boolean | undefined) {
  return actions.map((action) => {
    const Icon = resolvePluginIcon(action.icon);
    return {
      kind: "item" as const,
      key: `link-plugin-${action.id}`,
      testId: `task-context-link-plugin-${action.id}`,
      icon: <Icon className="mr-2 h-4 w-4" />,
      label: action.label,
      disabled: disabled || action.disabled,
      // Radix closes the current menu during this event. Defer plugin work one
      // microtask so a modal/panel never mounts beneath an open menu surface.
      onSelect: () => queueMicrotask(action.onSelect),
    };
  });
}

export function buildLinkSubmenu(args: LinkSubmenuArgs): KanbanCardMenuEntry | null {
  const children = [
    ...builtInLinkItems(args),
    ...pluginLinkItems(args.pluginLinkActions ?? [], args.disabled),
  ];
  if (children.length === 0) return null;
  return {
    kind: "submenu",
    key: "link",
    testId: "task-context-link",
    icon: <IconLink className="mr-2 h-4 w-4" />,
    label: "Link",
    disabled: args.disabled,
    className: "w-56",
    children,
  };
}
