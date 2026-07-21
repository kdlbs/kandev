"use client";

import {
  createContext,
  useContext,
  useMemo,
  useState,
  type ComponentProps,
  type ReactNode,
} from "react";
import { IconActivity } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { cn } from "@kandev/ui/lib/utils";
import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { usePathname } from "@/lib/routing/client-router";
import { AppStatusBar } from "./app-status-bar";
import { AppStatusDrawer } from "./app-status-drawer";

type AppStatusDrawerContextValue = {
  enabled: boolean;
  drawerOpen: boolean;
  openStatusDrawer: () => void;
  setStatusDrawerOpen: (open: boolean) => void;
};

const AppStatusDrawerContext = createContext<AppStatusDrawerContextValue | null>(null);
const unavailableDrawer: AppStatusDrawerContextValue = {
  enabled: false,
  drawerOpen: false,
  openStatusDrawer: () => {},
  setStatusDrawerOpen: () => {},
};

export function useAppStatusDrawer() {
  const context = useContext(AppStatusDrawerContext);
  return context ?? unavailableDrawer;
}

type AppStatusDrawerTriggerProps = Omit<ComponentProps<typeof Button>, "onClick"> & {
  label?: string;
};

export function AppStatusDrawerTrigger({
  className,
  children,
  label = "Open status",
  ...buttonProps
}: AppStatusDrawerTriggerProps) {
  const drawer = useContext(AppStatusDrawerContext);
  if (!drawer?.enabled) return null;
  return (
    <Button
      {...buttonProps}
      type="button"
      variant={buttonProps.variant ?? "ghost"}
      size={buttonProps.size ?? "icon"}
      className={cn("h-11 w-11 cursor-pointer md:hidden", className)}
      aria-label={label}
      onClick={drawer.openStatusDrawer}
      data-testid="app-status-drawer-trigger"
    >
      {children ?? <IconActivity className="h-4 w-4" />}
    </Button>
  );
}

export function AppStatusSurfaceProvider({ children }: { children: ReactNode }) {
  const [drawerOpen, setStatusDrawerOpen] = useState(false);
  const pathname = usePathname();
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const appStatusBarEnabled = useFeature("appStatusBar");
  const { isMobile, isFullDesktop } = useResponsiveBreakpoint();
  const drawer = useMemo<AppStatusDrawerContextValue>(
    () => ({
      enabled: appStatusBarEnabled,
      drawerOpen,
      openStatusDrawer: () => setStatusDrawerOpen(true),
      setStatusDrawerOpen,
    }),
    [appStatusBarEnabled, drawerOpen],
  );
  const surfaceProps = {
    pathname,
    activeWorkspaceId,
    activeTaskId,
    activeSessionId,
  };

  return (
    <AppStatusDrawerContext.Provider value={drawer}>
      <div className="flex h-dvh min-h-0 w-full flex-col overflow-hidden">
        {children}
        {appStatusBarEnabled &&
          (isMobile ? (
            <AppStatusDrawer
              {...surfaceProps}
              open={drawerOpen}
              onOpenChange={setStatusDrawerOpen}
            />
          ) : (
            <AppStatusBar {...surfaceProps} density={isFullDesktop ? "full" : "compact"} />
          ))}
      </div>
    </AppStatusDrawerContext.Provider>
  );
}
