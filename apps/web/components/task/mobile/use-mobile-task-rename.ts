"use client";

import { useCallback, useState } from "react";
import { useTaskActions } from "@/hooks/use-task-actions";
import { useToast } from "@/components/toast-provider";

export function useMobileTaskRename() {
  const { renameTaskById } = useTaskActions();
  const { toast } = useToast();
  const [renamingTask, setRenamingTask] = useState<{ id: string; title: string } | null>(null);

  const handleRenameTask = useCallback((taskId: string, currentTitle: string) => {
    setRenamingTask({ id: taskId, title: currentTitle });
  }, []);

  const handleRenameSubmit = useCallback(
    async (newTitle: string) => {
      if (!renamingTask) return;
      try {
        await renameTaskById(renamingTask.id, newTitle);
      } catch (error) {
        console.error("Failed to rename task:", error);
        toast({
          title: "Failed to rename task",
          description: error instanceof Error ? error.message : "Failed to rename task",
          variant: "error",
        });
      } finally {
        setRenamingTask(null);
      }
    },
    [renameTaskById, renamingTask, toast],
  );

  return { renamingTask, setRenamingTask, handleRenameTask, handleRenameSubmit };
}
