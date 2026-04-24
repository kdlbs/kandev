"use client";

import Link from "next/link";
import { IconCircle, IconCircleCheck, IconPlus, IconChevronDown } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Spinner } from "@kandev/ui/spinner";
import { cn } from "@/lib/utils";
import { formatRelativeTime } from "@/lib/utils";
import type { GitHubIssue } from "@/lib/types/github";
import type { LaunchPayload, TaskPreset } from "./quick-task-launcher";

type IssueListProps = {
  items: GitHubIssue[];
  loading: boolean;
  error: string | null;
  presets: TaskPreset[];
  onStartTask: (payload: LaunchPayload) => void;
};

function StartTaskMenu({
  issue,
  presets,
  onStartTask,
}: {
  issue: GitHubIssue;
  presets: TaskPreset[];
  onStartTask: IssueListProps["onStartTask"];
}) {
  const launch = (preset: TaskPreset) => onStartTask({ kind: "issue", issue, preset });
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button size="sm" variant="outline" className="h-7 gap-1 cursor-pointer">
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
}: {
  issue: GitHubIssue;
  presets: TaskPreset[];
  onStartTask: IssueListProps["onStartTask"];
}) {
  const StateIcon = issue.state === "open" ? IconCircle : IconCircleCheck;
  const stateClass =
    issue.state === "open"
      ? "text-emerald-600 dark:text-emerald-400"
      : "text-purple-600 dark:text-purple-400";
  return (
    <div className="flex items-start gap-3 px-4 py-3 hover:bg-muted/40 transition-colors">
      <StateIcon className={cn("h-4 w-4 mt-1 shrink-0", stateClass)} />
      <div className="min-w-0 flex-1">
        <Link
          href={issue.html_url}
          target="_blank"
          rel="noopener noreferrer"
          className="text-sm font-semibold hover:underline block truncate cursor-pointer"
        >
          {issue.title}
        </Link>
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1 mt-0.5 text-xs text-muted-foreground">
          <span className="whitespace-nowrap">
            {issue.repo_owner}/{issue.repo_name}#{issue.number}
          </span>
          <span>·</span>
          <span className="whitespace-nowrap">
            by {issue.author_login} · opened {formatRelativeTime(issue.created_at)}
          </span>
          <IssueLabels labels={issue.labels} />
        </div>
      </div>
      <div className="shrink-0">
        <StartTaskMenu issue={issue} presets={presets} onStartTask={onStartTask} />
      </div>
    </div>
  );
}

function IssueListBody({ loading, error, items, presets, onStartTask }: IssueListProps) {
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
        No issues match this filter.
      </div>
    );
  }
  return (
    <div className="divide-y">
      {items.map((issue) => (
        <IssueRow
          key={`${issue.repo_owner}/${issue.repo_name}#${issue.number}`}
          issue={issue}
          presets={presets}
          onStartTask={onStartTask}
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
