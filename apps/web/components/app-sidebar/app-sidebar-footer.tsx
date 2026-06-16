"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import {
  IconBuildings,
  IconChartBar,
  IconLayoutKanban,
  IconSettings,
  IconSparkles,
  IconStethoscope,
} from "@tabler/icons-react";
import type { Icon as TablerIcon } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { ImproveKandevDialog } from "@/components/improve-kandev-dialog";
import { ReleaseNotesDialog } from "@/components/release-notes/release-notes-dialog";
import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useReleaseNotes } from "@/hooks/use-release-notes";
import { ThemeToggle } from "@/components/theme-toggle";
import { linkToTask } from "@/lib/links";
import { cn } from "@/lib/utils";
import {
  isOfficeWorkspace,
  resolveFirstOfficeWorkspace,
  resolveLastKanbanWorkspace,
  workspaceHomeHref,
} from "./app-sidebar-workspace-navigation";

type AppSidebarFooterProps = {
  collapsed: boolean;
};

type FooterIconButtonProps = {
  icon: TablerIcon;
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

export function AppSidebarFooter({ collapsed }: AppSidebarFooterProps) {
  const router = useRouter();
  const workspaces = useAppStore((s) => s.workspaces);
  const workspaceId = workspaces.activeId;
  const activeWorkspace = workspaces.items.find((workspace) => workspace.id === workspaceId);
  const activeIsOffice = isOfficeWorkspace(activeWorkspace);
  const targetWorkspace = activeIsOffice
    ? resolveLastKanbanWorkspace(workspaces.items)
    : resolveFirstOfficeWorkspace(workspaces.items);
  const settingsMode = useAppStore((s) => s.appSidebar.settingsMode);
  const toggleSettingsMode = useAppStore((s) => s.toggleAppSidebarSettingsMode);
  const officeEnabled = useFeature("office");
  const releaseNotes = useReleaseNotes();
  const [improveOpen, setImproveOpen] = useState(false);

  return (
    <div
      className={cn(
        "flex items-center border-t border-border shrink-0",
        collapsed ? "flex-col gap-1 justify-center px-1 py-1.5" : "px-2 py-1.5 gap-1 flex-wrap",
      )}
    >
      <FooterIconButton
        icon={IconSettings}
        label={settingsMode ? "Close settings" : "Settings"}
        collapsed={collapsed}
        onClick={toggleSettingsMode}
        active={settingsMode}
        testId="sidebar-settings-gear"
      />
      <FooterIconButton
        icon={IconChartBar}
        label="Stats"
        collapsed={collapsed}
        onClick={() => router.push("/stats")}
        testId="sidebar-stats-button"
      />
      <FooterIconButton
        icon={IconStethoscope}
        label="Improve Kandev"
        collapsed={collapsed}
        onClick={() => setImproveOpen(true)}
        testId="sidebar-improve-kandev-button"
      />
      {releaseNotes.showTopbarButton && (
        <FooterIconButton
          icon={IconSparkles}
          label="What's new"
          collapsed={collapsed}
          onClick={releaseNotes.openDialog}
          badge={releaseNotes.hasUnseen}
          testId="sidebar-release-notes-button"
        />
      )}
      {officeEnabled && (
        <FooterIconButton
          icon={activeIsOffice ? IconLayoutKanban : IconBuildings}
          label={activeIsOffice ? "Kanban" : "Office"}
          collapsed={collapsed}
          onClick={() => router.push(workspaceHomeHref(targetWorkspace ?? undefined))}
          testId={activeIsOffice ? "sidebar-kanban-button" : "sidebar-office-button"}
        />
      )}
      <ThemeToggle />
      <ImproveKandevDialog
        open={improveOpen}
        onOpenChange={setImproveOpen}
        workspaceId={workspaceId ?? null}
        onSuccess={(task) => router.push(linkToTask(task.id))}
      />
      {releaseNotes.hasNotes && (
        <ReleaseNotesDialog
          open={releaseNotes.dialogOpen}
          onOpenChange={releaseNotes.closeDialog}
          entries={releaseNotes.unseenEntries}
          latestVersion={releaseNotes.latestVersion}
        />
      )}
    </div>
  );
}
