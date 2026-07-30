"use client";

import { useCallback } from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { TaskNotePanel } from "./task-note-panel";

type TaskNoteEditDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  taskId: string;
  taskTitle: string;
};

export function TaskNoteEditDialog({
  open,
  onOpenChange,
  taskId,
  taskTitle,
}: TaskNoteEditDialogProps) {
  const handleOpenChange = useCallback(
    (nextOpen: boolean) => onOpenChange(nextOpen),
    [onOpenChange],
  );

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="!max-w-4xl w-[92vw] h-[80dvh] flex flex-col p-0 overflow-hidden">
        <DialogHeader className="border-b px-6 py-4">
          <DialogTitle>Edit notes — {taskTitle}</DialogTitle>
          <DialogDescription>Review and update the shared task notes.</DialogDescription>
        </DialogHeader>
        <div className="min-h-0 flex-1">
          {open && <TaskNotePanel taskId={taskId} visible={open} />}
        </div>
      </DialogContent>
    </Dialog>
  );
}
