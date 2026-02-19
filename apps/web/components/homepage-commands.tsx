"use client";

import { useMemo } from "react";
import { useRouter } from "next/navigation";
import { IconLayoutKanban, IconGitBranch, IconList, IconPlus } from "@tabler/icons-react";
import { useRegisterCommands } from "@/hooks/use-register-commands";
import { useKanbanDisplaySettings } from "@/hooks/use-kanban-display-settings";
import { SHORTCUTS } from "@/lib/keyboard/constants";
import { linkToTasks } from "@/lib/links";
import type { CommandItem } from "@/lib/commands/types";

type HomepageCommandsProps = {
  onCreateTask: () => void;
};

export function HomepageCommands({ onCreateTask }: HomepageCommandsProps) {
  const router = useRouter();
  const { onViewModeChange } = useKanbanDisplaySettings();

  const commands = useMemo<CommandItem[]>(
    () => [
      {
        id: "task-create",
        label: "Create New Task",
        group: "Tasks",
        icon: <IconPlus className="size-3.5" />,
        shortcut: SHORTCUTS.NEW_TASK,
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
          router.push("/");
          onViewModeChange("");
        },
      },
      {
        id: "view-pipeline",
        label: "Switch to Pipeline View",
        group: "View",
        icon: <IconGitBranch className="size-3.5" />,
        keywords: ["pipeline", "graph", "view"],
        priority: 0,
        action: () => {
          router.push("/");
          onViewModeChange("graph2");
        },
      },
      {
        id: "view-list",
        label: "Switch to List View",
        group: "View",
        icon: <IconList className="size-3.5" />,
        keywords: ["list", "table", "view"],
        priority: 0,
        action: () => router.push(linkToTasks()),
      },
    ],
    [onCreateTask, router, onViewModeChange],
  );

  useRegisterCommands(commands);

  return null;
}
