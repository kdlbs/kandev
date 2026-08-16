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
import { MAX_INLINE_PLUGIN_FOOTER_ITEMS } from "@/lib/navigation/plugin-footer-budget";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
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
};

type FooterIconButtonProps = {
  icon: DestinationIcon;
  label: string;
  collapsed: boolean;
  onClick?: () => void;
  badge?: boolean;
  testId?: string;
  /** Toggle state: rotates the icon a half-turn (spins back out when cleared). */
  active?: boolean;
};

function FooterIconButton({
  icon: Icon,
  label,
  collapsed,
  onClick,
  badge,
  testId,
  active,
}: FooterIconButtonProps) {
  const buttonProps = {
    variant: "ghost" as const,
    size: "icon" as const,
    className: "h-7 w-7 cursor-pointer relative",
  };

  const content = (
    <>
      <Icon
        className={cn(
          "h-3.5 w-3.5 transition-transform duration-300",
          active && "rotate-180 text-foreground",
        )}
      />
      {badge && (
        <span className="absolute -top-0.5 -right-0.5 h-2 w-2 rounded-full bg-primary border border-background" />
      )}
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
  onTaskCreated: (task: { id: string }) => void;
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

/**
 * Re-exported so existing unit-test imports from this module keep working —
 * the value itself lives in `plugin-footer-budget.ts` (see that module's
 * doc comment) so Playwright specs, which run outside the React tree, can
 * import it too without pulling in this file's JSX/React dependencies.
 */
export { MAX_INLINE_PLUGIN_FOOTER_ITEMS };

type InsightDestinations = ReturnType<typeof useStaticDestinations>;

/**
 * Splits the resolved `insights` destinations into the inline run and the
 * overflow run. First-party entries (today, only `stats`) are never counted
 * against the budget and always render inline; the budget applies to
 * `source === "plugin"` entries only, partitioning them in place without
 * reordering — concatenating `inline` and `overflow` reproduces the original
 * order.
 */
function partitionInsightDestinations(destinations: InsightDestinations): {
  inline: InsightDestinations;
  overflow: InsightDestinations;
} {
  const inline: InsightDestinations = [];
  const overflow: InsightDestinations = [];
  let pluginCount = 0;

  for (const destination of destinations) {
    if (destination.source !== "plugin") {
      inline.push(destination);
      continue;
    }
    if (pluginCount < MAX_INLINE_PLUGIN_FOOTER_ITEMS) {
      inline.push(destination);
    } else {
      overflow.push(destination);
    }
    pluginCount += 1;
  }

  return { inline, overflow };
}

/**
 * Overflow trigger for plugin `insights` destinations past the inline
 * budget. Reuses `FooterIconButton`'s icon-button treatment (size, tooltip,
 * hover) wrapped as a `@kandev/ui/dropdown-menu` trigger. Each menu item
 * carries the same `data-testid`/accessible-name derivation as the inline
 * button it would otherwise be — see spec.md#Rendered-identity.
 */
function InsightOverflowMenu({
  destinations,
  collapsed,
  router,
}: {
  destinations: InsightDestinations;
  collapsed: boolean;
  router: ReturnType<typeof useRouter>;
}) {
  const { t } = useTranslation();
  const label = t("sidebar:morePluginItems");

  return (
    <Tooltip>
      <DropdownMenu>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="h-7 w-7 cursor-pointer"
              aria-label={label}
              data-testid="sidebar-plugin-overflow-button"
            >
              <IconDots className="h-3.5 w-3.5" />
            </Button>
          </DropdownMenuTrigger>
        </TooltipTrigger>
        <DropdownMenuContent>
          {destinations.map((destination) => (
            <DropdownMenuItem
              key={destination.id}
              className="cursor-pointer"
              data-testid={`sidebar-${destination.id}-button`}
              onClick={() => router.push(destination.href)}
            >
              <destination.icon className="h-4 w-4 mr-2" />
              {destination.label}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
      <TooltipContent side={collapsed ? "right" : "top"}>{label}</TooltipContent>
    </Tooltip>
  );
}

function InsightFooterButtons({
  destinations,
  collapsed,
  router,
}: {
  destinations: InsightDestinations;
  collapsed: boolean;
  router: ReturnType<typeof useRouter>;
}) {
  const { inline, overflow } = partitionInsightDestinations(destinations);

  return (
    <>
      {inline.map((destination) => (
        <FooterIconButton
          key={destination.id}
          icon={destination.icon}
          label={destination.label}
          collapsed={collapsed}
          onClick={() => router.push(destination.href)}
          testId={`sidebar-${destination.id}-button`}
        />
      ))}
      {overflow.length > 0 && (
        <InsightOverflowMenu destinations={overflow} collapsed={collapsed} router={router} />
      )}
    </>
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

  return () => {
    const onSettingsRoute = isSettingsRoute(pathname);
    if (!settingsMode && !onSettingsRoute) {
      router.push("/settings", { onNavigated: onToggleSettingsMode });
      return;
    }
    if (settingsMode && onSettingsRoute) {
      router.push(workspaceHomeHref(activeWorkspace), { onNavigated: onToggleSettingsMode });
      return;
    }
    onToggleSettingsMode();
  };
}

export function AppSidebarFooter({ collapsed, onToggleSettingsMode }: AppSidebarFooterProps) {
  const { t } = useTranslation();
  const router = useRouter();
  const workspaces = useAppStore((s) => s.workspaces);
  const workspaceId = workspaces.activeId;
  const activeWorkspace = workspaces.items.find((workspace) => workspace.id === workspaceId);
  const settingsMode = useAppStore((s) => s.appSidebar.settingsMode);
  const toggleSettings = useSettingsGearToggle(settingsMode, activeWorkspace, onToggleSettingsMode);
  const appStatusBarEnabled = useAppStore((s) => s.userSettings.appStatusBarEnabled);
  const insightDestinations = useStaticDestinations("sidebar", "insights");
  const releaseNotes = useReleaseNotes();
  const improveOpen = useAppStore((s) => s.appSidebar.improveDialogOpen);
  const setImproveOpen = useAppStore((s) => s.setImproveDialogOpen);
  const authMode = useAppStore((s) => s.auth.mode);
  const authUser = useAppStore((s) => s.auth.user);
  const showCurrentUser = authMode === "enabled" && authUser !== null;

  return (
    <div
      className={cn(
        "flex items-center border-t border-border shrink-0",
        collapsed ? "flex-col gap-1 justify-center px-1 py-1.5" : "px-2 py-1.5 gap-1 flex-wrap",
      )}
    >
      <FooterIconButton
        icon={IconSettings}
        label={settingsMode ? t("sidebar:closeSettings") : t("common:settings")}
        collapsed={collapsed}
        onClick={toggleSettings}
        active={settingsMode}
        testId="sidebar-settings-gear"
      />
      <InsightFooterButtons
        destinations={insightDestinations}
        collapsed={collapsed}
        router={router}
      />
      <FooterIconButton
        icon={IconStethoscope}
        label={t("sidebar:improveKandev")}
        collapsed={collapsed}
        onClick={() => setImproveOpen(true)}
        testId="sidebar-improve-kandev-button"
      />
      {releaseNotes.showTopbarButton && (
        <FooterIconButton
          icon={IconSparkles}
          label={t("sidebar:whatsNew")}
          collapsed={collapsed}
          onClick={releaseNotes.openDialog}
          badge={releaseNotes.hasUnseen}
          testId="sidebar-release-notes-button"
        />
      )}
      <ThemeToggle />
      <SidebarConnectionFallback collapsed={collapsed} appStatusBarEnabled={appStatusBarEnabled} />
      {showCurrentUser && (
        <CurrentUserChip collapsed={collapsed} className={cn(!collapsed && "ml-auto")} />
      )}
      <SidebarFooterDialogs
        improveOpen={improveOpen}
        onImproveOpenChange={setImproveOpen}
        workspaceId={workspaceId ?? null}
        onTaskCreated={(task) => router.push(linkToTask(task.id))}
        releaseNotes={releaseNotes}
      />
    </div>
  );
}
