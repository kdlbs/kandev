"use client";

import { useTranslation } from "react-i18next";
import { useRouter, usePathname } from "@/lib/routing/client-router";
import {
  IconBuildings,
  IconLayoutKanban,
  IconSettings,
  IconSparkles,
  IconStethoscope,
  IconWifiOff,
} from "@tabler/icons-react";
import { useStaticDestinations } from "@/hooks/use-app-destinations";
import type { DestinationIcon } from "@/lib/navigation/types";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { ImproveKandevDialog } from "@/components/improve-kandev-dialog";
import { ReleaseNotesDialog } from "@/components/release-notes/release-notes-dialog";
import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useReleaseNotes } from "@/hooks/use-release-notes";
import { ThemeToggle } from "@/components/theme-toggle";
import { CurrentUserChip } from "./current-user-chip";
import { linkToTask } from "@/lib/links";
import { cn } from "@/lib/utils";
import {
  isOfficeWorkspace,
  rememberLastOfficeWorkspace,
  rememberLastKanbanWorkspace,
  resolveLastOfficeWorkspace,
  resolveLastKanbanWorkspace,
  workspaceHomeHref,
} from "./app-sidebar-workspace-navigation";
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

function InsightFooterButtons({
  destinations,
  collapsed,
  router,
}: {
  destinations: ReturnType<typeof useStaticDestinations>;
  collapsed: boolean;
  router: ReturnType<typeof useRouter>;
}) {
  return (
    <>
      {destinations.map((destination) => (
        <FooterIconButton
          key={destination.id}
          icon={destination.icon}
          label={destination.label}
          collapsed={collapsed}
          onClick={() => router.push(destination.href)}
          testId={`sidebar-${destination.id}-button`}
        />
      ))}
    </>
  );
}

export function AppSidebarFooter({ collapsed, onToggleSettingsMode }: AppSidebarFooterProps) {
  const { t } = useTranslation();
  const router = useRouter();
  const pathname = usePathname();
  const workspaces = useAppStore((s) => s.workspaces);
  const workspaceId = workspaces.activeId;
  const activeWorkspace = workspaces.items.find((workspace) => workspace.id === workspaceId);
  const activeIsOffice = isOfficeWorkspace(activeWorkspace);
  const targetWorkspace = activeIsOffice
    ? resolveLastKanbanWorkspace(workspaces.items)
    : resolveLastOfficeWorkspace(workspaces.items);
  const settingsMode = useAppStore((s) => s.appSidebar.settingsMode);
  const enterSettings = () => {
    // One click navigates to Settings (like the Stats/Office buttons).
    if (!settingsMode && !isSettingsRoute(pathname)) {
      router.push("/settings");
    }
    onToggleSettingsMode();
  };
  const officeEnabled = useFeature("office");
  const appStatusBarEnabled = useFeature("appStatusBar");
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
        onClick={enterSettings}
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
      {officeEnabled && (
        <FooterIconButton
          icon={activeIsOffice ? IconLayoutKanban : IconBuildings}
          label={activeIsOffice ? t("sidebar:kanban") : t("sidebar:office")}
          collapsed={collapsed}
          onClick={() => {
            if (!activeIsOffice) rememberLastKanbanWorkspace(activeWorkspace);
            if (activeIsOffice) rememberLastOfficeWorkspace(activeWorkspace);
            const href =
              !activeIsOffice && !targetWorkspace
                ? "/office/setup?mode=new"
                : workspaceHomeHref(targetWorkspace ?? undefined);
            router.push(href);
          }}
          testId={activeIsOffice ? "sidebar-kanban-button" : "sidebar-office-button"}
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
