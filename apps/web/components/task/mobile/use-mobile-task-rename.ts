"use client";

import { useCallback, useState } from "react";
import { useTaskActions } from "@/hooks/use-task-actions";

export function useMobileTaskRename() {
  const { renameTaskById } = useTaskActions();
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
      } finally {
        setRenamingTask(null);
      }
    },
    [renameTaskById, renamingTask],
  );

  return { renamingTask, setRenamingTask, handleRenameTask, handleRenameSubmit };
}
