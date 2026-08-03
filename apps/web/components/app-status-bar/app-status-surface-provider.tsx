"use client";

import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ComponentProps,
  type ReactNode,
} from "react";
import { IconActivity } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { cn } from "@kandev/ui/lib/utils";
import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { usePathname } from "@/lib/routing/client-router";
import { AppStatusBar } from "./app-status-bar";
import { AppStatusDrawer } from "./app-status-drawer";
import { connectionIssueDetails } from "./connection-status-item";
import type { ConnectionIssueSeverity } from "@/lib/types/connection";

type AppStatusDrawerContextValue = {
  enabled: boolean;
  issueSeverity: ConnectionIssueSeverity;
  connectionOnly: boolean;
  drawerOpen: boolean;
  openStatusDrawer: () => void;
  setStatusDrawerOpen: (open: boolean) => void;
};

const AppStatusDrawerContext = createContext<AppStatusDrawerContextValue | null>(null);
const unavailableDrawer: AppStatusDrawerContextValue = {
  enabled: false,
  issueSeverity: "none",
  connectionOnly: false,
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
  label,
  ...buttonProps
}: AppStatusDrawerTriggerProps) {
  const { t } = useTranslation();
  const drawer = useContext(AppStatusDrawerContext);
  if (!drawer?.enabled) return null;
  const issueDetails =
    drawer.issueSeverity === "none" ? null : connectionIssueDetails(drawer.issueSeverity);
  const issueActive = issueDetails !== null;
  // The default lives in the body, not the destructuring pattern: parameter
  // defaults evaluate before the component body, so `t` is not in scope there.
  const accessibleLabel = issueDetails
    ? t(issueDetails.descriptionKey)
    : (label ?? t("sidebar:openStatus"));
  return (
    <Button
      {...buttonProps}
      type="button"
      variant={buttonProps.variant ?? "ghost"}
      size={buttonProps.size ?? "icon"}
      className={cn(
        "relative h-11 w-11 cursor-pointer",
        drawerTriggerVisibilityClass(drawer.connectionOnly),
        issueActive && (drawer.issueSeverity === "lost" ? "text-destructive" : "text-amber-500"),
        className,
      )}
      aria-label={accessibleLabel}
      onClick={drawer.openStatusDrawer}
      data-testid="app-status-drawer-trigger"
      data-connection-severity={issueActive ? drawer.issueSeverity : undefined}
    >
      {children ?? <IconActivity className="h-4 w-4" />}
      {issueActive && (
        <span
          className={cn(
            "absolute right-2 top-2 size-2 rounded-full ring-2 ring-background",
            drawer.issueSeverity === "lost" ? "bg-destructive" : "bg-amber-500",
          )}
          aria-hidden="true"
        />
      )}
    </Button>
  );
}

function drawerTriggerVisibilityClass(connectionOnly: boolean) {
  return connectionOnly ? "lg:hidden" : "sm:hidden";
}

export function AppStatusSurfaceProvider({ children }: { children: ReactNode }) {
  const [drawerOpen, setStatusDrawerOpen] = useState(false);
  const pathname = usePathname();
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const issueSeverity = useAppStore((state) => state.connection.issueSeverity);
  const appStatusBarEnabled = useFeature("appStatusBar");
  const { isMobile, isTablet, isFullDesktop } = useResponsiveBreakpoint();
  const connectionOnly = !appStatusBarEnabled && issueSeverity !== "none";
  const useDrawerSurface = isMobile || (isTablet && connectionOnly);
  const drawerEnabled = useDrawerSurface && (appStatusBarEnabled || connectionOnly);

  useEffect(() => {
    if (!drawerEnabled) setStatusDrawerOpen(false);
  }, [drawerEnabled]);

  const drawer = useMemo<AppStatusDrawerContextValue>(
    () => ({
      enabled: drawerEnabled,
      issueSeverity,
      connectionOnly,
      drawerOpen,
      openStatusDrawer: () => {
        if (drawerEnabled) setStatusDrawerOpen(true);
      },
      setStatusDrawerOpen,
    }),
    [connectionOnly, drawerEnabled, drawerOpen, issueSeverity],
  );
  const surfaceProps = {
    pathname,
    activeWorkspaceId,
    activeTaskId,
    activeSessionId,
  };

  return (
    <AppStatusDrawerContext.Provider value={drawer}>
      <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
        {children}
        {(useDrawerSurface ? drawerEnabled : appStatusBarEnabled) &&
          (useDrawerSurface ? (
            <AppStatusDrawer
              {...surfaceProps}
              open={drawerOpen}
              onOpenChange={setStatusDrawerOpen}
              connectionOnly={connectionOnly}
            />
          ) : (
            <AppStatusBar {...surfaceProps} density={isFullDesktop ? "full" : "compact"} />
          ))}
      </div>
    </AppStatusDrawerContext.Provider>
  );
}
