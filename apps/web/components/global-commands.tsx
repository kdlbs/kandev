"use client";

import { useMemo } from "react";
import { useRouter } from "next/navigation";
import { useTheme } from "next-themes";
import {
  IconSearch,
  IconHome,
  IconList,
  IconSettings,
  IconChartBar,
  IconSun,
  IconMoon,
  IconRobot,
  IconCpu,
  IconServer,
  IconFolder,
  IconMessageCircle,
} from "@tabler/icons-react";
import { useRegisterCommands } from "@/hooks/use-register-commands";
import type { CommandItem } from "@/lib/commands/types";

type PushFn = ReturnType<typeof useRouter>["push"];

function buildNavigationCommands(push: PushFn): CommandItem[] {
  return [
    {
      id: "search-tasks",
      label: "Search Tasks",
      group: "Search",
      icon: <IconSearch className="size-3.5" />,
      keywords: ["find", "search", "task"],
      enterMode: "search-tasks",
      priority: 50,
    },
    {
      id: "nav-home",
      label: "Go to Home",
      group: "Navigation",
      icon: <IconHome className="size-3.5" />,
      keywords: ["home", "kanban", "board"],
      action: () => push("/"),
    },
    {
      id: "nav-tasks",
      label: "Go to All Tasks",
      group: "Navigation",
      icon: <IconList className="size-3.5" />,
      keywords: ["tasks", "list", "all"],
      action: () => push("/tasks"),
    },
    {
      id: "nav-settings",
      label: "Go to Settings",
      group: "Navigation",
      icon: <IconSettings className="size-3.5" />,
      keywords: ["settings", "preferences", "config"],
      action: () => push("/settings/general"),
    },
    {
      id: "nav-stats",
      label: "Go to Stats",
      group: "Navigation",
      icon: <IconChartBar className="size-3.5" />,
      keywords: ["stats", "analytics", "metrics"],
      action: () => push("/stats"),
    },
    {
      id: "settings-agents",
      label: "Agents Settings",
      group: "Settings",
      icon: <IconRobot className="size-3.5" />,
      keywords: ["agents", "ai", "claude"],
      action: () => push("/settings/agents"),
    },
    {
      id: "settings-executors",
      label: "Executors Settings",
      group: "Settings",
      icon: <IconCpu className="size-3.5" />,
      keywords: ["executors", "compute", "run"],
      action: () => push("/settings/executors"),
    },
    {
      id: "settings-environments",
      label: "Environments Settings",
      group: "Settings",
      icon: <IconServer className="size-3.5" />,
      keywords: ["environments", "env", "variables"],
      action: () => push("/settings/environments"),
    },
    {
      id: "settings-workspace",
      label: "Workspace Settings",
      group: "Settings",
      icon: <IconFolder className="size-3.5" />,
      keywords: ["workspace", "project"],
      action: () => push("/settings/workspace"),
    },
    {
      id: "settings-prompts",
      label: "Prompts Settings",
      group: "Settings",
      icon: <IconMessageCircle className="size-3.5" />,
      keywords: ["prompts", "templates", "message"],
      action: () => push("/settings/prompts"),
    },
  ];
}

function buildThemeCommand(
  resolvedTheme: string | undefined,
  setTheme: (theme: string) => void,
): CommandItem {
  const isDark = resolvedTheme === "dark";
  return {
    id: "pref-theme",
    label: isDark ? "Switch to Light Mode" : "Switch to Dark Mode",
    group: "Preferences",
    icon: isDark ? <IconSun className="size-3.5" /> : <IconMoon className="size-3.5" />,
    keywords: ["theme", "dark", "light", "mode"],
    action: () => setTheme(isDark ? "light" : "dark"),
  };
}

export function GlobalCommands() {
  const router = useRouter();
  const { resolvedTheme, setTheme } = useTheme();

  const commands = useMemo<CommandItem[]>(
    () => [...buildNavigationCommands(router.push), buildThemeCommand(resolvedTheme, setTheme)],
    [router.push, resolvedTheme, setTheme],
  );

  useRegisterCommands(commands);

  return null;
}
