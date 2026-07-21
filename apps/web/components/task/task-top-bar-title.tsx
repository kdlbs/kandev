"use client";

import { useCallback, useEffect, useRef, useState } from "react";
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
  const titleRef = useRef<HTMLSpanElement>(null);
  const restoreFocusRef = useRef(false);
  const canRename = Boolean(taskId) && !isArchived;

  // Return focus to the title after a keyboard-driven exit (Enter/Escape) so
  // keyboard users don't lose their place; blur exits keep focus where the
  // user clicked.
  useEffect(() => {
    if (!isEditing && restoreFocusRef.current) {
      restoreFocusRef.current = false;
      titleRef.current?.focus();
    }
  }, [isEditing]);

  const startEditing = useCallback(() => {
    if (!taskId || isArchived) return;
    setDraft(taskTitle ?? "");
    setIsEditing(true);
  }, [taskId, isArchived, taskTitle]);

  const handleTitleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLSpanElement>) => {
      if (e.key === "Enter") {
        e.preventDefault();
        startEditing();
      }
    },
    [startEditing],
  );

  const handleCancel = useCallback(() => setIsEditing(false), []);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>) => {
      // IME composition uses Enter to confirm a candidate; let the IME handle
      // it instead of committing the rename. keyCode 229 covers IMEs that
      // report the accepting Enter after isComposing has flipped false.
      if (e.nativeEvent.isComposing || e.nativeEvent.keyCode === 229) return;
      if (e.key === "Enter") {
        e.preventDefault();
        const trimmed = draft.trim();
        if (taskId && !isArchived && trimmed && trimmed !== taskTitle) {
          renameTaskById(taskId, trimmed).catch((err) =>
            console.error("Failed to rename task:", err),
          );
        }
        restoreFocusRef.current = true;
        setIsEditing(false);
      } else if (e.key === "Escape") {
        e.preventDefault();
        e.stopPropagation();
        restoreFocusRef.current = true;
        setIsEditing(false);
      }
    },
    [draft, taskId, isArchived, taskTitle, renameTaskById],
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
              {/* BreadcrumbPage defaults to aria-disabled="true"; when renameable,
                  mark it enabled and make it keyboard-operable (tab + Enter) so
                  AT and pointer-actionability checks see a working control. */}
              <BreadcrumbPage
                ref={titleRef}
                className="block max-w-full truncate rounded-sm font-medium outline-none focus-visible:ring-[2px] focus-visible:ring-ring/35"
                aria-disabled={!canRename}
                tabIndex={canRename ? 0 : undefined}
                onDoubleClick={startEditing}
                onKeyDown={canRename ? handleTitleKeyDown : undefined}
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
