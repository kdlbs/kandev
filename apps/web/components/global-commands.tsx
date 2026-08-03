"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
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
import { usePluginShortcuts } from "@/hooks/use-plugin-shortcuts";
import { useAppStore } from "@/components/state-provider";
import { useQuickChatLauncher } from "@/hooks/use-quick-chat-launcher";
import { getShortcut } from "@/lib/keyboard/shortcut-overrides";
import { linkToTaskOverview } from "@/lib/links";
import type { CommandItem } from "@/lib/commands/types";

type PushFn = ReturnType<typeof useRouter>["push"];

// Catalog keys, not copy — safe at module scope (no `t()` call here). The
// palette groups by this resolved value, so every producer must use these.
const GROUP_NAVIGATION = "common:commandGroupNavigation";
const GROUP_SETTINGS = "common:commandGroupSettings";
const GROUP_ACTIONS = "common:commandGroupActions";

/**
 * Search keywords are stored as one comma-separated catalog value so a
 * translator can localize the whole set in one entry. They are matched, never
 * displayed; the palette itself selects commands by `id` (see
 * `command-panel-footer.tsx`), so no behavior keys off this copy.
 */
function searchKeywords(t: TFunction, key: string): string[] {
  return t(key)
    .split(",")
    .map((keyword) => keyword.trim())
    .filter(Boolean);
}

function buildNavigationCommands(push: PushFn, t: TFunction): CommandItem[] {
  return [
    {
      id: "nav-home",
      label: t("common:commandGoToHome"),
      group: t(GROUP_NAVIGATION),
      icon: <IconHome className="size-3.5" />,
      keywords: searchKeywords(t, "common:commandGoToHomeKeywords"),
      action: () => push(linkToTaskOverview()),
    },
    {
      id: "nav-tasks",
      label: t("common:commandGoToAllTasks"),
      group: t(GROUP_NAVIGATION),
      icon: <IconList className="size-3.5" />,
      keywords: searchKeywords(t, "common:commandGoToAllTasksKeywords"),
      action: () => push("/tasks"),
    },
    {
      id: "nav-settings",
      label: t("common:commandGoToSettings"),
      group: t(GROUP_NAVIGATION),
      icon: <IconSettings className="size-3.5" />,
      keywords: searchKeywords(t, "common:commandGoToSettingsKeywords"),
      action: () => push("/settings/general"),
    },
    {
      id: "nav-stats",
      label: t("common:commandGoToStats"),
      group: t(GROUP_NAVIGATION),
      icon: <IconChartBar className="size-3.5" />,
      keywords: searchKeywords(t, "common:commandGoToStatsKeywords"),
      action: () => push("/stats"),
    },
    {
      id: "nav-github",
      label: t("common:commandGoToGitHubDashboard"),
      group: t(GROUP_NAVIGATION),
      icon: <IconBrandGithub className="size-3.5" />,
      keywords: searchKeywords(t, "common:commandGoToGitHubDashboardKeywords"),
      action: () => push("/github"),
    },
    {
      id: "settings-agents",
      label: t("common:commandAgentsSettings"),
      group: t(GROUP_SETTINGS),
      icon: <IconRobot className="size-3.5" />,
      keywords: searchKeywords(t, "common:commandAgentsSettingsKeywords"),
      action: () => push("/settings/agents"),
    },
    {
      id: "settings-executors",
      label: t("common:commandExecutorsSettings"),
      group: t(GROUP_SETTINGS),
      icon: <IconCpu className="size-3.5" />,
      keywords: searchKeywords(t, "common:commandExecutorsSettingsKeywords"),
      action: () => push("/settings/executors"),
    },
    {
      id: "settings-workspace",
      label: t("common:commandWorkspaceSettings"),
      group: t(GROUP_SETTINGS),
      icon: <IconFolder className="size-3.5" />,
      keywords: searchKeywords(t, "common:commandWorkspaceSettingsKeywords"),
      action: () => push("/settings/workspace"),
    },
    {
      id: "settings-prompts",
      label: t("common:commandPromptsSettings"),
      group: t(GROUP_SETTINGS),
      icon: <IconMessageCircle className="size-3.5" />,
      keywords: searchKeywords(t, "common:commandPromptsSettingsKeywords"),
      action: () => push("/settings/prompts"),
    },
  ];
}

function buildThemeCommand(
  resolvedTheme: string | undefined,
  setTheme: (theme: string) => void,
  t: TFunction,
): CommandItem {
  const isDark = resolvedTheme === "dark";
  const destinationTheme = isDark ? "light" : "dark";
  return {
    id: "pref-theme",
    label: isDark ? t("common:commandSwitchToLightMode") : t("common:commandSwitchToDarkMode"),
    group: t("common:commandGroupPreferences"),
    icon: isDark ? <IconSun className="size-3.5" /> : <IconMoon className="size-3.5" />,
    keywords: searchKeywords(t, "common:commandThemeKeywords"),
    action: () => setTheme(destinationTheme),
  };
}

export function GlobalCommands() {
  const { t } = useTranslation();
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
      label: t("common:commandQuickChat"),
      group: t(GROUP_ACTIONS),
      icon: <IconMessageCircle className="size-3.5" />,
      keywords: searchKeywords(t, "common:commandQuickChatKeywords"),
      shortcut: quickChatShortcut,
      action: handleOpenQuickChat,
    }),
    [handleOpenQuickChat, quickChatShortcut, t],
  );

  const configChatCommand: CommandItem = useMemo(
    () => ({
      id: "config-chat",
      label: t("common:configurationChat"),
      group: t(GROUP_ACTIONS),
      icon: <IconSparkles className="size-3.5" />,
      keywords: searchKeywords(t, "common:commandConfigChatKeywords"),
      action: handleOpenConfigChat,
    }),
    [handleOpenConfigChat, t],
  );

  const commands = useMemo<CommandItem[]>(
    () => [
      ...buildNavigationCommands(router.push, t),
      buildThemeCommand(resolvedTheme, setTheme, t),
      quickChatCommand,
      configChatCommand,
    ],
    [router.push, resolvedTheme, setTheme, quickChatCommand, configChatCommand, t],
  );

  useRegisterCommands(commands);
  useKeyboardShortcut(quickChatShortcut, handleOpenQuickChat);
  // Order matters: useAppShortcuts (core) must register its capture-phase
  // keydown listener before usePluginShortcuts so core shortcuts win when a
  // combo matches both — see the precedence note on each hook.
  useAppShortcuts();
  usePluginShortcuts();

  return null;
}
