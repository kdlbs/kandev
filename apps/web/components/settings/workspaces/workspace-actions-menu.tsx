"use client";

import { useRef } from "react";
import { useTranslation } from "react-i18next";
import { IconCopy, IconDots } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { canCloneWorkspace } from "@/hooks/domains/workspace/use-workspace-clone";
import type { WorkspaceItem } from "@/lib/state/slices/workspace/selectors";

export function WorkspaceActionsMenu({
  workspace,
  onClone,
}: {
  workspace: WorkspaceItem;
  onClone: (workspace: WorkspaceItem) => void;
}) {
  const { t } = useTranslation();
  const trigger = useRef<HTMLButtonElement>(null);
  const cloneRequested = useRef(false);
  if (!canCloneWorkspace(workspace)) return null;

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          ref={trigger}
          type="button"
          variant="ghost"
          size="icon"
          className="relative z-10 shrink-0 text-muted-foreground"
          aria-label={t("workspaces:workspaceActionsNamed", { name: workspace.name })}
          data-testid="workspace-actions-menu"
        >
          <IconDots className="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="start"
        className="w-max min-w-44 max-w-[calc(100vw-2rem)]"
        onCloseAutoFocus={(event) => {
          if (!cloneRequested.current) return;
          cloneRequested.current = false;
          event.preventDefault();
          // The menu item unmounts; the persistent trigger owns form focus return.
          trigger.current?.focus();
          onClone(workspace);
        }}
      >
        <DropdownMenuItem
          onSelect={() => {
            cloneRequested.current = true;
          }}
        >
          <IconCopy className="size-4" />
          {t("workspaces:cloneWorkspace")}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
