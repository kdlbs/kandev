"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useRouter } from "@/lib/routing/client-router";
import { IconLayoutKanban, IconGitBranch, IconList, IconPlus } from "@tabler/icons-react";
import { searchKeywords } from "@/lib/commands/search-keywords";
import { useRegisterCommands } from "@/hooks/use-register-commands";
import { useKanbanDisplaySettings } from "@/hooks/use-kanban-display-settings";
import { linkToTaskOverview, linkToTasks } from "@/lib/links";
import type { CommandItem } from "@/lib/commands/types";
import { useAppStore } from "@/components/state-provider";
import { getShortcut } from "@/lib/keyboard/shortcut-overrides";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

type HomepageCommandsProps = {
  onCreateTask: () => void;
};

export function HomepageCommands({ onCreateTask }: HomepageCommandsProps) {
  const { t } = useTranslation();
  const router = useRouter();
  const { onViewModeChange } = useKanbanDisplaySettings();
  const { isMobile } = useResponsiveBreakpoint();
  const keyboardShortcuts = useAppStore((s) => s.userSettings.keyboardShortcuts);
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);
  const activeWorkflowId = useAppStore((s) => s.workflows.activeId);
  const newTaskShortcut = getShortcut("NEW_TASK", keyboardShortcuts);
  const taskOverviewHref = linkToTaskOverview({
    workspaceId: activeWorkspaceId ?? undefined,
    workflowId: activeWorkflowId ?? undefined,
  });
  const tasksHref = linkToTasks(activeWorkspaceId ?? undefined);

  const commands = useMemo<CommandItem[]>(() => {
    const items: CommandItem[] = [
      {
        id: "task-create",
        label: t("common:commandCreateNewTask"),
        group: t("common:commandGroupTasks"),
        icon: <IconPlus className="size-3.5" />,
        shortcut: newTaskShortcut,
        keywords: searchKeywords(t, "common:commandCreateNewTaskKeywords"),
        action: onCreateTask,
        priority: 0,
      },
      {
        id: "view-kanban",
        label: t("common:commandSwitchToKanbanView"),
        group: t("common:commandGroupView"),
        icon: <IconLayoutKanban className="size-3.5" />,
        keywords: searchKeywords(t, "common:commandSwitchToKanbanViewKeywords"),
        priority: 0,
        action: () => {
          router.push(taskOverviewHref);
          if (!isMobile) onViewModeChange("");
        },
      },
    ];

    if (!isMobile) {
      items.push({
        id: "view-pipeline",
        label: t("common:commandSwitchToPipelineView"),
        group: t("common:commandGroupView"),
        icon: <IconGitBranch className="size-3.5" />,
        keywords: searchKeywords(t, "common:commandSwitchToPipelineViewKeywords"),
        priority: 0,
        action: () => {
          router.push(taskOverviewHref);
          onViewModeChange("graph2");
        },
      });
    }

    items.push({
      id: "view-list",
      label: t("common:commandSwitchToListView"),
      group: t("common:commandGroupView"),
      icon: <IconList className="size-3.5" />,
      keywords: searchKeywords(t, "common:commandSwitchToListViewKeywords"),
      priority: 0,
      action: () => router.push(tasksHref),
    });

    return items;
  }, [
    onCreateTask,
    router,
    onViewModeChange,
    newTaskShortcut,
    isMobile,
    taskOverviewHref,
    tasksHref,
    t,
  ]);

  useRegisterCommands(commands);

  return null;
}
