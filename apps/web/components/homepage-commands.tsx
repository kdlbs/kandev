"use client";

import { useMemo } from "react";
import { useRouter } from "@/lib/routing/client-router";
import { IconLayoutKanban, IconGitBranch, IconList, IconPlus } from "@tabler/icons-react";
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
        label: "Create New Task",
        group: "Tasks",
        icon: <IconPlus className="size-3.5" />,
        shortcut: newTaskShortcut,
        keywords: ["new", "create", "task", "add"],
        action: onCreateTask,
        priority: 0,
      },
      {
        id: "view-kanban",
        label: "Switch to Kanban View",
        group: "View",
        icon: <IconLayoutKanban className="size-3.5" />,
        keywords: ["kanban", "board", "view"],
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
        label: "Switch to Pipeline View",
        group: "View",
        icon: <IconGitBranch className="size-3.5" />,
        keywords: ["pipeline", "graph", "view"],
        priority: 0,
        action: () => {
          router.push(taskOverviewHref);
          onViewModeChange("graph2");
        },
      });
    }

    items.push({
      id: "view-list",
      label: "Switch to List View",
      group: "View",
      icon: <IconList className="size-3.5" />,
      keywords: ["list", "table", "view"],
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
  ]);

  useRegisterCommands(commands);

  return null;
}
