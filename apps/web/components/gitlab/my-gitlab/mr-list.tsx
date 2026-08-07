"use client";

import { cn, formatRelativeTime } from "@/lib/utils";
import type { MR, TaskMR } from "@/lib/types/gitlab";
import type { GitLabLaunchPayload, GitLabTaskPreset } from "./quick-task-launcher";
import { gitLabMRKey } from "@/lib/gitlab-identity";
import { MRRowTaskIndicator } from "./mr-row-task-indicator";
import { StartTaskMenu } from "./start-task-menu";
import { ChangeRequestList, ChangeRequestRow } from "@/components/integrations/change-request-list";
import {
  IntegrationIcon,
  type IntegrationIconName,
} from "@/components/integrations/integration-icon";

type MRListProps = {
  items: MR[];
  loading: boolean;
  error: string | null;
  presets?: GitLabTaskPreset[];
  onStartTask?: (payload: GitLabLaunchPayload) => void;
  mrKeyToTasks?: Map<string, TaskMR[]>;
};

function mrStateIcon(mr: MR): { name: IntegrationIconName; className: string } {
  const state = mr.state === "opened" ? "open" : mr.state;
  if (state === "merged")
    return { name: "merged", className: "text-purple-600 dark:text-purple-400" };
  if (state === "closed")
    return { name: "pull-request-closed", className: "text-red-600 dark:text-red-400" };
  if (mr.draft) return { name: "pull-request", className: "text-muted-foreground" };
  return { name: "pull-request", className: "text-emerald-600 dark:text-emerald-400" };
}

function MRRow({
  mr,
  tasks,
  presets,
  onStartTask,
}: {
  mr: MR;
  tasks: TaskMR[] | undefined;
  presets: GitLabTaskPreset[];
  onStartTask?: MRListProps["onStartTask"];
}) {
  const { name: stateIconName, className: stateIconClass } = mrStateIcon(mr);
  return (
    <ChangeRequestRow
      stateIcon={<IntegrationIcon name={stateIconName} className={cn("h-4 w-4", stateIconClass)} />}
      title={mr.title}
      href={mr.web_url}
      metadata={
        <>
          <span className="whitespace-nowrap">
            {mr.project_path}!{mr.iid}
          </span>
          <span>·</span>
          <span className="whitespace-nowrap">
            {mr.head_branch} → {mr.base_branch}
          </span>
          <span>·</span>
          <span className="whitespace-nowrap">
            by {mr.author_username} · opened {formatRelativeTime(mr.created_at)}
          </span>
        </>
      }
      taskIndicator={<MRRowTaskIndicator tasks={tasks} />}
      action={
        onStartTask ? (
          <StartTaskMenu
            presets={presets}
            onSelect={(preset) => onStartTask({ kind: "mr", mr, preset })}
          />
        ) : null
      }
      testId="mr-row"
      dataAttributes={{ "data-mr-iid": mr.iid }}
    />
  );
}

function MRListBody({ items, presets = [], onStartTask, mrKeyToTasks }: MRListProps) {
  return (
    <>
      {items.map((mr) => (
        <MRRow
          key={`${mr.project_path}-${mr.iid}`}
          mr={mr}
          tasks={mrKeyToTasks?.get(gitLabMRKey(mr.web_url, mr.project_path, mr.iid))}
          presets={presets}
          onStartTask={onStartTask}
        />
      ))}
    </>
  );
}

export function MRList(props: MRListProps) {
  return (
    <ChangeRequestList
      loading={props.loading}
      error={props.error}
      emptyMessage="No merge requests match this filter."
      isEmpty={props.items.length === 0}
    >
      <MRListBody {...props} />
    </ChangeRequestList>
  );
}
