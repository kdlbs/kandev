"use client";

import { IconCircle, IconCircleCheck } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Spinner } from "@kandev/ui/spinner";
import { cn, formatRelativeTime } from "@/lib/utils";
import type { Issue } from "@/lib/types/gitlab";
import type { GitLabLaunchPayload, GitLabTaskPreset } from "./quick-task-launcher";
import { StartTaskMenu } from "./start-task-menu";
import { SubscriptionToggle } from "../subscription-toggle";
import { RowTitleLink } from "./row-title-link";
import { useTranslation } from "react-i18next";

type IssueListProps = {
  items: Issue[];
  loading: boolean;
  error: string | null;
  presets?: GitLabTaskPreset[];
  onStartTask?: (payload: GitLabLaunchPayload) => void;
  workspaceId?: string;
  host?: string;
};

function IssueMilestoneChip({ milestone }: { milestone: string }) {
  if (!milestone) return null;
  return (
    <Badge
      variant="outline"
      className="text-[10px] px-1.5 py-0 h-4"
      data-testid="gitlab-issue-milestone"
    >
      {milestone}
    </Badge>
  );
}

function IssueLabels({ labels }: { labels: string[] }) {
  if (!labels?.length) return null;
  return (
    <>
      {labels.slice(0, 4).map((l) => (
        <Badge key={l} variant="secondary" className="text-[10px] px-1.5 py-0 h-4">
          {l}
        </Badge>
      ))}
      {labels.length > 4 && (
        <span className="text-[10px] text-muted-foreground">+{labels.length - 4}</span>
      )}
    </>
  );
}

function IssueRow({
  issue,
  presets,
  onStartTask,
  workspaceId,
  host,
}: {
  issue: Issue;
  presets: GitLabTaskPreset[];
  onStartTask?: IssueListProps["onStartTask"];
  workspaceId?: string;
  host?: string;
}) {
  const { t } = useTranslation();
  const isOpen = issue.state !== "closed";
  const StateIcon = isOpen ? IconCircle : IconCircleCheck;
  const stateClass = isOpen
    ? "text-emerald-600 dark:text-emerald-400"
    : "text-purple-600 dark:text-purple-400";
  return (
    <div
      className="flex items-start gap-3 px-4 py-3 hover:bg-muted/40 transition-colors"
      data-testid="issue-row"
      data-issue-iid={issue.iid}
    >
      <StateIcon className={cn("h-4 w-4 mt-1 shrink-0", stateClass)} />
      <div className="min-w-0 flex-1">
        <RowTitleLink href={issue.web_url} title={issue.title} />
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1 mt-0.5 text-xs text-muted-foreground">
          <span className="whitespace-nowrap">
            {issue.project_path}#{issue.iid}
          </span>
          <span>·</span>
          <span className="whitespace-nowrap">
            {t("gitlab:byAuthorOpenedAgo", {
              author: issue.author_username,
              time: formatRelativeTime(issue.created_at),
            })}
          </span>
          <IssueMilestoneChip milestone={issue.milestone ?? ""} />
          <IssueLabels labels={issue.labels} />
        </div>
      </div>
      <div className="shrink-0">
        <div className="flex items-center gap-2">
          {workspaceId && host ? (
            <SubscriptionToggle
              kind="issue"
              workspaceId={workspaceId}
              host={host}
              project={issue.project_path}
              iid={issue.iid}
            />
          ) : null}
          {onStartTask ? (
            <StartTaskMenu
              presets={presets}
              onSelect={(preset) => onStartTask({ kind: "issue", issue, preset })}
            />
          ) : null}
        </div>
      </div>
    </div>
  );
}

function IssueListBody({
  loading,
  error,
  items,
  presets = [],
  onStartTask,
  workspaceId,
  host,
}: IssueListProps) {
  const { t } = useTranslation();
  if (loading) {
    return (
      <div className="flex justify-center py-10">
        <Spinner />
      </div>
    );
  }
  if (error) {
    return <div className="text-center py-10 text-destructive text-sm">{error}</div>;
  }
  if (items.length === 0) {
    return (
      <div className="text-center py-10 text-muted-foreground text-sm">
        {t("gitlab:noIssuesMatchThisFilter")}
      </div>
    );
  }
  return (
    <div className="divide-y">
      {items.map((issue) => (
        <IssueRow
          key={`${issue.project_path}-${issue.iid}`}
          issue={issue}
          presets={presets}
          onStartTask={onStartTask}
          workspaceId={workspaceId}
          host={host}
        />
      ))}
    </div>
  );
}

export function IssueList(props: IssueListProps) {
  return (
    <div className="rounded-md border overflow-hidden">
      <IssueListBody {...props} />
    </div>
  );
}
