"use client";

import { useCallback, useState } from "react";
import { Breadcrumb, BreadcrumbItem, BreadcrumbList, BreadcrumbPage } from "@kandev/ui/breadcrumb";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { Input } from "@kandev/ui/input";
import { useTaskActions } from "@/hooks/use-task-actions";

type TaskTopBarTitleProps = {
  taskId?: string | null;
  taskTitle?: string;
  isArchived?: boolean;
};

export function TaskTopBarTitle({ taskId, taskTitle, isArchived }: TaskTopBarTitleProps) {
  const { renameTaskById } = useTaskActions();
  const [isEditing, setIsEditing] = useState(false);
  const [draft, setDraft] = useState("");
  const canRename = Boolean(taskId) && !isArchived;

  const handleDoubleClick = useCallback(() => {
    if (!taskId || isArchived) return;
    setDraft(taskTitle ?? "");
    setIsEditing(true);
  }, [taskId, isArchived, taskTitle]);

  const handleCancel = useCallback(() => setIsEditing(false), []);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>) => {
      // IME composition uses Enter to confirm a candidate; let the IME
      // handle it instead of committing the rename.
      if (e.nativeEvent.isComposing) return;
      if (e.key === "Enter") {
        e.preventDefault();
        const trimmed = draft.trim();
        if (taskId && trimmed && trimmed !== taskTitle) {
          renameTaskById(taskId, trimmed).catch((err) =>
            console.error("Failed to rename task:", err),
          );
        }
        setIsEditing(false);
      } else if (e.key === "Escape") {
        e.preventDefault();
        e.stopPropagation();
        setIsEditing(false);
      }
    },
    [draft, taskId, taskTitle, renameTaskById],
  );

  if (isEditing) {
    return (
      <Input
        data-testid="task-title-rename-input"
        aria-label="Task title"
        autoFocus
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onFocus={(e) => e.target.select()}
        onBlur={handleCancel}
        onKeyDown={handleKeyDown}
        className="h-7 w-72 max-w-full text-sm font-medium"
      />
    );
  }

  return (
    <Breadcrumb className="min-w-0 max-w-full">
      <BreadcrumbList className="min-w-0 max-w-full flex-nowrap text-sm">
        <BreadcrumbItem className="min-w-0 max-w-full">
          <Tooltip>
            <TooltipTrigger asChild>
              {/* BreadcrumbPage defaults to aria-disabled="true"; mark it enabled
                  when renameable so pointer-actionability checks (and AT) see the
                  double-click target as interactive. */}
              <BreadcrumbPage
                className="block max-w-full truncate font-medium"
                aria-disabled={!canRename}
                onDoubleClick={handleDoubleClick}
              >
                {taskTitle ?? "Task details"}
              </BreadcrumbPage>
            </TooltipTrigger>
            <TooltipContent>{taskTitle ?? "Task details"}</TooltipContent>
          </Tooltip>
        </BreadcrumbItem>
      </BreadcrumbList>
    </Breadcrumb>
  );
}
