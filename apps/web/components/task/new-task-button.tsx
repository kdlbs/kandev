"use client";

import { memo, useMemo, useState } from "react";
import { IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { TaskCreateDialog } from "@/components/task-create-dialog";
import { useAppStore } from "@/components/state-provider";
import { useRegisterCommands } from "@/hooks/use-register-commands";
import { getShortcut } from "@/lib/keyboard/shortcut-overrides";
import type { CommandItem } from "@/lib/commands/types";
import type { Task } from "@/lib/types/http";

type NewTaskButtonProps = {
  workspaceId: string | null;
  workflowId: string | null;
  steps: Array<{
    id: string;
    title: string;
    color?: string;
    events?: {
      on_enter?: Array<{ type: string; config?: Record<string, unknown> }>;
      on_turn_complete?: Array<{ type: string; config?: Record<string, unknown> }>;
    };
  }>;
  onSuccess: (
    task: Task,
    mode: "create" | "edit",
    meta?: { taskSessionId?: string | null },
  ) => void;
};

export const NewTaskButton = memo(function NewTaskButton({
  workspaceId,
  workflowId,
  steps,
  onSuccess,
}: NewTaskButtonProps) {
  const [dialogOpen, setDialogOpen] = useState(false);

  const keyboardShortcuts = useAppStore((s) => s.userSettings.keyboardShortcuts);
  const newTaskShortcut = getShortcut("NEW_TASK", keyboardShortcuts);
  const commands = useMemo<CommandItem[]>(
    () => [
      {
        id: "task-create",
        label: "Create New Task",
        group: "Tasks",
        icon: <IconPlus className="size-3.5" />,
        shortcut: newTaskShortcut,
        keywords: ["new", "create", "task", "add"],
        action: () => setDialogOpen(true),
        priority: 0,
      },
    ],
    [newTaskShortcut],
  );
  useRegisterCommands(commands);

  return (
    <>
      <Button
        size="sm"
        variant="outline"
        className="h-6 gap-1 cursor-pointer"
        onClick={() => setDialogOpen(true)}
      >
        <IconPlus className="h-4 w-4" />
        Task
      </Button>
      <TaskCreateDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        mode="create"
        workspaceId={workspaceId}
        workflowId={workflowId}
        defaultStepId={steps[0]?.id ?? null}
        steps={steps}
        onSuccess={onSuccess}
      />
    </>
  );
});
