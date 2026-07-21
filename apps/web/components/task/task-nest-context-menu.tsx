"use client";

import { IconArrowUpRight, IconSubtask } from "@tabler/icons-react";
import {
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuSub,
  ContextMenuSubContent,
  ContextMenuSubTrigger,
} from "@kandev/ui/context-menu";
import { useAppStore } from "@/components/state-provider";
import { useNestTask } from "@/hooks/use-nest-task";
import { computeNestCandidates } from "@/lib/sidebar/nest-candidates";
import type { TaskSwitcherItem } from "./task-switcher";

type TaskNestContextMenuItemsProps = {
  task: TaskSwitcherItem;
  disabled?: boolean;
};

/**
 * "Nest" sub-menu: nest a task under another task (make it a sub-task),
 * re-parent it, or un-nest it back to the root. Candidate parents are the
 * other tasks in the same workflow, excluding the task's own descendants
 * (which would create a cycle) and its current parent.
 */
export function TaskNestContextMenuItems({ task, disabled }: TaskNestContextMenuItemsProps) {
  const workflowId = task.workflowId;
  const tasks = useAppStore((s) =>
    workflowId ? s.kanbanMulti?.snapshots?.[workflowId]?.tasks : undefined,
  );
  const nestTask = useNestTask();

  if (!workflowId) return null;

  const candidates = computeNestCandidates(tasks ?? [], task.id);
  const hasParent = Boolean(task.parentTaskId);

  return (
    <ContextMenuSub>
      <ContextMenuSubTrigger disabled={disabled}>
        <IconSubtask className="mr-2 h-4 w-4" />
        Nest under
      </ContextMenuSubTrigger>
      <ContextMenuSubContent className="max-h-72 w-56 overflow-y-auto">
        {hasParent && (
          <>
            <ContextMenuItem
              className="cursor-pointer"
              onSelect={() => void nestTask(task.id, workflowId, null)}
            >
              <IconArrowUpRight className="mr-2 h-4 w-4" />
              Un-nest (remove parent)
            </ContextMenuItem>
            <ContextMenuSeparator />
          </>
        )}
        {candidates.length === 0 ? (
          <ContextMenuItem disabled>No other tasks</ContextMenuItem>
        ) : (
          candidates.map((candidate) => (
            <ContextMenuItem
              key={candidate.id}
              className="cursor-pointer"
              onSelect={() => void nestTask(task.id, workflowId, candidate.id)}
            >
              <span className="truncate">{candidate.title}</span>
            </ContextMenuItem>
          ))
        )}
      </ContextMenuSubContent>
    </ContextMenuSub>
  );
}
