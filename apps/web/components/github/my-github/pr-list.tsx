"use client";

import { cn, formatRelativeTime } from "@/lib/utils";
import type { GitHubPR, GitHubPRStatus, TaskPR } from "@/lib/types/github";
import type { LaunchPayload, TaskPreset } from "./quick-task-launcher";
import { PRStatusBadges } from "./pr-status-badges";
import { prStatusKey, usePRStatuses } from "./use-pr-statuses";
import { PRRowTaskIndicator } from "./pr-row-task-indicator";
import { ChangeRequestList, ChangeRequestRow } from "@/components/integrations/change-request-list";
import { IntegrationStartTaskMenu } from "@/components/integrations/integration-start-task-menu";
import {
  IntegrationIcon,
  type IntegrationIconName,
} from "@/components/integrations/integration-icon";

type PRListProps = {
  workspaceId: string | null;
  items: GitHubPR[];
  loading: boolean;
  error: string | null;
  presets: TaskPreset[];
  onStartTask: (payload: LaunchPayload) => void;
  prKeyToTasks?: Map<string, TaskPR[]>;
};

// Prefer the enriched PR returned by the batched status endpoint — the search
// API used to populate `items` does not include head/base branches, so the
// launcher needs the enriched copy to pre-fill the task dialog correctly.
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
  const launch = (preset: TaskPreset) => onStartTask({ kind: "pr", pr, preset });
  return (
    <IntegrationStartTaskMenu
      presets={presets}
      onSelect={launch}
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
            by {pr.author_login} · opened {formatRelativeTime(pr.created_at)}
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
  return (
    <ChangeRequestList
      loading={props.loading}
      error={props.error}
      emptyMessage="No pull requests match this filter."
      isEmpty={props.items.length === 0}
    >
      <PRListBody {...props} />
    </ChangeRequestList>
  );
}
