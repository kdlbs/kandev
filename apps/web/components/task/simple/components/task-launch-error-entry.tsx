"use client";

import { useState } from "react";
import {
  IconAlertTriangle,
  IconCheck,
  IconGitBranch,
  IconLoader2,
  IconRefresh,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useToast } from "@/components/toast-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { TaskRepository } from "@/lib/types/http";
import type { TaskLaunchRecoveryAction } from "@/lib/types/task-launch-error";
import type { TaskStatusSummaryActiveError } from "@/lib/types/task-status-summary";
import { TaskLaunchBranchPicker } from "@/components/task/task-launch-branch-picker";
import { useTranslation } from "react-i18next";

type TaskLaunchErrorEntryProps = {
  taskId: string;
  workspaceId: string;
  error: TaskStatusSummaryActiveError;
  repositories?: TaskRepository[];
};

const pendingRecoveryRequests = new Map<string, Promise<unknown>>();

const TASK_LAUNCH_ERROR_CATEGORIES = new Set([
  "base_branch_missing",
  "default_branch_unresolved",
  "pr_already_closed",
  "generic_launch_failure",
]);

export function isLaunchErrorCategory(category: string | undefined): boolean {
  return Boolean(category && TASK_LAUNCH_ERROR_CATEGORIES.has(category));
}

export function isTypedTaskLaunchError(
  error: TaskStatusSummaryActiveError | null | undefined,
): error is TaskStatusSummaryActiveError & { category: string } {
  return Boolean(error?.stamp && isLaunchErrorCategory(error.category));
}

function categoryTitle(category: string | undefined, t: (key: string) => string): string {
  switch (category) {
    case "base_branch_missing":
      return t("task:launchErrorBaseBranchMissing");
    case "default_branch_unresolved":
      return t("task:launchErrorDefaultBranchUnresolved");
    case "pr_already_closed":
      return t("task:launchErrorPrAlreadyClosed");
    default:
      return t("task:launchErrorGeneric");
  }
}

function actionLabel(action: TaskLaunchRecoveryAction, t: (key: string) => string): string {
  switch (action) {
    case "retry_default":
      return t("task:retryWithDefaultBranch");
    case "pick_base_branch":
      return t("task:pickBaseBranch");
    case "mark_review_done":
      return t("task:markReviewDone");
  }
}

function RecoveryActionIcon({
  action,
  pending,
}: {
  action: TaskLaunchRecoveryAction;
  pending: boolean;
}) {
  if (pending) return <IconLoader2 className="h-3 w-3 animate-spin" />;
  if (action === "retry_default") return <IconRefresh className="h-3 w-3" />;
  if (action === "pick_base_branch") return <IconGitBranch className="h-3 w-3" />;
  return <IconCheck className="h-3 w-3" />;
}

function TaskLaunchRecoveryActions({
  actions,
  workspaceId,
  repositories,
  taskRepositoryId,
  currentBase,
  pendingAction,
  onRecover,
}: {
  actions: TaskLaunchRecoveryAction[];
  workspaceId: string;
  repositories?: TaskRepository[];
  taskRepositoryId?: string;
  currentBase: string;
  pendingAction: TaskLaunchRecoveryAction | null;
  onRecover: (action: TaskLaunchRecoveryAction, baseBranch?: string) => Promise<void>;
}) {
  const { t } = useTranslation();
  return (
    <div className="mt-2 flex min-w-0 flex-wrap items-center gap-2">
      {actions.map((action) => {
        const opensBranchPicker = action === "pick_base_branch";
        const button = (
          <Button
            variant="outline"
            size="sm"
            className="h-auto min-h-11 max-w-full cursor-pointer gap-1.5 text-xs sm:min-h-8"
            disabled={pendingAction !== null}
            onClick={opensBranchPicker ? undefined : () => void onRecover(action)}
            data-testid={`task-launch-${action}-button`}
          >
            <RecoveryActionIcon action={action} pending={pendingAction === action} />
            <span className="truncate">{actionLabel(action, t)}</span>
          </Button>
        );
        if (action !== "pick_base_branch") return <span key={action}>{button}</span>;
        return (
          <TaskLaunchBranchPicker
            key={action}
            workspaceId={workspaceId}
            repositories={repositories}
            taskRepositoryId={taskRepositoryId}
            currentBase={currentBase}
            onSelect={(branch) => onRecover(action, branch)}
            trigger={button}
          />
        );
      })}
    </div>
  );
}

export function TaskLaunchErrorEntry({
  taskId,
  workspaceId,
  error,
  repositories,
}: TaskLaunchErrorEntryProps) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const [pendingAction, setPendingAction] = useState<TaskLaunchRecoveryAction | null>(null);
  const taskRepository = repositories?.find(
    (repository) => repository.id === error.task_repository_id,
  );
  const currentBase = taskRepository?.base_branch ?? t("task:baseBranchNotSet");

  const sendRecovery = async (action: TaskLaunchRecoveryAction, baseBranch?: string) => {
    const client = getWebSocketClient();
    if (!client || pendingAction) return;
    const requestKey = JSON.stringify({ taskId, stamp: error.stamp, action, baseBranch });
    const existingRequest = pendingRecoveryRequests.get(requestKey);
    if (existingRequest) {
      setPendingAction(action);
      try {
        await existingRequest;
      } catch {
        // The request owner reports the recovery error to avoid duplicate toasts.
      } finally {
        setPendingAction(null);
      }
      return;
    }
    setPendingAction(action);
    const request = Promise.resolve().then(() =>
      client.request("task.launch.recover", {
        task_id: taskId,
        ...(error.session_id ? { session_id: error.session_id } : {}),
        ...(error.task_repository_id ? { task_repository_id: error.task_repository_id } : {}),
        action,
        ...(baseBranch ? { base_branch: baseBranch } : {}),
        error_stamp: error.stamp,
      }),
    );
    pendingRecoveryRequests.set(requestKey, request);
    try {
      await request;
    } catch {
      toast({
        title: t("task:launchRecoveryFailed"),
        description: t("task:tryRecoveryAgain"),
        variant: "error",
      });
    } finally {
      pendingRecoveryRequests.delete(requestKey);
      setPendingAction(null);
    }
  };

  return (
    <div
      className="flex min-w-0 gap-3 border-b border-border/50 py-3"
      data-testid="task-launch-error-entry"
      role="alert"
    >
      <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-red-500/10 text-red-600 dark:text-red-400">
        <IconAlertTriangle className="h-4 w-4" aria-hidden="true" />
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <span className="text-sm font-medium">{categoryTitle(error.category, t)}</span>
          <span className="text-xs text-muted-foreground">{t("task:launchNeedsAttention")}</span>
        </div>
        <p className="mt-1 max-w-prose break-words text-sm text-muted-foreground">
          {error.preview}
        </p>
        <TaskLaunchRecoveryActions
          actions={error.recovery_actions ?? []}
          workspaceId={workspaceId}
          repositories={repositories}
          taskRepositoryId={error.task_repository_id}
          currentBase={currentBase}
          pendingAction={pendingAction}
          onRecover={sendRecovery}
        />
      </div>
    </div>
  );
}
