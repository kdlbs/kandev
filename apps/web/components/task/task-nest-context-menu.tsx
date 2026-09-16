"use client";

import { IconArrowUpRight, IconSubtask } from "@tabler/icons-react";
import {
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuSub,
  ContextMenuSubContent,
  ContextMenuSubTrigger,
} from "@kandev/ui/context-menu";
import { useNestTask } from "@/hooks/use-nest-task";
import { computeNestCandidates } from "@/lib/sidebar/nest-candidates";
import type { TaskSwitcherItem } from "./task-switcher";
import { useTranslation } from "react-i18next";

type TaskNestContextMenuItemsProps = {
  task: TaskSwitcherItem;
  nestCandidateTasks?: TaskSwitcherItem[];
  disabled?: boolean;
};

/**
 * "Nest" sub-menu: nest a task under another task (make it a sub-task),
 * re-parent it, or un-nest it back to the root. Candidate parents are the
 * other tasks in the same workflow, excluding the task's own descendants
 * (which would create a cycle) and its current parent.
 */
export function TaskNestContextMenuItems({
  task,
  nestCandidateTasks = [],
  disabled,
}: TaskNestContextMenuItemsProps) {
  const { t } = useTranslation();
  const workflowId = task.workflowId;
  const nestTask = useNestTask();

  if (!workflowId || task.isArchived) return null;

  const candidates = computeNestCandidates(
    nestCandidateTasks.filter((candidate) => candidate.workflowId === workflowId),
    task.id,
  );
  const hasParent = Boolean(task.parentTaskId);

  return (
    <ContextMenuSub>
      <ContextMenuSubTrigger disabled={disabled}>
        <IconSubtask className="mr-2 h-4 w-4" />
        {t("task:nestUnder")}
      </ContextMenuSubTrigger>
      <ContextMenuSubContent className="max-h-72 w-56 overflow-y-auto">
        {hasParent && (
          <>
            <ContextMenuItem
              className="cursor-pointer"
              onSelect={() => void nestTask(task.id, workflowId, null)}
            >
              <IconArrowUpRight className="mr-2 h-4 w-4" />
              {t("task:unNestRemoveParent")}
            </ContextMenuItem>
            <ContextMenuSeparator />
          </>
        )}
        {candidates.length === 0 ? (
          <ContextMenuItem disabled>{t("task:noOtherTasks")}</ContextMenuItem>
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
