"use client";

import { IconFlag } from "@tabler/icons-react";
import {
  ContextMenuItem,
  ContextMenuSub,
  ContextMenuSubContent,
  ContextMenuSubTrigger,
} from "@kandev/ui/context-menu";
import { useTranslation } from "react-i18next";
import {
  isTaskPriority,
  TASK_PRIORITY_LABEL_KEYS,
  TASK_PRIORITY_TOKENS,
} from "@/lib/tasks/task-priority";
import type { TaskPriority } from "@/lib/types/http";

export function TaskPriorityContextMenu({
  currentPriority,
  disabled,
  onSelect,
}: {
  currentPriority?: string | null;
  disabled?: boolean;
  onSelect: (priority: TaskPriority) => void;
}) {
  const { t } = useTranslation();

  return (
    <ContextMenuSub>
      <ContextMenuSubTrigger data-testid="task-context-priority" disabled={disabled}>
        <IconFlag className="mr-2 h-4 w-4" />
        {t("kanban:priority")}
      </ContextMenuSubTrigger>
      <ContextMenuSubContent className="w-40">
        {TASK_PRIORITY_TOKENS.map((priority) => {
          const isCurrent = isTaskPriority(currentPriority) && currentPriority === priority;
          return (
            <ContextMenuItem
              key={priority}
              data-testid={`task-context-priority-${priority}`}
              onSelect={() => onSelect(priority)}
            >
              <span className="flex-1 truncate">{t(TASK_PRIORITY_LABEL_KEYS[priority])}</span>
              {isCurrent ? (
                <span
                  data-testid={`task-context-priority-current-${priority}`}
                  className="ml-auto text-[10px] text-muted-foreground"
                >
                  {t("kanban:current")}
                </span>
              ) : null}
            </ContextMenuItem>
          );
        })}
      </ContextMenuSubContent>
    </ContextMenuSub>
  );
}
