"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { IconActivity } from "@tabler/icons-react";
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle } from "@kandev/ui/drawer";
import { cn } from "@kandev/ui/lib/utils";
import { useAppStatusItems, type AppStatusItem } from "./app-status-items";
import { useAppStatusBarOrder } from "./use-app-status-bar-order";

type AppStatusDrawerProps = {
  pathname: string;
  activeWorkspaceId: string | null;
  activeTaskId: string | null;
  activeSessionId: string | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  connectionOnly?: boolean;
};

export function AppStatusDrawer({
  pathname,
  activeWorkspaceId,
  activeTaskId,
  activeSessionId,
  open,
  onOpenChange,
  connectionOnly = false,
}: AppStatusDrawerProps) {
  const { t } = useTranslation();
  const context = useMemo(
    () => ({ pathname, activeWorkspaceId, activeTaskId, activeSessionId }),
    [pathname, activeWorkspaceId, activeTaskId, activeSessionId],
  );
  const activeItems = useAppStatusItems(context, { connectionOnly });
  const { projected } = useAppStatusBarOrder(activeItems);
  const orderedItems = [...projected.left, ...projected.right];
  let drawerHeight = "h-[min(32rem,calc(100dvh-16px-env(safe-area-inset-bottom,0px)))]";
  if (orderedItems.length <= 1) {
    drawerHeight = "h-[min(15rem,calc(100dvh-16px-env(safe-area-inset-bottom,0px)))]";
  } else if (orderedItems.length === 2) {
    drawerHeight = "h-[min(20rem,calc(100dvh-16px-env(safe-area-inset-bottom,0px)))]";
  }

  return (
    <Drawer open={open} onOpenChange={onOpenChange}>
      <DrawerContent
        className={cn(
          drawerHeight,
          "max-h-[calc(100dvh-16px-env(safe-area-inset-bottom,0px))] overflow-hidden pb-[max(0.75rem,env(safe-area-inset-bottom))] before:rounded-[20px]",
        )}
      >
        <div
          data-testid="app-status-drawer"
          className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl bg-card text-card-foreground antialiased [font-family:var(--font-geist-sans)] shadow-[0_1px_2px_rgb(0_0_0_/_0.04),0_0_0_1px_rgb(0_0_0_/_0.04)] dark:shadow-[0_0_0_1px_rgb(255_255_255_/_0.08)]"
        >
          <DrawerHeader
            data-testid="app-status-drawer-summary"
            className="shrink-0 border-b border-border/80 px-5 pb-4 pt-3 text-left"
          >
            <div className="flex items-center gap-3">
              <div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary shadow-[0_1px_2px_rgb(0_0_0_/_0.04)]">
                <IconActivity className="size-4" stroke={1.8} aria-hidden="true" />
              </div>
              <div className="min-w-0">
                <p className="text-[10px] font-semibold uppercase tracking-[0.14em] text-muted-foreground">
                  {t("common:workspace")}
                </p>
                <DrawerTitle className="text-base font-semibold tracking-[-0.015em] text-balance">
                  {t("common:status")}
                </DrawerTitle>
              </div>
            </div>
          </DrawerHeader>
          <div
            data-testid="app-status-drawer-scroll-region"
            className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-3 py-3"
          >
            <section className="space-y-2" aria-label={t("sidebar:applicationStatusItems")}>
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
      className="flex min-h-11 w-full min-w-0 items-center rounded-lg bg-muted/50 px-3 py-2 shadow-[0_1px_2px_rgb(0_0_0_/_0.03),0_0_0_1px_rgb(0_0_0_/_0.04)] transition-[background-color,box-shadow] duration-150 ease-out hover:bg-muted/80 focus-within:bg-muted/80 focus-within:shadow-[0_1px_2px_rgb(0_0_0_/_0.06),0_0_0_1px_rgb(0_0_0_/_0.06)] empty:hidden dark:shadow-[0_0_0_1px_rgb(255_255_255_/_0.06)]"
      data-status-item-id={item.id}
    >
      {item.render({ presentation: "mobile-drawer", density: "full", drawerOpen: open })}
    </div>
  );
}
