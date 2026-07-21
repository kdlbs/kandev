"use client";

import { Drawer, DrawerContent, DrawerHeader, DrawerTitle } from "@kandev/ui/drawer";
import { StatusSurfaceMetrics } from "@/components/system-metrics/status-surface-metrics";
import { ConnectionStatusItem } from "./connection-status-item";
import { AppStatusBarPluginSlots } from "./app-status-bar-plugin-slots";

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
            <section className="space-y-1" aria-label="Connection status">
              <h3 className="px-1 text-sm font-medium">Connection</h3>
              <div className="flex min-h-11 items-center rounded-md px-3 hover:bg-muted/60">
                <ConnectionStatusItem />
              </div>
            </section>
            <div className="mt-5">
              <StatusSurfaceMetrics
                activeSessionId={activeSessionId}
                presentation="mobile-drawer"
                density="full"
                drawerOpen={open}
              />
            </div>
            <StatusDrawerPluginSection
              placement="left"
              pathname={pathname}
              activeWorkspaceId={activeWorkspaceId}
              activeTaskId={activeTaskId}
              activeSessionId={activeSessionId}
            />
            <StatusDrawerPluginSection
              placement="right"
              pathname={pathname}
              activeWorkspaceId={activeWorkspaceId}
              activeTaskId={activeTaskId}
              activeSessionId={activeSessionId}
            />
          </div>
        </div>
      </DrawerContent>
    </Drawer>
  );
}

function StatusDrawerPluginSection({
  placement,
  pathname,
  activeWorkspaceId,
  activeTaskId,
  activeSessionId,
}: {
  placement: "left" | "right";
  pathname: string;
  activeWorkspaceId: string | null;
  activeTaskId: string | null;
  activeSessionId: string | null;
}) {
  return (
    <section className="mt-5 space-y-1" aria-label={`Status plugins (${placement})`}>
      <AppStatusBarPluginSlots
        placement={placement}
        presentation="mobile-drawer"
        density="full"
        pathname={pathname}
        activeWorkspaceId={activeWorkspaceId}
        activeTaskId={activeTaskId}
        activeSessionId={activeSessionId}
      />
    </section>
  );
}
