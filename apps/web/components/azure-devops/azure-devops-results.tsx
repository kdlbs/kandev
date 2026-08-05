"use client";

import {
  IconChevronDown,
  IconExternalLink,
  IconMessageCircle,
  IconPlus,
} from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import type {
  AzureDevOpsActionPreset,
  AzureDevOpsPullRequest,
  AzureDevOpsWorkItem,
} from "@/lib/types/azure-devops";
import { useTranslation } from "react-i18next";

function EmptyResult({ loading, error }: { loading: boolean; error: string | null }) {
  const { t } = useTranslation();
  if (loading)
    return (
      <div className="p-6 text-sm text-muted-foreground">{t("azuredevops:loadingResults")}</div>
    );
  if (error)
    return (
      <div className="p-6 text-sm text-destructive" role="alert">
        {error}
      </div>
    );
  return (
    <div className="p-6 text-sm text-muted-foreground">{t("azuredevops:noMatchingResults")}</div>
  );
}

// A discriminant, not copy: the aria-label is a whole sentence per kind, because
// interpolating an English noun into a translated sentence is untranslatable.
type TaskActionsItemKind = "workItem" | "pullRequest";

const TASK_ACTIONS_LABEL_KEYS: Record<TaskActionsItemKind, string> = {
  workItem: "azuredevops:taskActionsForWorkItem",
  pullRequest: "azuredevops:taskActionsForPullRequest",
};

function TaskActionsMenu<T extends { id: number }>({
  item,
  itemKind,
  actions,
  onSelect,
}: {
  item: T;
  itemKind: TaskActionsItemKind;
  actions: AzureDevOpsActionPreset[];
  onSelect: (item: T, action: AzureDevOpsActionPreset) => void;
}) {
  const { t } = useTranslation();
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="cursor-pointer"
          aria-label={t(TASK_ACTIONS_LABEL_KEYS[itemKind], { id: item.id })}
        >
          <IconPlus className="h-4 w-4" /> {t("azuredevops:task")}{" "}
          <IconChevronDown className="h-3 w-3" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {actions.map((action) => (
          <DropdownMenuItem key={action.id} onSelect={() => onSelect(item, action)}>
            <span className="flex flex-col">
              <span>{action.label}</span>
              {action.hint && <span className="text-xs text-muted-foreground">{action.hint}</span>}
            </span>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export function AzureDevOpsWorkItemResults({
  items,
  loading,
  error,
  onStartTask,
  onOpenDetail,
  quickActions = [],
  onQuickAction,
}: {
  items: AzureDevOpsWorkItem[];
  loading: boolean;
  error: string | null;
  onStartTask: (item: AzureDevOpsWorkItem) => void;
  onOpenDetail?: (item: AzureDevOpsWorkItem) => void;
  quickActions?: AzureDevOpsActionPreset[];
  onQuickAction?: (item: AzureDevOpsWorkItem, action: AzureDevOpsActionPreset) => void;
}) {
  const { t } = useTranslation();
  if (loading || error || items.length === 0)
    return <EmptyResult loading={loading} error={error} />;
  return (
    <div className="divide-y" data-testid="azure-devops-work-item-results">
      {items.map((item) => (
        <div key={item.id} className="flex flex-col gap-3 px-4 py-3 sm:flex-row sm:items-start">
          <div
            className="min-w-0 flex-1 cursor-pointer space-y-1"
            role={onOpenDetail ? "button" : undefined}
            tabIndex={onOpenDetail ? 0 : undefined}
            onClick={() => onOpenDetail?.(item)}
            onKeyDown={(event) => {
              if (onOpenDetail && (event.key === "Enter" || event.key === " ")) {
                event.preventDefault();
                onOpenDetail(item);
              }
            }}
            data-testid={`azure-work-item-row-${item.id}`}
          >
            <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
              <span className="font-mono">#{item.id}</span>
              <Badge variant="outline">{item.type}</Badge>
              <span>{item.state}</span>
            </div>
            <div className="break-words text-sm font-medium">{item.title}</div>
            <div className="text-xs text-muted-foreground">
              {item.assignedTo || t("azuredevops:unassigned")}
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
                  aria-label={t("azuredevops:openWorkItemInAzureDevops", { id: item.id })}
                >
                  <IconExternalLink className="h-4 w-4" />
                </a>
              </Button>
            )}
            {quickActions.length > 0 && onQuickAction ? (
              <TaskActionsMenu
                item={item}
                itemKind="workItem"
                actions={quickActions}
                onSelect={onQuickAction}
              />
            ) : (
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="cursor-pointer"
                onClick={() => onStartTask(item)}
              >
                <IconPlus className="h-4 w-4" />
                {t("azuredevops:startTask")}
              </Button>
            )}
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
  quickActions = [],
  onQuickAction,
}: {
  items: AzureDevOpsPullRequest[];
  loading: boolean;
  error: string | null;
  onFeedback: (pullRequest: AzureDevOpsPullRequest) => void;
  onStartTask: (pullRequest: AzureDevOpsPullRequest) => void;
  quickActions?: AzureDevOpsActionPreset[];
  onQuickAction?: (pullRequest: AzureDevOpsPullRequest, action: AzureDevOpsActionPreset) => void;
}) {
  const { t } = useTranslation();
  if (loading || error || items.length === 0)
    return <EmptyResult loading={loading} error={error} />;
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
              {pullRequest.isDraft && <Badge variant="secondary">{t("azuredevops:draft")}</Badge>}
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
              {t("azuredevops:feedback")}
            </Button>
            {quickActions.length > 0 && onQuickAction ? (
              <TaskActionsMenu
                item={pullRequest}
                itemKind="pullRequest"
                actions={quickActions}
                onSelect={onQuickAction}
              />
            ) : (
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="cursor-pointer"
                onClick={() => onStartTask(pullRequest)}
              >
                <IconPlus className="h-4 w-4" />
                {t("azuredevops:startTask")}
              </Button>
            )}
            <Button asChild variant="ghost" size="icon-sm" className="cursor-pointer">
              <a
                href={pullRequest.webUrl}
                target="_blank"
                rel="noreferrer"
                aria-label={t("azuredevops:openPullRequestInAzureDevops", { id: pullRequest.id })}
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
