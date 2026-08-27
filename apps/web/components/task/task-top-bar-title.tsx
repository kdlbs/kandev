"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { Input } from "@kandev/ui/input";
import { useTaskActions } from "@/hooks/use-task-actions";
import { useTaskTitleSelectionRestore } from "@/hooks/use-task-title-selection-restore";

type TaskTopBarTitleProps = {
  taskId?: string | null;
  taskTitle?: string;
  isArchived?: boolean;
};

export function TaskTopBarTitle({ taskId, taskTitle, isArchived }: TaskTopBarTitleProps) {
  const { renameTaskById } = useTaskActions();
  const { t } = useTranslation();
  const [isEditing, setIsEditing] = useState(false);
  const [draft, setDraft] = useState("");
  const { inputRef, clampChange } = useTaskTitleSelectionRestore(draft);
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
      // Both keys, because the idle title announces itself as a button and a
      // button that ignores Space is a promise the control does not keep.
      // Space would also scroll the page, hence the preventDefault on it.
      if (e.key === "Enter" || e.key === " ") {
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
        ref={inputRef}
        data-testid="task-title-rename-input"
        aria-label={t("task:taskTitle")}
        autoFocus
        value={draft}
        onChange={(e) => setDraft(clampChange(e))}
        onFocus={(e) => e.target.select()}
        onBlur={handleCancel}
        onKeyDown={handleKeyDown}
        className="h-7 w-72 max-w-full text-sm font-medium"
      />
    );
  }

  // The surrounding PageTopbar title crumb owns the breadcrumb semantics
  // (aria-current, position in the trail); this renders only the interactive
  // rename control. When renameable it is a real control — focusable, named by
  // its own text, and announced as a button, which is what it behaves like:
  // activating it opens the inline editor. Without a role it would be a
  // focusable span, reaching AT as unlabelled plain text. When it is not
  // renameable it is exactly that, plain text, so it takes no role and leaves
  // the tab order.
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          ref={titleRef}
          data-testid="task-topbar-title"
          className="block min-w-0 max-w-full truncate text-sm rounded-sm font-medium outline-none focus-visible:ring-[2px] focus-visible:ring-ring/35"
          role={canRename ? "button" : undefined}
          aria-disabled={!canRename}
          tabIndex={canRename ? 0 : undefined}
          onDoubleClick={startEditing}
          onKeyDown={canRename ? handleTitleKeyDown : undefined}
        >
          {taskTitle ?? t("task:taskDetails")}
        </span>
      </TooltipTrigger>
      <TooltipContent className="max-w-sm whitespace-normal break-words">
        <span className="block">{taskTitle ?? t("task:taskDetails")}</span>
        {canRename && <span className="mt-1 block">{t("task:doubleClickToEditOrPress")}</span>}
      </TooltipContent>
    </Tooltip>
  );
}
