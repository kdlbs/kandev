"use client";

import { useMemo } from "react";
import { useRouter } from "@/lib/routing/client-router";
import { useTheme } from "@/components/theme/app-theme";
import {
  IconHome,
  IconList,
  IconSettings,
  IconChartBar,
  IconSun,
  IconMoon,
  IconRobot,
  IconCpu,
  IconFolder,
  IconMessageCircle,
  IconSparkles,
  IconBrandGithub,
} from "@tabler/icons-react";
import { useRegisterCommands } from "@/hooks/use-register-commands";
import { useKeyboardShortcut } from "@/hooks/use-keyboard-shortcut";
import { useAppShortcuts } from "@/hooks/use-app-shortcuts";
import { useAppStore } from "@/components/state-provider";
import { useQuickChatLauncher } from "@/hooks/use-quick-chat-launcher";
import { getShortcut } from "@/lib/keyboard/shortcut-overrides";
import type { CommandItem } from "@/lib/commands/types";

type PushFn = ReturnType<typeof useRouter>["push"];

function buildNavigationCommands(push: PushFn): CommandItem[] {
  return [
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
      keywords: ["settings", "preferences", "config", "general settings"],
      action: () => push("/settings/general"),
    },
    {
      id: "nav-stats",
      label: "Go to Stats",
      group: "Navigation",
      icon: <IconChartBar className="size-3.5" />,
      keywords: ["stats", "statistics", "analytics", "metrics"],
      action: () => push("/stats"),
    },
    {
      id: "nav-github",
      label: "Go to GitHub Dashboard",
      group: "Navigation",
      icon: <IconBrandGithub className="size-3.5" />,
      keywords: ["github", "dashboard", "pr", "pull request", "code review", "issues", "review"],
      action: () => push("/github"),
    },
    {
      id: "settings-agents",
      label: "Agents Settings",
      group: "Settings",
      icon: <IconRobot className="size-3.5" />,
      keywords: ["agents", "agent settings", "agent profiles", "installed agents", "claude"],
      action: () => push("/settings/agents"),
    },
    {
      id: "settings-executors",
      label: "Executors Settings",
      group: "Settings",
      icon: <IconCpu className="size-3.5" />,
      keywords: [
        "executors",
        "executor profiles",
        "execution environment",
        "environment variables",
        "runtime",
        "compute",
      ],
      action: () => push("/settings/executors"),
    },
    {
      id: "settings-workspace",
      label: "Workspace Settings",
      group: "Settings",
      icon: <IconFolder className="size-3.5" />,
      keywords: ["workspace", "workspaces"],
      action: () => push("/settings/workspace"),
    },
    {
      id: "settings-prompts",
      label: "Prompts Settings",
      group: "Settings",
      icon: <IconMessageCircle className="size-3.5" />,
      keywords: [
        "prompts",
        "prompt settings",
        "custom prompts",
        "prompt snippets",
        "prompt templates",
      ],
      action: () => push("/settings/prompts"),
    },
  ];
}

function buildThemeCommand(
  resolvedTheme: string | undefined,
  setTheme: (theme: string) => void,
): CommandItem {
  const isDark = resolvedTheme === "dark";
  const destinationTheme = isDark ? "light" : "dark";
  return {
    id: "pref-theme",
    label: isDark ? "Switch to Light Mode" : "Switch to Dark Mode",
    group: "Preferences",
    icon: isDark ? <IconSun className="size-3.5" /> : <IconMoon className="size-3.5" />,
    keywords: ["theme", "color theme", "appearance"],
    action: () => setTheme(destinationTheme),
  };
}

export function GlobalCommands() {
  const router = useRouter();
  const { resolvedTheme, setTheme } = useTheme();
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);
  const handleOpenQuickChat = useQuickChatLauncher(activeWorkspaceId);
  const handleOpenConfigChat = useQuickChatLauncher(activeWorkspaceId, "config");

  const keyboardShortcuts = useAppStore((s) => s.userSettings.keyboardShortcuts);
  const quickChatShortcut = getShortcut("QUICK_CHAT", keyboardShortcuts);

  const quickChatCommand: CommandItem = useMemo(
    () => ({
      id: "quick-chat",
      label: "Quick Chat",
      group: "Actions",
      icon: <IconMessageCircle className="size-3.5" />,
      keywords: ["quick chat", "new quick chat", "quick question", "ask agent"],
      shortcut: quickChatShortcut,
      action: handleOpenQuickChat,
    }),
    [handleOpenQuickChat, quickChatShortcut],
  );

  const configChatCommand: CommandItem = useMemo(
    () => ({
      id: "config-chat",
      label: "Configuration Chat",
      group: "Actions",
      icon: <IconSparkles className="size-3.5" />,
      keywords: [
        "config chat",
        "config mode",
        "configure kandev",
        "workflow configuration",
        "mcp configuration",
      ],
      action: handleOpenConfigChat,
    }),
    [handleOpenConfigChat],
  );

  const commands = useMemo<CommandItem[]>(
    () => [
      ...buildNavigationCommands(router.push),
      buildThemeCommand(resolvedTheme, setTheme),
      quickChatCommand,
      configChatCommand,
    ],
    [router.push, resolvedTheme, setTheme, quickChatCommand, configChatCommand],
  );

  useRegisterCommands(commands);
  useKeyboardShortcut(quickChatShortcut, handleOpenQuickChat);
  useAppShortcuts();

  return null;
}
