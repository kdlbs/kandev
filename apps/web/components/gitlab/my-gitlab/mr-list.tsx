"use client";

import Link from "@/components/routing/app-link";
import {
  IconGitPullRequest,
  IconGitPullRequestClosed,
  IconGitMerge,
  IconExternalLink,
} from "@tabler/icons-react";
import type { Icon } from "@tabler/icons-react";
import { Spinner } from "@kandev/ui/spinner";
import { cn, formatRelativeTime } from "@/lib/utils";
import type { MR, TaskMR } from "@/lib/types/gitlab";
import type { GitLabLaunchPayload, GitLabTaskPreset } from "./quick-task-launcher";
import { gitLabMRKey } from "@/lib/gitlab-identity";
import { MRRowTaskIndicator } from "./mr-row-task-indicator";
import { StartTaskMenu } from "./start-task-menu";

type MRListProps = {
  items: MR[];
  loading: boolean;
  error: string | null;
  presets?: GitLabTaskPreset[];
  onStartTask?: (payload: GitLabLaunchPayload) => void;
  mrKeyToTasks?: Map<string, TaskMR[]>;
};

function mrStateIcon(mr: MR): { Icon: Icon; className: string } {
  const state = mr.state === "opened" ? "open" : mr.state;
  if (state === "merged")
    return { Icon: IconGitMerge, className: "text-purple-600 dark:text-purple-400" };
  if (state === "closed")
    return { Icon: IconGitPullRequestClosed, className: "text-red-600 dark:text-red-400" };
  if (mr.draft) return { Icon: IconGitPullRequest, className: "text-muted-foreground" };
  return { Icon: IconGitPullRequest, className: "text-emerald-600 dark:text-emerald-400" };
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
  const { Icon: StateIcon, className: stateIconClass } = mrStateIcon(mr);
  return (
    <div
      className="flex items-start gap-3 px-4 py-3 hover:bg-muted/40 transition-colors"
      data-testid="mr-row"
      data-mr-iid={mr.iid}
    >
      <StateIcon className={cn("h-4 w-4 mt-1 shrink-0", stateIconClass)} />
      <div className="min-w-0 flex-1">
        <Link
          href={mr.web_url}
          target="_blank"
          rel="noopener noreferrer"
          className="text-sm font-semibold hover:underline inline-flex items-center gap-1.5 truncate cursor-pointer"
        >
          <span className="truncate">{mr.title}</span>
          <IconExternalLink className="h-3 w-3 shrink-0 text-muted-foreground" />
        </Link>
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1 mt-0.5 text-xs text-muted-foreground">
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
          <MRRowTaskIndicator tasks={tasks} />
        </div>
      </div>
      <div className="shrink-0">
        {onStartTask ? (
          <StartTaskMenu
            presets={presets}
            onSelect={(preset) => onStartTask({ kind: "mr", mr, preset })}
          />
        ) : null}
      </div>
    </div>
  );
}

function MRListBody({
  loading,
  error,
  items,
  presets = [],
  onStartTask,
  mrKeyToTasks,
}: MRListProps) {
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
        No merge requests match this filter.
      </div>
    );
  }
  return (
    <div className="divide-y">
      {items.map((mr) => (
        <MRRow
          key={`${mr.project_path}-${mr.iid}`}
          mr={mr}
          tasks={mrKeyToTasks?.get(gitLabMRKey(mr.web_url, mr.project_path, mr.iid))}
          presets={presets}
          onStartTask={onStartTask}
        />
      ))}
    </div>
  );
}

export function MRList(props: MRListProps) {
  return (
    <div className="rounded-md border overflow-hidden">
      <MRListBody {...props} />
    </div>
  );
}
