"use client";

import { useRef, type ReactNode } from "react";
import { IconChevronDown, IconFolderOpen, IconLoader2 } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { useTaskFolderAction } from "@/hooks/use-task-folder-action";
import { TaskFolderPicker } from "./task-folder-picker";

export function EditorActionsDropdown({
  sessionId,
  children,
}: {
  sessionId: string | null;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  const action = useTaskFolderAction(sessionId);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const selectedSessionRef = useRef<string | null>(null);

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            ref={triggerRef}
            size="sm"
            variant="outline"
            className="rounded-none border-0 border-l px-2 cursor-pointer focus-visible:ring-inset"
            data-testid="editors-menu-list"
            aria-label={t("task:openEditor")}
            disabled={!sessionId}
          >
            <IconChevronDown className="h-4 w-4" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent
          align="end"
          onCloseAutoFocus={(event) => {
            const selectedSession = selectedSessionRef.current;
            selectedSessionRef.current = null;
            if (!selectedSession || selectedSession !== sessionId) return;
            event.preventDefault();
            triggerRef.current?.focus();
            action.open(triggerRef.current ?? undefined);
          }}
        >
          {children}
          <DropdownMenuSeparator />
          <DropdownMenuItem
            className="cursor-pointer"
            disabled={action.disabled}
            onSelect={() => {
              selectedSessionRef.current = sessionId;
            }}
          >
            {action.isLoading ? (
              <IconLoader2 className="size-4 animate-spin" aria-hidden />
            ) : (
              <IconFolderOpen className="size-4" aria-hidden />
            )}
            {t("editors:openFolder")}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <span role="status" className="sr-only">
        {action.isLoading ? t("editors:openingFolder") : ""}
      </span>
      <TaskFolderPicker action={action} />
    </>
  );
}
