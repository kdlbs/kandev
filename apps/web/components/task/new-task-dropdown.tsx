"use client";

import { useState } from "react";
import { IconPlus, IconSubtask, IconChevronDown } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { TaskCreateDialog } from "@/components/task-create-dialog";
import { NewSubtaskDialog } from "./new-subtask-dialog";
import type { Task } from "@/lib/types/http";

type NewTaskDropdownProps = {
  workspaceId: string | null;
  workflowId: string | null;
  steps: Array<{ id: string; title: string; color?: string; events?: Record<string, unknown> }>;
  activeTaskId: string | null;
  activeTaskTitle: string;
  onTaskCreated: (
    task: Task,
    mode: "create" | "edit",
    meta?: { taskSessionId?: string | null },
  ) => void;
};

export function NewTaskDropdown({
  workspaceId,
  workflowId,
  steps,
  activeTaskId,
  activeTaskTitle,
  onTaskCreated,
}: NewTaskDropdownProps) {
  const [showTaskDialog, setShowTaskDialog] = useState(false);
  const [showSubtaskDialog, setShowSubtaskDialog] = useState(false);

  return (
    <>
      <div className="flex items-center">
        <Button
          size="sm"
          variant="outline"
          className={`h-6 gap-1 cursor-pointer ${activeTaskId ? "rounded-r-none border-r-0" : ""}`}
          onClick={() => setShowTaskDialog(true)}
        >
          <IconPlus className="h-3.5 w-3.5" />
          Task
        </Button>
        {activeTaskId && (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                size="sm"
                variant="outline"
                className="h-6 px-1 rounded-l-none cursor-pointer"
                data-testid="new-task-chevron"
              >
                <IconChevronDown className="h-3 w-3" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem
                className="cursor-pointer text-xs gap-1.5"
                onClick={() => setShowSubtaskDialog(true)}
                data-testid="new-subtask-button"
              >
                <IconSubtask className="h-3.5 w-3.5" />
                Subtask
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        )}
      </div>
      <TaskCreateDialog
        open={showTaskDialog}
        onOpenChange={setShowTaskDialog}
        mode="create"
        workspaceId={workspaceId}
        workflowId={workflowId}
        defaultStepId={steps[0]?.id ?? null}
        steps={steps}
        onSuccess={onTaskCreated}
      />
      {activeTaskId && (
        <NewSubtaskDialog
          open={showSubtaskDialog}
          onOpenChange={setShowSubtaskDialog}
          parentTaskId={activeTaskId}
          parentTaskTitle={activeTaskTitle}
        />
      )}
    </>
  );
}
