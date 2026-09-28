"use client";

import type { RefObject } from "react";
import { Button } from "@kandev/ui/button";
import { IconMessageCircle, IconSearch, IconTerminal2 } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { QuickChatActivityIndicator } from "@/components/quick-chat/quick-chat-activity-indicator";
import { useQuickChatActivity } from "@/components/quick-chat/use-quick-chat-activity";
import { useQuickChatLauncher } from "@/hooks/use-quick-chat-launcher";
import { useQuickTerminalLauncher } from "@/hooks/use-quick-terminal-launcher";
import { MainTopBarPluginActions } from "./main-top-bar-plugin-actions";
import { useAppStore } from "@/components/state-provider";
import { StatusSurfaceMetrics } from "@/components/system-metrics/status-surface-metrics";
import { cn } from "@/lib/utils";
import type { TaskListingPage } from "@/lib/task-listing/view-navigation";

export function MobileListingMenuActions({
  workspaceId,
  workspaceLabel,
  currentPage,
  open,
  closeMenu,
  onToggleSearch,
  isSearchOpen,
  returnFocusRef,
  showWorkspaceActions = true,
  showQuickActions = true,
}: {
  showWorkspaceActions?: boolean;
  showQuickActions?: boolean;
  workspaceId?: string;
  workspaceLabel: string;
  currentPage: TaskListingPage;
  open: boolean;
  closeMenu: (restoreFocus?: boolean) => void;
  onToggleSearch?: () => void;
  isSearchOpen: boolean;
  returnFocusRef: RefObject<HTMLElement | null>;
}) {
  const { t } = useTranslation();
  const statusBarEnabled = useAppStore((state) => state.userSettings.appStatusBarEnabled);

  function launch(action: () => void, restoreFocus = false) {
    closeMenu(restoreFocus);
    requestAnimationFrame(action);
  }

  return (
    <div
      className={cn(
        showQuickActions ? "flex flex-col gap-3" : "contents",
        "[&>div:empty]:hidden [&_[data-slot=button]]:!min-h-11 [&_[data-slot=button]]:!min-w-11",
      )}
    >
      {onToggleSearch && (
        <Button
          variant={isSearchOpen ? "secondary" : "outline"}
          className="h-11 w-full cursor-pointer justify-start gap-3 px-3 text-sm"
          aria-pressed={isSearchOpen}
          data-testid="mobile-search-toggle"
          onClick={() => launch(onToggleSearch, isSearchOpen)}
        >
          <IconSearch className="h-4 w-4" />
          {t("kanban:searchTasks")}
        </Button>
      )}
      {showWorkspaceActions && showQuickActions && workspaceId && (
        <MobileQuickActions
          workspaceId={workspaceId}
          closeMenu={closeMenu}
          returnFocusRef={returnFocusRef}
        />
      )}
      {showWorkspaceActions && (
        <MainTopBarPluginActions
          workspaceId={workspaceId}
          workspaceLabel={workspaceLabel}
          currentPage={currentPage}
          presentation="mobile"
        />
      )}
      {showWorkspaceActions && !statusBarEnabled && (
        <StatusSurfaceMetrics
          presentation="mobile-drawer"
          density="compact"
          drawerOpen={open}
          iconSize="size-4"
        />
      )}
    </div>
  );
}

export function MobileQuickActions({
  workspaceId,
  closeMenu,
  returnFocusRef,
  inline = false,
}: {
  workspaceId: string;
  closeMenu: (restoreFocus?: boolean) => void;
  returnFocusRef: RefObject<HTMLElement | null>;
  inline?: boolean;
}) {
  const { t } = useTranslation();
  const { activity, label } = useQuickChatActivity(workspaceId);
  const openQuickChat = useQuickChatLauncher(workspaceId, "chat", { returnFocusRef });
  const openQuickTerminal = useQuickTerminalLauncher(workspaceId, { returnFocusRef });
  function launch(action: () => void) {
    closeMenu(false);
    requestAnimationFrame(action);
  }
  return (
    <div className={inline ? "grid grid-cols-2 gap-2" : "flex flex-col gap-3"}>
      <Button
        variant="outline"
        className="min-h-11 h-auto min-w-0 w-full cursor-pointer justify-start gap-2 whitespace-normal px-3 py-2 text-left text-sm"
        aria-label={label}
        data-testid="mobile-quick-chat-button"
        data-legacy-testid="threads-menu-quick-chat"
        onClick={() => launch(openQuickChat)}
      >
        <span className="relative flex">
          <IconMessageCircle className="h-4 w-4" />
          <QuickChatActivityIndicator activity={activity} />
        </span>
        {t("sidebar:quickChat")}
      </Button>
      <Button
        variant="outline"
        className="min-h-11 h-auto min-w-0 w-full cursor-pointer justify-start gap-2 whitespace-normal px-3 py-2 text-left text-sm"
        data-testid="mobile-quick-terminal-button"
        data-legacy-testid="threads-menu-quick-terminal"
        onClick={() => launch(openQuickTerminal)}
      >
        <IconTerminal2 className="h-4 w-4" />
        {t("sidebar:quickTerminal")}
      </Button>
    </div>
  );
}
