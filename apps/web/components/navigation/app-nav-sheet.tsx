"use client";

import { useArchivedTaskState } from "@/components/task/task-archived-context";
import { useOptionalPortForwardingVisibility } from "@/components/task/port-forwarding-visibility-provider";

import { useCallback, useEffect, useRef, useState, type MouseEvent, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { MobileWorkspaceActionsSection } from "@/components/app-sidebar/app-sidebar-workspace-actions";
import { AppSidebarWorkspacePicker } from "@/components/app-sidebar/app-sidebar-workspace-picker";
import {
  MobileListingMenuActions,
  MobileQuickActions,
} from "@/components/kanban/mobile-listing-menu-actions";
import { useAppStore } from "@/components/state-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useInOffice } from "@/hooks/use-in-office";
import { usePathname } from "@/lib/routing/client-router";
import { useHasSavedSidebarLayout } from "@/hooks/domains/sidebar/use-sidebar-layout-navigation";
import { AppNavSections, useAppNavDialogs } from "./app-nav-sections";
import { MobileAutomationsSection } from "./mobile-automations-section";
import { AppNavTrigger } from "./app-nav-trigger";
import { AppNavSurface } from "./app-nav-surface";

import { useMobileTaskNavigationOutlet } from "./mobile-task-navigation-provider";
import { useWorkbenchTaskSelection } from "@/components/task/mobile/task-sheet-selection-context";
import { useRouter } from "@/lib/routing/client-router";
import { linkToTask, replaceTaskUrl } from "@/lib/links";

type AppNavSheetProps = {
  pageNav?: ReactNode | ((close: () => void) => ReactNode);
  /** Page-scoped plugin controls grouped with plugin navigation on phones. */
  pluginActions?: ReactNode;
  omitDestinations?: string[];
  /** Reuse a workbench's existing task picker and selection controller. */
  onOpenTaskViews?: () => void;
};

/** Shared phone app navigation; wider page shells retain their side sheet. */
export function AppNavSheet(props: AppNavSheetProps) {
  const { pageNav, pluginActions, omitDestinations, onOpenTaskViews } = props;
  const { isMobile } = useResponsiveBreakpoint();
  const pathname = usePathname();
  const inOffice = useInOffice();
  const workspace = useActiveNavigationWorkspace();
  const [open, setOpen] = useState(false);
  const opener = useRef<HTMLButtonElement | null>(null);
  const restoreFocus = useRef(true);
  const close = useCallback(() => setOpen(false), []);
  const controls = useAppNavDialogs(close, onOpenTaskViews);
  const renderedPageNav = typeof pageNav === "function" ? pageNav(close) : pageNav;
  const currentPage = listingPageForPath(pathname);

  return (
    <>
      <AppNavSurface
        isMobile={isMobile}
        open={open}
        onOpenChange={(next) => {
          if (next) restoreFocus.current = true;
          setOpen(next);
        }}
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          if (restoreFocus.current && opener.current?.isConnected)
            opener.current.focus({ preventScroll: true });
          controls.onMenuCloseAutoFocus?.(event);
        }}
        trigger={<AppNavTrigger ref={opener} aria-expanded={open} />}
      >
        <nav
          className="flex min-h-0 flex-1 flex-col gap-6 overflow-y-auto overscroll-contain p-4 pb-[max(1rem,env(safe-area-inset-bottom,0px))]"
          onClick={(event) => closeMenuOnLinkClick(event, close)}
        >
          {isMobile && <NavigationWorkspacePicker close={close} />}
          {!isMobile && renderedPageNav}
          <AppNavSections
            onNavigate={close}
            phoneNavigation={isMobile}
            omitDestinations={navigationOmissions(isMobile, omitDestinations)}
            quickActions={
              <NavigationQuickActions
                isMobile={isMobile}
                workspaceId={workspace?.id}
                returnFocusRef={opener}
                closeMenu={(focus = false) => {
                  restoreFocus.current = focus;
                  close();
                }}
              />
            }
            afterPrimary={
              isMobile ? (
                <MobileNavigationExtras
                  localNav={renderedPageNav}
                  workspaceId={workspace?.id}
                  showTasks={open && !inOffice}
                  close={close}
                />
              ) : undefined
            }
            workspaceActions={
              <>
                {isMobile && (
                  <MobileListingMenuActions
                    showQuickActions={false}
                    workspaceId={workspace?.id}
                    workspaceLabel={workspace?.name ?? ""}
                    currentPage={currentPage}
                    open={open}
                    closeMenu={(focus = false) => {
                      restoreFocus.current = focus;
                      close();
                    }}
                    isSearchOpen={false}
                    returnFocusRef={opener}
                  />
                )}
                <MobileWorkspaceActionsSection />
                <NavigationAutomations
                  isMobile={isMobile}
                  open={open}
                  inOffice={inOffice}
                  workspaceId={workspace?.id}
                  close={close}
                />
              </>
            }
            pluginActions={pluginActions}
            controls={isMobile ? { ...controls, openTaskViews: undefined } : controls}
          />
        </nav>
      </AppNavSurface>
      {controls.dialogs}
    </>
  );
}

function useActiveNavigationWorkspace() {
  return useAppStore((state) =>
    state.workspaces.items.find((workspace) => workspace.id === state.workspaces.activeId),
  );
}

function closeMenuOnLinkClick(event: MouseEvent<HTMLElement>, close: () => void) {
  if (event.target instanceof Element && event.target.closest("a[href]")) close();
}

function NavigationWorkspacePicker({ close }: { close: () => void }) {
  const { t } = useTranslation();
  return (
    <section className="flex flex-col gap-2">
      <h3 className="text-sm font-medium">{t("common:workspace")}</h3>
      <AppSidebarWorkspacePicker
        modal={false}
        onActionComplete={close}
        triggerClassName="min-h-11 w-full flex-none"
        triggerTestId="mobile-workspace-trigger"
        chevronTestId="mobile-workspace-trigger-chevron"
        itemTestIdPrefix="mobile-workspace-item"
        contentClassName="w-80 max-w-[calc(100vw-2rem)]"
      />
    </section>
  );
}

function NavigationQuickActions({
  isMobile,
  workspaceId,
  ...props
}: {
  isMobile: boolean;
  workspaceId?: string;
} & Omit<Parameters<typeof MobileQuickActions>[0], "workspaceId">) {
  return isMobile && workspaceId ? (
    <MobileQuickActions {...props} workspaceId={workspaceId} inline />
  ) : null;
}

function MobileNavigationExtras({
  localNav,
  workspaceId,
  showTasks,
  close,
}: {
  localNav: ReactNode;
  workspaceId?: string;
  showTasks: boolean;
  close: () => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      {localNav && (
        <section className="flex flex-col gap-3" aria-label={t("common:pageNavigation")}>
          <h3 className="text-sm font-medium">{t("common:pageNavigation")}</h3>
          {localNav}
        </section>
      )}
      {showTasks && workspaceId && <MobileTaskOutlet key={workspaceId} close={close} />}
    </>
  );
}

function listingPageForPath(pathname: string) {
  if (pathname === "/tasks") return "tasks";
  if (pathname === "/threads") return "threads";
  return "kanban";
}

function MobileTaskOutlet({ close }: { close: () => void }) {
  const register = useMobileTaskNavigationOutlet();
  const archivedState = useArchivedTaskState();
  const portForwarding = useOptionalPortForwardingVisibility();
  const selection = useWorkbenchTaskSelection();
  const router = useRouter();
  const element = useRef<HTMLDivElement>(null);
  const navigate = useCallback(
    (id: string) => {
      if (selection) replaceTaskUrl(id);
      else router.push(linkToTask(id));
    },
    [router, selection],
  );
  useEffect(() => {
    if (element.current)
      register({
        element: element.current,
        close,
        navigate,
        selection,
        archivedState,
        portForwarding,
      });
    return () => register(null);
  }, [register, close, navigate, selection, archivedState, portForwarding]);
  return <section ref={element} data-testid="mobile-navigation-tasks" />;
}

function NavigationAutomations({
  isMobile,
  open,
  inOffice,
  workspaceId,
  close,
}: {
  isMobile: boolean;
  open: boolean;
  inOffice: boolean;
  workspaceId?: string;
  close: () => void;
}) {
  const hasSavedSidebarLayout = useHasSavedSidebarLayout();
  if (!isMobile || !open || inOffice || !workspaceId || hasSavedSidebarLayout) return null;
  return (
    <MobileAutomationsSection key={workspaceId} workspaceId={workspaceId} onNavigate={close} />
  );
}

function navigationOmissions(isMobile: boolean, omitted: string[] = []) {
  return isMobile ? [...omitted, "tasks", "threads"] : omitted;
}
