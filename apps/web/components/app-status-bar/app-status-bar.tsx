"use client";

import { useMemo } from "react";
import { cn } from "@kandev/ui/lib/utils";
import { useAppStatusItems, type AppStatusItem } from "./app-status-items";
import { useAppStatusBarDrag } from "./use-app-status-bar-drag";
import { useAppStatusBarOrder } from "./use-app-status-bar-order";

type AppStatusBarProps = {
  pathname: string;
  activeWorkspaceId: string | null;
  activeTaskId: string | null;
  activeSessionId: string | null;
  density: "full" | "compact";
};

/**
 * Interaction reference: Orca StatusBar at d9d939a33b5858495ffb33489a952f1ac9293610.
 * Kandev implementation is independent; third-party notice ships in Settings > Licenses.
 */
export function AppStatusBar({
  pathname,
  activeWorkspaceId,
  activeTaskId,
  activeSessionId,
  density,
}: AppStatusBarProps) {
  const context = useMemo(
    () => ({ pathname, activeWorkspaceId, activeTaskId, activeSessionId }),
    [pathname, activeWorkspaceId, activeTaskId, activeSessionId],
  );
  const activeItems = useAppStatusItems(context);
  const { projected, moveItem } = useAppStatusBarOrder(activeItems);
  const drag = useAppStatusBarDrag({ moveItem });

  return (
    <footer
      ref={drag.barRef}
      {...drag.barHandlers}
      className={cn(
        "relative flex h-6 shrink-0 select-none items-center gap-4 overflow-hidden bg-background px-3 text-xs font-medium leading-none text-foreground/80 antialiased [font-family:var(--font-geist-sans)] before:pointer-events-none before:absolute before:inset-x-0 before:top-0 before:h-px before:bg-border",
        drag.draggingId && "cursor-grabbing",
      )}
      data-testid="app-status-bar"
      aria-label="Application status"
    >
      <StatusItemGroup side="left" testId="app-status-bar-left-plugins">
        {projected.left.map((item) => (
          <BarStatusItem key={item.id} item={item} side="left" density={density} drag={drag} />
        ))}
      </StatusItemGroup>
      <div className="h-full min-w-0 flex-1" data-testid="app-status-bar-spacer" />
      <StatusItemGroup side="right" testId="app-status-bar-right-plugins">
        {projected.right.map((item) => (
          <BarStatusItem key={item.id} item={item} side="right" density={density} drag={drag} />
        ))}
      </StatusItemGroup>
    </footer>
  );
}

function StatusItemGroup({
  side,
  testId,
  children,
}: {
  side: "left" | "right";
  testId: string;
  children: React.ReactNode;
}) {
  return (
    <div
      className="flex h-full min-w-0 items-center gap-3 overflow-hidden"
      data-testid={testId}
      data-status-side={side}
    >
      {children}
    </div>
  );
}

function BarStatusItem({
  item,
  side,
  density,
  drag,
}: {
  item: AppStatusItem;
  side: "left" | "right";
  density: "full" | "compact";
  drag: ReturnType<typeof useAppStatusBarDrag>;
}) {
  return (
    <div
      className={cn(
        "inline-flex h-full min-w-0 shrink-0 items-center leading-none transition-opacity duration-150 empty:hidden",
        drag.draggingId === item.id && "opacity-45",
      )}
      data-status-item-id={item.id}
      data-status-side={side}
      onPointerDown={(event) => drag.onItemPointerDown(item.id, event)}
    >
      {item.render({ presentation: "bar", density, drawerOpen: false })}
    </div>
  );
}
