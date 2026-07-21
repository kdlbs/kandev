"use client";

import Link from "@/components/routing/app-link";
import { IconCircle, IconCircleCheck, IconExternalLink } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Spinner } from "@kandev/ui/spinner";
import { cn, formatRelativeTime } from "@/lib/utils";
import type { Issue } from "@/lib/types/gitlab";

type IssueListProps = {
  items: Issue[];
  loading: boolean;
  error: string | null;
};

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

function IssueRow({ issue }: { issue: Issue }) {
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
        <Link
          href={issue.web_url}
          target="_blank"
          rel="noopener noreferrer"
          className="text-sm font-semibold hover:underline inline-flex items-center gap-1.5 truncate cursor-pointer"
        >
          <span className="truncate">{issue.title}</span>
          <IconExternalLink className="h-3 w-3 shrink-0 text-muted-foreground" />
        </Link>
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1 mt-0.5 text-xs text-muted-foreground">
          <span className="whitespace-nowrap">
            {issue.project_path}#{issue.iid}
          </span>
          <span>·</span>
          <span className="whitespace-nowrap">
            by {issue.author_username} · opened {formatRelativeTime(issue.created_at)}
          </span>
          <IssueLabels labels={issue.labels} />
        </div>
      </div>
    </div>
  );
}

function IssueListBody({ loading, error, items }: IssueListProps) {
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
        <IssueRow key={`${issue.project_path}-${issue.iid}`} issue={issue} />
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
