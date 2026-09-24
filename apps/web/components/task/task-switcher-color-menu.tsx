"use client";

import { IconPalette } from "@tabler/icons-react";
import {
  ContextMenuItem,
  ContextMenuRadioGroup,
  ContextMenuRadioItem,
  ContextMenuSeparator,
  ContextMenuSub,
  ContextMenuSubContent,
  ContextMenuSubTrigger,
} from "@kandev/ui/context-menu";
import { useTaskColorSelection } from "@/hooks/use-task-color-selection";
import {
  TASK_COLORS,
  TASK_COLOR_BAR_CLASS,
  TASK_COLOR_LABEL_KEYS,
  type TaskColor,
} from "@/lib/task-colors";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import type { AutomaticTaskColorSource } from "@/lib/sidebar/task-color-rules";

export function TaskColorMenu({
  taskId,
  taskIds,
  disabled,
  automaticColorSource,
}: {
  taskId?: string;
  taskIds?: string[];
  disabled?: boolean;
  automaticColorSource?: AutomaticTaskColorSource;
}) {
  const { t } = useTranslation();
  const {
    ids,
    commonColor: currentColor,
    hasColor,
    setColors,
    isPending,
  } = useTaskColorSelection(taskIds ?? (taskId ? [taskId] : []));
  return (
    <ContextMenuSub>
      <ContextMenuSubTrigger disabled={disabled || isPending || ids.length === 0}>
        <IconPalette className="mr-2 h-4 w-4" />
        {t("task:color")}
        {isPending && <span role="status">{t("task:bulkColorSaving")}</span>}
        {currentColor && (
          <span
            className={cn(
              "ml-2 inline-block h-2 w-2 rounded-full",
              TASK_COLOR_BAR_CLASS[currentColor],
            )}
          />
        )}
      </ContextMenuSubTrigger>
      <ContextMenuSubContent className="w-64">
        {taskIds && (
          <div className="px-2 py-1.5 text-xs text-muted-foreground">
            {t("task:bulkColorAutomaticHint")}
          </div>
        )}
        {automaticColorSource && (
          <>
            <ContextMenuItem disabled data-testid="automatic-task-color-source">
              {t("task:automaticColorSource", { rule: automaticColorSource.label })}
            </ContextMenuItem>
            <ContextMenuSeparator />
          </>
        )}
        <ContextMenuRadioGroup value={currentColor ?? ""}>
          {TASK_COLORS.map((color) => (
            <TaskColorMenuItem
              key={color}
              color={color}
              disabled={isPending}
              onSelect={() => {
                void setColors(ids, color);
              }}
            />
          ))}
        </ContextMenuRadioGroup>
        <ContextMenuSeparator />
        <ContextMenuItem
          disabled={isPending || !hasColor}
          onSelect={() => {
            void setColors(ids, null);
          }}
        >
          <span className="mr-2 inline-block h-2 w-2 rounded-full border border-muted-foreground/40" />
          {t("task:groupNone")}
        </ContextMenuItem>
      </ContextMenuSubContent>
    </ContextMenuSub>
  );
}

function TaskColorMenuItem({
  color,
  disabled,
  onSelect,
}: {
  color: TaskColor;
  disabled?: boolean;
  onSelect: () => void;
}) {
  const { t } = useTranslation();
  return (
    <ContextMenuRadioItem value={color} disabled={disabled} onSelect={onSelect}>
      <span className={cn("mr-2 inline-block h-2 w-2 rounded-full", TASK_COLOR_BAR_CLASS[color])} />
      {t(TASK_COLOR_LABEL_KEYS[color])}
    </ContextMenuRadioItem>
  );
}
