"use client";

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

/** The sidebar task row's "Link" submenu — extracted out of task-switcher-context-menu.tsx to stay under the file-size limit as that file grows plugin surfaces. */
export function TaskLinkMenu({
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
}) {
  const { t } = useTranslation();
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
  return (
    <ContextMenuSub>
      <ContextMenuSubTrigger disabled={disabled}>
        <IconLink className="mr-2 h-4 w-4" />
        {t("task:link")}
      </ContextMenuSubTrigger>
      <ContextMenuSubContent className="w-56">
        {onLinkPullRequest && (
          <ContextMenuItem disabled={disabled} onSelect={onLinkPullRequest}>
            <IconGitPullRequest className="mr-2 h-4 w-4" />
            {t("task:githubPullRequest")}
          </ContextMenuItem>
        )}
        {onLinkIssue && (
          <ContextMenuItem disabled={disabled} onSelect={onLinkIssue}>
            <IconCircleDot className="mr-2 h-4 w-4" />
            {t("task:githubIssue")}
          </ContextMenuItem>
        )}
        {onLinkMergeRequest && (
          <ContextMenuItem
            className="min-h-12! sm:min-h-7!"
            disabled={disabled}
            onSelect={onLinkMergeRequest}
          >
            <IconBrandGitlab className="mr-2 h-4 w-4" />
            {t("task:gitlabMergeRequest")}
          </ContextMenuItem>
        )}
        {onLinkJiraTicket && (
          <ContextMenuItem disabled={disabled} onSelect={onLinkJiraTicket}>
            <IconTicket className="mr-2 h-4 w-4" />
            {t("task:jiraTicket")}
          </ContextMenuItem>
        )}
        {onLinkLinearIssue && (
          <ContextMenuItem disabled={disabled} onSelect={onLinkLinearIssue}>
            <IconCircleDot className="mr-2 h-4 w-4" />
            {t("task:linearIssue")}
          </ContextMenuItem>
        )}
        {onLinkSentryIssue && (
          <ContextMenuItem disabled={disabled} onSelect={onLinkSentryIssue}>
            <IconBrandSentry className="mr-2 h-4 w-4" />
            {t("task:sentryIssue")}
          </ContextMenuItem>
        )}
      </ContextMenuSubContent>
    </ContextMenuSub>
  );
}
