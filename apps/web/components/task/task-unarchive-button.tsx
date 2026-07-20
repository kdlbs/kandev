"use client";

import { useState } from "react";
import { IconArchiveOff, IconLoader } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { unarchiveTask } from "@/lib/api";
import { unarchiveToastPayload } from "@/lib/tasks/unarchive-feedback";
import { useToast } from "@/components/toast-provider";

export function TaskUnarchiveButton({
  taskId,
  onUnarchived,
}: {
  taskId?: string | null;
  onUnarchived?: (taskId: string) => void;
}) {
  const { toast } = useToast();
  const [isPending, setIsPending] = useState(false);
  if (!taskId) return null;

  const handleClick = async () => {
    setIsPending(true);
    try {
      const result = await unarchiveTask(taskId);
      toast(unarchiveToastPayload(result));
      if (result.success && result.unarchived_ids.includes(taskId)) {
        onUnarchived?.(taskId);
      } else if (result.success) {
        console.warn("[TaskUnarchiveButton] task missing from successful response", taskId);
      }
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
