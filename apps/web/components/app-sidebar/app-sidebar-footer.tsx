"use client";

import { useTranslation } from "react-i18next";
import { useRouter, usePathname } from "@/lib/routing/client-router";
import {
  IconDots,
  IconSettings,
  IconSparkles,
  IconStethoscope,
  IconWifiOff,
} from "@tabler/icons-react";
import { useStaticDestinations } from "@/hooks/use-app-destinations";
import type { DestinationIcon } from "@/lib/navigation/types";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { ImproveKandevDialog } from "@/components/improve-kandev-dialog";
import { ReleaseNotesDialog } from "@/components/release-notes/release-notes-dialog";
import { useAppStore } from "@/components/state-provider";
import { useReleaseNotes } from "@/hooks/use-release-notes";
import { ThemeToggle } from "@/components/theme-toggle";
import { CurrentUserChip } from "./current-user-chip";
import { linkToTask } from "@/lib/links";
import { cn } from "@/lib/utils";
import { workspaceHomeHref } from "./app-sidebar-workspace-navigation";
import { isSettingsRoute } from "./app-sidebar-route";
import { useConnectionIssueCopy } from "../app-status-bar/connection-status-item";
import type { ConnectionIssueSeverity } from "@/lib/types/connection";

type AppSidebarFooterProps = {
  collapsed: boolean;
  onToggleSettingsMode: () => void;
  layoutManaged?: boolean;
};

type FooterIconButtonProps = {
  icon: DestinationIcon;
  label: string;
  collapsed: boolean;
  onClick?: () => void;
  testId?: string;
  /** Toggle state: rotates the icon a half-turn (spins back out when cleared). */
  active?: boolean;
  showLabel?: boolean;
};

function FooterIconButton({
  icon: Icon,
  label,
  collapsed,
  onClick,
  testId,
  active,
  showLabel = false,
}: FooterIconButtonProps) {
  const buttonProps = {
    variant: "ghost" as const,
    size: "icon" as const,
    className: cn(
      "h-7 cursor-pointer relative [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:min-w-11",
      showLabel ? "min-w-0 flex-1 justify-start gap-2 px-2" : "w-7",
    ),
  };

  const content = (
    <>
      <Icon
        className={cn(
          "h-3.5 w-3.5 transition-transform duration-300",
          active && "rotate-180 text-foreground",
        )}
      />
    </>
  );

  const trigger = (
    <Button
      type="button"
      onClick={onClick}
      {...buttonProps}
      aria-label={label}
      aria-pressed={active}
      data-testid={testId}
    >
      {content}
      {showLabel && <span className="truncate text-left">{label}</span>}
    </Button>
  );

  return (
    <Tooltip>
      <TooltipTrigger asChild>{trigger}</TooltipTrigger>
      <TooltipContent side={collapsed ? "right" : "top"}>{label}</TooltipContent>
    </Tooltip>
  );
}

function SidebarConnectionWarning({
  collapsed,
  severity,
}: {
  collapsed: boolean;
  severity: Exclude<ConnectionIssueSeverity, "none">;
}) {
  const details = useConnectionIssueCopy(severity);
  if (!details) return null;
  const { label, description } = details;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className="relative flex h-9 w-9 shrink-0 items-center justify-center rounded-md text-muted-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring"
          role="status"
          aria-label={description}
          tabIndex={0}
          data-testid="sidebar-connection-warning"
          data-connection-severity={severity}
        >
          <IconWifiOff className="size-4" aria-hidden="true" />
          <span
            className={cn(
              "absolute right-1 top-1 size-2 rounded-full ring-2 ring-background",
              details.dotClass,
            )}
            aria-hidden="true"
          />
          <span className="sr-only">{label}</span>
        </span>
      </TooltipTrigger>
      <TooltipContent side={collapsed ? "right" : "top"}>{description}</TooltipContent>
    </Tooltip>
  );
}

function SidebarConnectionFallback({
  collapsed,
  appStatusBarEnabled,
}: {
  collapsed: boolean;
  appStatusBarEnabled: boolean;
}) {
  const severity = useAppStore((state) => state.connection.issueSeverity);
  if (appStatusBarEnabled || severity === "none") return null;
  return <SidebarConnectionWarning collapsed={collapsed} severity={severity} />;
}

function SidebarFooterDialogs({
  improveOpen,
  onImproveOpenChange,
  workspaceId,
  onTaskCreated,
  releaseNotes,
}: {
  improveOpen: boolean;
  onImproveOpenChange: (open: boolean) => void;
  workspaceId: string | null;
  onTaskCreated: (task: { id: string }, meta?: { autoFocus?: boolean }) => void;
  releaseNotes: ReturnType<typeof useReleaseNotes>;
}) {
  return (
    <>
      <ImproveKandevDialog
        open={improveOpen}
        onOpenChange={onImproveOpenChange}
        workspaceId={workspaceId}
        onSuccess={onTaskCreated}
      />
      {releaseNotes.hasNotes && (
        <ReleaseNotesDialog
          open={releaseNotes.dialogOpen}
          onOpenChange={releaseNotes.closeDialog}
          entries={releaseNotes.unseenEntries}
          latestVersion={releaseNotes.latestVersion}
        />
      )}
    </>
  );
}

function SidebarFooterMenu({
  destinations,
  collapsed,
  releaseNotes,
  onImprove,
}: {
  destinations: ReturnType<typeof useStaticDestinations>;
  collapsed: boolean;
  releaseNotes: ReturnType<typeof useReleaseNotes>;
  onImprove: () => void;
}) {
  const { t } = useTranslation();
  const router = useRouter();
  const label = t("common:showMoreActions");

  return (
    <Tooltip>
      <DropdownMenu>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="relative cursor-pointer"
              aria-label={label}
              data-testid="sidebar-footer-more-button"
            >
              <IconDots className="size-4" aria-hidden="true" />
              {releaseNotes.showTopbarButton && releaseNotes.hasUnseen && (
                <span
                  className="absolute right-0.5 top-0.5 size-1.5 rounded-full bg-primary"
                  aria-hidden="true"
                />
              )}
            </Button>
          </DropdownMenuTrigger>
        </TooltipTrigger>
        <DropdownMenuContent
          side="top"
          align={collapsed ? "start" : "end"}
          className="w-60"
          data-testid="sidebar-footer-menu"
        >
          {destinations.map((destination) => (
            <DropdownMenuItem
              key={destination.id}
              className="cursor-pointer [@media(pointer:coarse)]:min-h-11"
              data-testid={`sidebar-${destination.id}-button`}
              onClick={() => router.push(destination.href)}
            >
              <destination.icon className="size-4 shrink-0" aria-hidden="true" />
              <span className="truncate">{destination.label}</span>
            </DropdownMenuItem>
          ))}
          {destinations.length > 0 && <DropdownMenuSeparator />}
          <DropdownMenuItem
            onClick={onImprove}
            className="cursor-pointer [@media(pointer:coarse)]:min-h-11"
            data-testid="sidebar-improve-kandev-button"
          >
            <IconStethoscope className="size-4" aria-hidden="true" />
            {t("sidebar:improveKandev")}
          </DropdownMenuItem>
          {releaseNotes.hasNotes && (
            <DropdownMenuItem
              onClick={releaseNotes.openDialog}
              className="cursor-pointer [@media(pointer:coarse)]:min-h-11"
              data-testid="sidebar-release-notes-button"
            >
              <IconSparkles className="size-4" aria-hidden="true" />
              {t("sidebar:whatsNew")}
              {releaseNotes.hasUnseen && (
                <span className="ml-auto size-1.5 rounded-full bg-primary" aria-hidden="true" />
              )}
            </DropdownMenuItem>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
      <TooltipContent side={collapsed ? "right" : "top"}>{label}</TooltipContent>
    </Tooltip>
  );
}

/**
 * The gear both navigates and swaps the sidebar's content, in both directions.
 *
 * Entering: without the navigation the main panel keeps showing whatever the
 * user was on (e.g. a task session), so the first click looks like a no-op and a
 * second click on a tree leaf is what actually reaches Settings. Match the
 * Stats/Office buttons — one click gets you there.
 *
 * Leaving: swapping the sidebar back while the main panel stayed on a settings
 * page left the two disagreeing — kanban navigation beside an open settings
 * page, with no route back to the tree except the gear that had just closed it.
 *
 * Either way the swap waits for the navigation to commit. A settings page with
 * unsaved edits blocks the push, and "Continue editing" cancels it: toggling
 * regardless left the URL in Settings with the sidebar already back on kanban
 * navigation — the same disagreement, reached from the other side.
 */
function useSettingsGearToggle(
  settingsMode: boolean,
  activeWorkspace: Parameters<typeof workspaceHomeHref>[0],
  onToggleSettingsMode: () => void,
) {
  const router = useRouter();
  const pathname = usePathname();
  const startupPage = useAppStore((s) => s.userSettings.startupPage);

  return () => {
    const onSettingsRoute = isSettingsRoute(pathname);
    if (!settingsMode && !onSettingsRoute) {
      router.push("/settings", { onNavigated: onToggleSettingsMode });
      return;
    }
    if (settingsMode && onSettingsRoute) {
      router.push(workspaceHomeHref(activeWorkspace, startupPage), {
        onNavigated: onToggleSettingsMode,
      });
      return;
    }
    onToggleSettingsMode();
  };
}

export function AppSidebarFooter({
  collapsed,
  onToggleSettingsMode,
  layoutManaged = false,
}: AppSidebarFooterProps) {
  const { t } = useTranslation();
  const router = useRouter();
  const workspaces = useAppStore((s) => s.workspaces);
  const workspaceId = workspaces.activeId;
  const activeWorkspace = workspaces.items.find((workspace) => workspace.id === workspaceId);
  const settingsMode = useAppStore((s) => s.appSidebar.settingsMode);
  const toggleSettings = useSettingsGearToggle(settingsMode, activeWorkspace, onToggleSettingsMode);
  const appStatusBarEnabled = useAppStore((s) => s.userSettings.appStatusBarEnabled);
  const allInsightDestinations = useStaticDestinations("sidebar", "insights");
  const statsDestination = allInsightDestinations.find((destination) => destination.id === "stats");
  const insightDestinations = allInsightDestinations.filter(
    (destination) =>
      destination.id !== "stats" && (!layoutManaged || destination.source !== "plugin"),
  );
  const releaseNotes = useReleaseNotes();
  const improveOpen = useAppStore((s) => s.appSidebar.improveDialogOpen);
  const setImproveOpen = useAppStore((s) => s.setImproveDialogOpen);
  const authMode = useAppStore((s) => s.auth.mode);
  const authUser = useAppStore((s) => s.auth.user);
  const showCurrentUser = authMode === "enabled" && authUser !== null;

  return (
    <div
      data-testid="sidebar-footer"
      className={cn(
        "flex items-center gap-1 border-t border-border shrink-0",
        collapsed ? "flex-col justify-center px-1 py-1.5" : "px-2 py-2",
      )}
    >
      {showCurrentUser && <CurrentUserChip collapsed={collapsed} />}
      <FooterIconButton
        showLabel={!collapsed}
        icon={IconSettings}
        label={settingsMode ? t("sidebar:closeSettings") : t("common:settings")}
        collapsed={collapsed}
        onClick={toggleSettings}
        active={settingsMode}
        testId="sidebar-settings-gear"
      />
      {statsDestination && (
        <FooterIconButton
          icon={statsDestination.icon}
          label={statsDestination.label}
          collapsed={collapsed}
          onClick={() => router.push(statsDestination.href)}
          testId="sidebar-stats-button"
        />
      )}
      <ThemeToggle className="size-7 [@media(pointer:coarse)]:size-11" />
      <SidebarFooterMenu
        destinations={insightDestinations}
        collapsed={collapsed}
        releaseNotes={releaseNotes}
        onImprove={() => setImproveOpen(true)}
      />
      <SidebarConnectionFallback collapsed={collapsed} appStatusBarEnabled={appStatusBarEnabled} />
      <SidebarFooterDialogs
        improveOpen={improveOpen}
        onImproveOpenChange={setImproveOpen}
        workspaceId={workspaceId ?? null}
        onTaskCreated={(task, meta) => {
          if (meta?.autoFocus !== false) router.push(linkToTask(task.id));
        }}
        releaseNotes={releaseNotes}
      />
    </div>
  );
}
