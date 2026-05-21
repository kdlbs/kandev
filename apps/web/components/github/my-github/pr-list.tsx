"use client";

import Link from "next/link";
import {
  IconGitPullRequest,
  IconGitPullRequestClosed,
  IconGitMerge,
  IconPlus,
  IconChevronDown,
} from "@tabler/icons-react";
import type { Icon } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Spinner } from "@kandev/ui/spinner";
import { cn, formatRelativeTime } from "@/lib/utils";
import type { GitHubPR, GitHubPRStatus, TaskPR } from "@/lib/types/github";
import type { LaunchPayload, TaskPreset } from "./quick-task-launcher";
import { PRStatusBadges } from "./pr-status-badges";
import { prStatusKey, usePRStatuses } from "./use-pr-statuses";
import { PRRowTaskIndicator } from "./pr-row-task-indicator";

type PRListProps = {
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

function prStateIcon(pr: GitHubPR): { Icon: Icon; className: string } {
  if (pr.state === "merged")
    return { Icon: IconGitMerge, className: "text-purple-600 dark:text-purple-400" };
  if (pr.state === "closed")
    return { Icon: IconGitPullRequestClosed, className: "text-red-600 dark:text-red-400" };
  if (pr.draft) return { Icon: IconGitPullRequest, className: "text-muted-foreground" };
  return { Icon: IconGitPullRequest, className: "text-emerald-600 dark:text-emerald-400" };
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
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          size="sm"
          variant="outline"
          className="h-7 gap-1 cursor-pointer"
          data-testid="pr-start-task-trigger"
        >
          <IconPlus className="h-3.5 w-3.5" />
          Task
          <IconChevronDown className="h-3 w-3" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-48">
        {presets.map((p) => {
          const ItemIcon = p.icon;
          return (
            <DropdownMenuItem
              key={p.id}
              className="cursor-pointer gap-2 py-1.5"
              onSelect={() => launch(p)}
              data-testid="pr-start-task-preset"
              data-preset-id={p.id}
            >
              <ItemIcon className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
              <div className="flex flex-col min-w-0">
                <span className="text-xs font-medium leading-tight">{p.label}</span>
                <span className="text-[11px] text-muted-foreground leading-tight">{p.hint}</span>
              </div>
            </DropdownMenuItem>
          );
        })}
      </DropdownMenuContent>
    </DropdownMenu>
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
  const { Icon: StateIcon, className: stateIconClass } = prStateIcon(pr);
  return (
    <div
      className="flex items-start gap-3 px-4 py-3 hover:bg-muted/40 transition-colors"
      data-testid="pr-row"
      data-pr-number={pr.number}
    >
      <StateIcon className={cn("h-4 w-4 mt-1 shrink-0", stateIconClass)} />
      <div className="min-w-0 flex-1">
        <Link
          href={pr.html_url}
          target="_blank"
          rel="noopener noreferrer"
          className="text-sm font-semibold hover:underline block truncate cursor-pointer"
        >
          {pr.title}
        </Link>
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1 mt-0.5 text-xs text-muted-foreground">
          <span className="whitespace-nowrap">
            {pr.repo_owner}/{pr.repo_name}#{pr.number}
          </span>
          <span>·</span>
          <span className="whitespace-nowrap">
            by {pr.author_login} · opened {formatRelativeTime(pr.created_at)}
          </span>
          <PRStatusBadges pr={pr} status={status} />
          <PRRowTaskIndicator tasks={tasks} />
        </div>
      </div>
      <div className="shrink-0">
        <StartTaskMenu
          pr={pickPRForLaunch(pr, status)}
          presets={presets}
          onStartTask={onStartTask}
        />
      </div>
    </div>
  );
}

function PRListBody({ loading, error, items, presets, onStartTask, prKeyToTasks }: PRListProps) {
  const statuses = usePRStatuses(items);
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
        No pull requests match this filter.
      </div>
    );
  }
  return (
    <div className="divide-y">
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
    </div>
  );
}

export function PRList(props: PRListProps) {
  return (
    <div className="rounded-md border overflow-hidden">
      <PRListBody {...props} />
    </div>
  );
}
