"use client";

import { IconExternalLink, IconMessageCircle, IconPlus } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import type { AzureDevOpsPullRequest, AzureDevOpsWorkItem } from "@/lib/types/azure-devops";

function EmptyResult({ loading, error }: { loading: boolean; error: string | null }) {
  if (loading) return <div className="p-6 text-sm text-muted-foreground">Loading results...</div>;
  if (error)
    return (
      <div className="p-6 text-sm text-destructive" role="alert">
        {error}
      </div>
    );
  return <div className="p-6 text-sm text-muted-foreground">No matching results.</div>;
}

export function AzureDevOpsWorkItemResults({
  items,
  loading,
  error,
  onStartTask,
}: {
  items: AzureDevOpsWorkItem[];
  loading: boolean;
  error: string | null;
  onStartTask: (item: AzureDevOpsWorkItem) => void;
}) {
  if (items.length === 0) return <EmptyResult loading={loading} error={error} />;
  return (
    <div className="divide-y" data-testid="azure-devops-work-item-results">
      {items.map((item) => (
        <div key={item.id} className="flex flex-col gap-3 px-4 py-3 sm:flex-row sm:items-start">
          <div className="min-w-0 flex-1 space-y-1">
            <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
              <span className="font-mono">#{item.id}</span>
              <Badge variant="outline">{item.type}</Badge>
              <span>{item.state}</span>
            </div>
            <div className="break-words text-sm font-medium">{item.title}</div>
            <div className="text-xs text-muted-foreground">
              {item.assignedTo || "Unassigned"}
              {item.areaPath ? ` · ${item.areaPath}` : ""}
            </div>
          </div>
          <div className="flex shrink-0 gap-2">
            {item.webUrl && (
              <Button asChild variant="ghost" size="icon-sm" className="cursor-pointer">
                <a
                  href={item.webUrl}
                  target="_blank"
                  rel="noreferrer"
                  aria-label={`Open work item ${item.id} in Azure DevOps`}
                >
                  <IconExternalLink className="h-4 w-4" />
                </a>
              </Button>
            )}
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="cursor-pointer"
              onClick={() => onStartTask(item)}
            >
              <IconPlus className="h-4 w-4" />
              Start task
            </Button>
          </div>
        </div>
      ))}
    </div>
  );
}

function shortBranch(value: string): string {
  return value.replace(/^refs\/heads\//, "");
}

export function AzureDevOpsPullRequestResults({
  items,
  loading,
  error,
  onFeedback,
  onStartTask,
}: {
  items: AzureDevOpsPullRequest[];
  loading: boolean;
  error: string | null;
  onFeedback: (pullRequest: AzureDevOpsPullRequest) => void;
  onStartTask: (pullRequest: AzureDevOpsPullRequest) => void;
}) {
  if (items.length === 0) return <EmptyResult loading={loading} error={error} />;
  return (
    <div className="divide-y" data-testid="azure-devops-pull-request-results">
      {items.map((pullRequest) => (
        <div
          key={`${pullRequest.repositoryId}:${pullRequest.id}`}
          className="flex flex-col gap-3 px-4 py-3 sm:flex-row sm:items-start"
        >
          <div className="min-w-0 flex-1 space-y-1">
            <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
              <span className="font-mono">PR {pullRequest.id}</span>
              <Badge variant="outline">{pullRequest.status}</Badge>
              {pullRequest.isDraft && <Badge variant="secondary">Draft</Badge>}
              <span>{pullRequest.repositoryName}</span>
            </div>
            <div className="break-words text-sm font-medium">{pullRequest.title}</div>
            <div className="break-all text-xs text-muted-foreground">
              {shortBranch(pullRequest.sourceBranch)} → {shortBranch(pullRequest.targetBranch)}
            </div>
          </div>
          <div className="flex shrink-0 flex-wrap gap-2">
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="cursor-pointer"
              onClick={() => onFeedback(pullRequest)}
            >
              <IconMessageCircle className="h-4 w-4" />
              Feedback
            </Button>
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="cursor-pointer"
              onClick={() => onStartTask(pullRequest)}
            >
              <IconPlus className="h-4 w-4" />
              Start task
            </Button>
            <Button asChild variant="ghost" size="icon-sm" className="cursor-pointer">
              <a
                href={pullRequest.webUrl}
                target="_blank"
                rel="noreferrer"
                aria-label={`Open pull request ${pullRequest.id} in Azure DevOps`}
              >
                <IconExternalLink className="h-4 w-4" />
              </a>
            </Button>
          </div>
        </div>
      ))}
    </div>
  );
}
