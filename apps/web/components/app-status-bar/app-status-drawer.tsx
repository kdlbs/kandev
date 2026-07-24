"use client";

import { useMemo } from "react";
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle } from "@kandev/ui/drawer";
import { useAppStatusItems, type AppStatusItem } from "./app-status-items";
import { useAppStatusBarOrder } from "./use-app-status-bar-order";

type AppStatusDrawerProps = {
  pathname: string;
  activeWorkspaceId: string | null;
  activeTaskId: string | null;
  activeSessionId: string | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function AppStatusDrawer({
  pathname,
  activeWorkspaceId,
  activeTaskId,
  activeSessionId,
  open,
  onOpenChange,
}: AppStatusDrawerProps) {
  const context = useMemo(
    () => ({ pathname, activeWorkspaceId, activeTaskId, activeSessionId }),
    [pathname, activeWorkspaceId, activeTaskId, activeSessionId],
  );
  const activeItems = useAppStatusItems(context);
  const { projected } = useAppStatusBarOrder(activeItems);
  const orderedItems = [...projected.left, ...projected.right];

  return (
    <Drawer open={open} onOpenChange={onOpenChange}>
      <DrawerContent className="h-[min(32rem,calc(100dvh-16px))] max-h-[calc(100dvh-16px)] overflow-hidden pb-[max(0.5rem,env(safe-area-inset-bottom))]">
        <div
          data-testid="app-status-drawer"
          className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl bg-background"
        >
          <DrawerHeader className="shrink-0 border-b border-border/70 pb-3 text-left">
            <DrawerTitle>Status</DrawerTitle>
          </DrawerHeader>
          <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-4 py-3">
            <section className="space-y-1" aria-label="Application status items">
              {orderedItems.map((item) => (
                <DrawerStatusItem key={item.id} item={item} open={open} />
              ))}
            </section>
          </div>
        </div>
      </DrawerContent>
    </Drawer>
  );
}

function DrawerStatusItem({ item, open }: { item: AppStatusItem; open: boolean }) {
  return (
    <div
      className="flex min-h-11 w-full min-w-0 items-center rounded-md px-3 hover:bg-muted/60 empty:hidden"
      data-status-item-id={item.id}
    >
      {item.render({ presentation: "mobile-drawer", density: "full", drawerOpen: open })}
    </div>
  );
}
