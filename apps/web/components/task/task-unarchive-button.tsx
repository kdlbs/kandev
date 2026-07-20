"use client";

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { IconArchiveOff, IconLoader } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { unarchiveTask } from "@/lib/api";
import { unarchiveToastPayload } from "@/lib/tasks/unarchive-feedback";
import { reconcileUnarchiveTaskQueries } from "@/lib/query/task-cache";
import { useToast } from "@/components/toast-provider";

// Shown in the task top bar when the task is archived. The mutation response
// reconciles Query immediately; task.updated remains the cross-client update.
export function TaskUnarchiveButton({ taskId }: { taskId?: string | null }) {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const [isPending, setIsPending] = useState(false);
  if (!taskId) return null;

  const handleClick = async () => {
    setIsPending(true);
    try {
      const result = await unarchiveTask(taskId);
      reconcileUnarchiveTaskQueries(queryClient, result);
      toast(unarchiveToastPayload(result));
    } catch (err) {
      toast({
        title: "Failed to unarchive task",
        description: err instanceof Error ? err.message : "Unknown error",
        variant: "error",
      });
    } finally {
      setIsPending(false);
    }
  };

  return (
    <Button
      size="sm"
      variant="outline"
      className="h-7 cursor-pointer px-2"
      disabled={isPending}
      onClick={handleClick}
      data-testid="task-unarchive-button"
    >
      {isPending ? (
        <IconLoader className="h-3.5 w-3.5 animate-spin" />
      ) : (
        <IconArchiveOff className="h-3.5 w-3.5" />
      )}
      Unarchive
    </Button>
  );
}
