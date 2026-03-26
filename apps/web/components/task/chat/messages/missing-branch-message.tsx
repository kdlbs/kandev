"use client";

import { useState, useCallback, memo } from "react";
import { IconAlertTriangle, IconArchive, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { useArchiveAndSwitchTask } from "@/hooks/use-task-actions";
import { useTaskRemoval } from "@/hooks/use-task-removal";
import { useAppStoreApi } from "@/components/state-provider";
import { deleteTask } from "@/lib/api/domains/kanban-api";
import type { Message } from "@/lib/types/http";
import type { MissingBranchMetadata } from "@/components/task/chat/types";

export const MissingBranchMessage = memo(function MissingBranchMessage({
  comment,
}: {
  comment: Message;
}) {
  const metadata = comment.metadata as MissingBranchMetadata | undefined;
  const branch = metadata?.missing_branch ?? "unknown";
  const taskId = useAppStore((s) => s.tasks.activeTaskId);
  const store = useAppStoreApi();

  const archiveAndSwitch = useArchiveAndSwitchTask();
  const { removeTaskFromBoard } = useTaskRemoval({ store });
  const [actionState, setActionState] = useState<"idle" | "busy">("idle");

  const handleArchive = useCallback(async () => {
    if (!taskId || actionState === "busy") return;
    setActionState("busy");
    try {
      await archiveAndSwitch(taskId);
    } catch {
      setActionState("idle");
    }
  }, [taskId, actionState, archiveAndSwitch]);

  const handleDelete = useCallback(async () => {
    if (!taskId || actionState === "busy") return;
    setActionState("busy");
    try {
      const { activeTaskId: wasActiveTaskId, activeSessionId: wasActiveSessionId } =
        store.getState().tasks;
      await deleteTask(taskId);
      await removeTaskFromBoard(taskId, { wasActiveTaskId, wasActiveSessionId });
    } catch {
      setActionState("idle");
    }
  }, [taskId, actionState, store, removeTaskFromBoard]);

  const disabled = actionState === "busy";

  return (
    <div className="w-full">
      <div className="flex items-start gap-3 w-full rounded px-2 py-1 -mx-2">
        <div className="flex-shrink-0 mt-0.5">
          <IconAlertTriangle className="h-4 w-4 text-amber-500" />
        </div>
        <div className="flex-1 min-w-0 pt-0.5">
          <div className="text-xs font-mono text-amber-600 dark:text-amber-400">
            The PR branch &quot;{branch}&quot; no longer exists (likely merged and deleted).
          </div>
          <div className="mt-2 flex items-center gap-2">
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="outline"
                  size="sm"
                  className="h-7 text-xs cursor-pointer gap-1.5"
                  disabled={disabled}
                  onClick={handleArchive}
                  data-testid="missing-branch-archive-button"
                >
                  <IconArchive className="h-3 w-3" />
                  Archive task
                </Button>
              </TooltipTrigger>
              <TooltipContent side="top">
                Keep task history and hide it from active work
              </TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="outline"
                  size="sm"
                  className="h-7 text-xs cursor-pointer gap-1.5 text-destructive hover:text-destructive"
                  disabled={disabled}
                  onClick={handleDelete}
                  data-testid="missing-branch-delete-button"
                >
                  <IconTrash className="h-3 w-3" />
                  Delete task
                </Button>
              </TooltipTrigger>
              <TooltipContent side="top">
                Permanently remove this task
              </TooltipContent>
            </Tooltip>
          </div>
        </div>
      </div>
    </div>
  );
});
