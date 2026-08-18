"use client";

import { useTranslation } from "react-i18next";
import { ChangeRequestList, ChangeRequestRow } from "@/components/integrations/change-request-list";
import {
  IntegrationIcon,
  type IntegrationIconName,
} from "@/components/integrations/integration-icon";
import { IntegrationStartTaskMenu } from "@/components/integrations/integration-start-task-menu";
import { cn, formatRelativeTime } from "@/lib/utils";
import type { GitHubPR, GitHubPRStatus, TaskPR } from "@/lib/types/github";
import type { LaunchPayload, TaskPreset } from "./quick-task-launcher";
import { PRStatusBadges } from "./pr-status-badges";
import { PRRowTaskIndicator } from "./pr-row-task-indicator";
import { prStatusKey, usePRStatuses } from "./use-pr-statuses";

type PRListProps = {
  workspaceId: string | null;
  items: GitHubPR[];
  loading: boolean;
  error: string | null;
  presets: TaskPreset[];
  onStartTask: (payload: LaunchPayload) => void;
  prKeyToTasks?: Map<string, TaskPR[]>;
};

export function pickPRForLaunch(pr: GitHubPR, status: GitHubPRStatus | null | undefined): GitHubPR {
  return status?.pr ?? pr;
}

function prStateIcon(pr: GitHubPR): { name: IntegrationIconName; className: string } {
  if (pr.state === "merged")
    return { name: "merged", className: "text-purple-600 dark:text-purple-400" };
  if (pr.state === "closed")
    return { name: "pull-request-closed", className: "text-red-600 dark:text-red-400" };
  if (pr.draft) return { name: "pull-request", className: "text-muted-foreground" };
  return { name: "pull-request", className: "text-emerald-600 dark:text-emerald-400" };
}

function StartTaskMenu({
  pr,
  presets,
  onStartTask,
}: {
  pr: GitHubPR;
  presets: TaskPreset[];
  onStartTask: PRListProps["onStartTask"];
}) {
  const { t } = useTranslation();
  return (
    <IntegrationStartTaskMenu
      presets={presets}
      onSelect={(preset) => onStartTask({ kind: "pr", pr, preset })}
      triggerLabel={t("github:task")}
      triggerAriaLabel={t("github:task")}
      triggerTestId="pr-start-task-trigger"
      itemTestId="pr-start-task-preset"
    />
  );
}

function PRRow({
  pr,
  status,
  presets,
  onStartTask,
  tasks,
}: {
  pr: GitHubPR;
  status: GitHubPRStatus | null | undefined;
  presets: TaskPreset[];
  onStartTask: PRListProps["onStartTask"];
  tasks: TaskPR[] | undefined;
}) {
  const { t } = useTranslation();
  const { name: stateIconName, className: stateIconClass } = prStateIcon(pr);
  return (
    <ChangeRequestRow
      stateIcon={<IntegrationIcon name={stateIconName} className={cn("h-4 w-4", stateIconClass)} />}
      title={pr.title}
      href={pr.html_url}
      metadata={
        <>
          <span className="whitespace-nowrap">
            {pr.repo_owner}/{pr.repo_name}#{pr.number}
          </span>
          <span>·</span>
          <span className="whitespace-nowrap">
            {t("github:byAuthorOpenedAgo", {
              author: pr.author_login,
              time: formatRelativeTime(pr.created_at),
            })}
          </span>
          <PRStatusBadges pr={pr} status={status} />
        </>
      }
      taskIndicator={<PRRowTaskIndicator tasks={tasks} />}
      action={
        <StartTaskMenu
          pr={pickPRForLaunch(pr, status)}
          presets={presets}
          onStartTask={onStartTask}
        />
      }
      testId="pr-row"
      dataAttributes={{ "data-pr-number": pr.number }}
    />
  );
}

function PRListBody({ workspaceId, items, presets, onStartTask, prKeyToTasks }: PRListProps) {
  const statuses = usePRStatuses(workspaceId, items);
  return (
    <>
      {items.map((pr) => {
        const key = prStatusKey(pr.repo_owner, pr.repo_name, pr.number);
        return (
          <PRRow
            key={key}
            pr={pr}
            status={statuses.get(key)}
            presets={presets}
            onStartTask={onStartTask}
            tasks={prKeyToTasks?.get(key)}
          />
        );
      })}
    </>
  );
}

export function PRList(props: PRListProps) {
  const { t } = useTranslation();
  return (
    <ChangeRequestList
      loading={props.loading}
      error={props.error}
      emptyMessage={t("github:noPullRequestsMatchThisFilter")}
      isEmpty={props.items.length === 0}
    >
      <PRListBody {...props} />
    </ChangeRequestList>
  );
}
