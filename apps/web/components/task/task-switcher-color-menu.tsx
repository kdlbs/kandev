"use client";

import { IconCheck, IconPalette } from "@tabler/icons-react";
import {
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuSub,
  ContextMenuSubContent,
  ContextMenuSubTrigger,
} from "@kandev/ui/context-menu";
import { useSetTaskColor, useTaskColor } from "@/hooks/use-task-color";
import {
  TASK_COLORS,
  TASK_COLOR_BAR_CLASS,
  TASK_COLOR_LABEL,
  type TaskColor,
} from "@/lib/task-colors";
import { cn } from "@/lib/utils";

export function TaskColorMenu({ taskId, disabled }: { taskId: string; disabled?: boolean }) {
  const currentColor = useTaskColor(taskId);
  const setColor = useSetTaskColor();
  return (
    <ContextMenuSub>
      <ContextMenuSubTrigger disabled={disabled}>
        <IconPalette className="mr-2 h-4 w-4" />
        Color
        {currentColor && (
          <span
            className={cn(
              "ml-2 inline-block h-2 w-2 rounded-full",
              TASK_COLOR_BAR_CLASS[currentColor],
            )}
          />
        )}
      </ContextMenuSubTrigger>
      <ContextMenuSubContent className="w-40">
        {TASK_COLORS.map((color) => (
          <TaskColorMenuItem
            key={color}
            color={color}
            selected={currentColor === color}
            onSelect={() => setColor(taskId, color)}
          />
        ))}
        <ContextMenuSeparator />
        <ContextMenuItem disabled={!currentColor} onSelect={() => setColor(taskId, null)}>
          <span className="mr-2 inline-block h-2 w-2 rounded-full border border-muted-foreground/40" />
          None
        </ContextMenuItem>
      </ContextMenuSubContent>
    </ContextMenuSub>
  );
}

function TaskColorMenuItem({
  color,
  selected,
  onSelect,
}: {
  color: TaskColor;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <ContextMenuItem onSelect={onSelect}>
      <span className={cn("mr-2 inline-block h-2 w-2 rounded-full", TASK_COLOR_BAR_CLASS[color])} />
      {TASK_COLOR_LABEL[color]}
      {selected && <IconCheck className="ml-auto h-3.5 w-3.5" />}
    </ContextMenuItem>
  );
}
