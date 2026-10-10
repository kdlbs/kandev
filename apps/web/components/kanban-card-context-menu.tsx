"use client";

import { ContextMenu, ContextMenuContent, ContextMenuTrigger } from "@kandev/ui/context-menu";
import {
  KanbanCardContextMenuItems,
  type KanbanCardMenuEntry,
} from "@/components/kanban-card-menu-items";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

export function KanbanCardContextMenu({
  entries,
  children,
  onOpenChange,
}: {
  entries: KanbanCardMenuEntry[];
  children: React.ReactNode;
  onOpenChange?: (open: boolean) => void;
}) {
  const { isDesktop } = useResponsiveBreakpoint();

  if (!isDesktop) return children;

  return (
    <ContextMenu onOpenChange={onOpenChange}>
      <ContextMenuTrigger className="block">{children}</ContextMenuTrigger>
      <ContextMenuContent className="w-56">
        <KanbanCardContextMenuItems entries={entries} />
      </ContextMenuContent>
    </ContextMenu>
  );
}
