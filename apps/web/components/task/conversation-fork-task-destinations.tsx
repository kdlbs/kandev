"use client";

import { TaskCreateDialog } from "@/components/task-create-dialog";
import type { TaskCreateDialogProps } from "@/components/task-create-dialog-types";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import { NewSubtaskDialog } from "./new-subtask-dialog";
import type { ConversationForkFormContext } from "./conversation-fork-types";
import { useRouter } from "@/lib/routing/client-router";
import { linkToTask } from "@/lib/links";

type Destination = "task" | "child_task" | null;
type SourceTask = KanbanState["tasks"][number];

export function ConversationForkTaskDestinations({
  destination,
  sourceTask,
  workspaceId,
  steps,
  conversationFork,
  onClose,
}: {
  destination: Destination;
  sourceTask: SourceTask;
  workspaceId: string | null;
  steps: TaskCreateDialogProps["steps"];
  conversationFork?: ConversationForkFormContext;
  onClose: () => void;
}) {
  const router = useRouter();
  const isTaskOpen = destination === "task";
  const isChildOpen = destination === "child_task";
  const closeOnDismiss = (open: boolean) => {
    if (!open) onClose();
  };

  return (
    <>
      <TaskCreateDialog
        open={isTaskOpen}
        onOpenChange={closeOnDismiss}
        mode="create"
        workspaceId={workspaceId}
        workflowId={sourceTask.workflowId}
        defaultStepId={sourceTask.workflowStepId}
        steps={steps}
        conversationFork={conversationFork}
        onSuccess={(task) => {
          conversationFork?.onConsumed();
          onClose();
          router.push(linkToTask(task.id));
        }}
      />
      <NewSubtaskDialog
        open={isChildOpen}
        onOpenChange={closeOnDismiss}
        parentTaskId={sourceTask.id}
        parentTaskTitle={sourceTask.title}
        conversationFork={conversationFork}
      />
    </>
  );
}
