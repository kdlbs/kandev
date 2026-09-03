"use client";

import type { RefObject } from "react";
import { IconDots } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from "@kandev/ui/dropdown-menu";
import {
  KanbanCardDropdownMenuItems,
  type KanbanCardMenuEntry,
} from "@/components/kanban-card-menu-items";

type TaskActionsMenuTriggerProps = {
  entries: KanbanCardMenuEntry[];
  testId: string;
  triggerRef: RefObject<HTMLButtonElement | null>;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

/** The "More options" dots trigger shared by the preview and detail surfaces. */
export function TaskActionsMenuTrigger({
  entries,
  testId,
  triggerRef,
  open,
  onOpenChange,
}: TaskActionsMenuTriggerProps) {
  const { t } = useTranslation();
  return (
    <DropdownMenu open={open} onOpenChange={onOpenChange}>
      <DropdownMenuTrigger asChild>
        <button
          ref={triggerRef}
          type="button"
          data-testid={testId}
          className="text-muted-foreground hover:text-foreground hover:bg-muted rounded-sm p-1 -m-1 transition-colors cursor-pointer"
          onClick={(e) => e.stopPropagation()}
          onPointerDown={(e) => e.stopPropagation()}
          aria-label={t("kanban:moreOptions")}
        >
          <IconDots className="h-4 w-4" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="end"
        className="w-56"
        onEscapeKeyDown={(event) => {
          // Consume this Escape ourselves instead of letting Radix's default
          // dismiss handle it: closing the menu via `onOpenChange` still
          // triggers Radix's synchronous focus-restoration flush, but calling
          // `preventDefault()` here (during document capture, before this
          // same event reaches any bubble-phase `window` listener a host
          // surface owns) lets that listener check `event.defaultPrevented`
          // and know this keypress was already handled by the menu, rather
          // than racing a state update that may not have re-rendered yet
          // (AC-TASKS-TASK-ACTIONS-MENU-001.11).
          event.preventDefault();
          onOpenChange(false);
        }}
      >
        <KanbanCardDropdownMenuItems entries={entries} />
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
