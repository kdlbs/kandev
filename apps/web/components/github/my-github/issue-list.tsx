"use client";

import { IconCircle, IconCircleCheck } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { useTranslation } from "react-i18next";
import { ChangeRequestList, ChangeRequestRow } from "@/components/integrations/change-request-list";
import { IntegrationStartTaskMenu } from "@/components/integrations/integration-start-task-menu";
import { TaskRowIndicator } from "@/components/integrations/task-row-indicator";
import { cn, formatRelativeTime } from "@/lib/utils";
import type { GitHubIssue, TaskIssueLink } from "@/lib/types/github";
import type { LaunchPayload, TaskPreset } from "./quick-task-launcher";

type IssueListProps = {
  items: GitHubIssue[];
  loading: boolean;
  error: string | null;
  presets: TaskPreset[];
  onStartTask: (payload: LaunchPayload) => void;
  issueKeyToTasks?: Map<string, TaskIssueLink[]>;
};

function IssueLabels({ labels }: { labels: string[] }) {
  if (!labels?.length) return null;
  return (
    <>
      {labels.slice(0, 4).map((label) => (
        <Badge
          key={label}
          variant="secondary"
          className="text-[10px] px-1.5 py-0 h-auto min-h-4 whitespace-normal wrap-anywhere"
        >
          {label}
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
  tasks,
}: {
  issue: GitHubIssue;
  presets: TaskPreset[];
  onStartTask: IssueListProps["onStartTask"];
  tasks: TaskIssueLink[] | undefined;
}) {
  const { t } = useTranslation();
  const StateIcon = issue.state === "open" ? IconCircle : IconCircleCheck;
  const stateClass =
    issue.state === "open"
      ? "text-emerald-600 dark:text-emerald-400"
      : "text-purple-600 dark:text-purple-400";
  return (
    <ChangeRequestRow
      stateIcon={<StateIcon className={cn("h-4 w-4", stateClass)} />}
      title={issue.title}
      href={issue.html_url}
      metadata={
        <>
          <span>
            {issue.repo_owner}/{issue.repo_name}#{issue.number}
          </span>
          <span>·</span>
          <span>
            {t("github:byAuthorOpenedAgo", {
              author: issue.author_login,
              time: formatRelativeTime(issue.created_at),
            })}
          </span>
          <IssueLabels labels={issue.labels} />
        </>
      }
      taskIndicator={
        <TaskRowIndicator
          tasks={tasks?.map((task) => ({
            id: task.task_id,
            taskId: task.task_id,
            fallbackTitle: task.task_title,
          }))}
          testIdPrefix="issue-row-task-indicator"
        />
      }
      action={
        <IntegrationStartTaskMenu
          presets={presets}
          onSelect={(preset) => onStartTask({ kind: "issue", issue, preset })}
          triggerTestId="issue-start-task-trigger"
          itemTestId="issue-start-task-preset"
        />
      }
      testId="issue-row"
      dataAttributes={{ "data-issue-number": issue.number }}
    />
  );
}

export function IssueList({
  items,
  loading,
  error,
  presets,
  onStartTask,
  issueKeyToTasks,
}: IssueListProps) {
  const { t } = useTranslation();
  return (
    <ChangeRequestList
      loading={loading}
      error={error}
      isEmpty={items.length === 0}
      emptyMessage={t("github:noIssuesMatchThisFilter")}
    >
      {items.map((issue) => {
        const key = `${issue.repo_owner}/${issue.repo_name}#${issue.number}`;
        return (
          <IssueRow
            key={key}
            issue={issue}
            presets={presets}
            onStartTask={onStartTask}
            tasks={issueKeyToTasks?.get(key)}
          />
        );
      })}
    </ChangeRequestList>
  );
}
