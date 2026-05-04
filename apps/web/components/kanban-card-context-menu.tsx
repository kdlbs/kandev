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
}: {
  entries: KanbanCardMenuEntry[];
  children: React.ReactNode;
}) {
  const { isDesktop } = useResponsiveBreakpoint();

  if (!isDesktop) return children;

  return (
    <ContextMenu>
      <ContextMenuTrigger className="block">{children}</ContextMenuTrigger>
      <ContextMenuContent className="w-56">
        <KanbanCardContextMenuItems entries={entries} />
      </ContextMenuContent>
    </ContextMenu>
  );
}
