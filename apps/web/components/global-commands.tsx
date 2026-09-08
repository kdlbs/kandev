"use client";

import { useMemo, useRef } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { useRouter } from "@/lib/routing/client-router";
import { useTheme } from "@/components/theme/app-theme";
import { getThemeToggleLabelKey, getThemeToggleTarget } from "@/components/theme/theme-toggle";
import { IconSun, IconMoon, IconMessageCircle, IconSparkles } from "@tabler/icons-react";
import { useStaticDestinations } from "@/hooks/use-app-destinations";
import { PALETTE_NAVIGATION_GROUP_KEY } from "@/lib/navigation/surface-policy";
import { useRegisterCommands } from "@/hooks/use-register-commands";
import { searchKeywords } from "@/lib/commands/search-keywords";
import { useKeyboardShortcut } from "@/hooks/use-keyboard-shortcut";
import { useAppShortcuts } from "@/hooks/use-app-shortcuts";
import { usePluginShortcuts } from "@/hooks/use-plugin-shortcuts";
import { useAppStore } from "@/components/state-provider";
import { useQuickChatLauncher } from "@/hooks/use-quick-chat-launcher";
import { getShortcut } from "@/lib/keyboard/shortcut-overrides";
import type { CommandItem } from "@/lib/commands/types";
import { SettingsDiscoveryCommands } from "@/components/settings-discovery-commands";

type PushFn = ReturnType<typeof useRouter>["push"];

// Catalog keys, not copy — safe at module scope (no `t()` call here). The
// palette groups by this resolved value, so every producer must use these.
const GROUP_NAVIGATION = PALETTE_NAVIGATION_GROUP_KEY;
const GROUP_ACTIONS = "common:commandGroupActions";

/**
 * The Navigation group comes from the navigation manifest
 * (`lib/navigation/core-destinations.ts`), so a destination cannot reach the palette
 * without also being offered on the surfaces it declares. Command ids and copy
 * live in each destination's `palette` block — they are stable API for tests and
 * telemetry. The Settings group below stays hand-written: those are deep links
 * into one destination, not destinations of their own.
 */
function useNavigationCommands(push: PushFn, t: TFunction): CommandItem[] {
  // `useStaticDestinations`, not `useAppDestinations`: this component is mounted
  // for the whole session, and the palette's one gated entry (GitHub) opts out of
  // the availability check, so subscribing to integration polling here would add
  // background requests for nothing.
  const destinations = useStaticDestinations("palette");
  // Cached on a value signature so the command array keeps its identity across
  // unrelated re-renders — `useRegisterCommands` re-registers whenever the array
  // changes. Same render-time cache pattern as `use-responsive-breakpoint.ts`;
  // a `useMemo` cannot work here because the resolved list is rebuilt each render.
  const group = t(GROUP_NAVIGATION);
  const resolved = destinations.map((destination) => ({
    destination,
    keywords: destination.palette?.keywordsKey
      ? searchKeywords(t, destination.palette.keywordsKey)
      : [],
  }));
  const signature = JSON.stringify(
    resolved.map(({ destination, keywords }) => [
      destination.id,
      destination.palette?.id,
      destination.href,
      destination.label,
      group,
      keywords,
    ]),
  );
  const cacheRef = useRef<{
    signature: string;
    push: PushFn;
    commands: CommandItem[];
  } | null>(null);

  if (
    !cacheRef.current ||
    cacheRef.current.signature !== signature ||
    cacheRef.current.push !== push
  ) {
    cacheRef.current = {
      signature,
      push,
      commands: resolved.map(({ destination, keywords }) => {
        const Icon = destination.icon;
        return {
          id: destination.palette?.id ?? `nav-${destination.id}`,
          label: destination.label,
          group,
          icon: <Icon className="size-3.5" />,
          keywords,
          action: () => push(destination.href),
        };
      }),
    };
  }

  return cacheRef.current.commands;
}

function buildThemeCommand(
  resolvedTheme: string | undefined,
  setTheme: (theme: string) => void,
  t: TFunction,
): CommandItem {
  const isDark = resolvedTheme === "dark";
  const currentTheme = isDark ? "dark" : "light";
  const destinationTheme = getThemeToggleTarget(currentTheme);
  return {
    id: "pref-theme",
    label: t(getThemeToggleLabelKey(currentTheme)),
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
  const handleOpenQuickChat = useQuickChatLauncher(activeWorkspaceId, "chat", {
    silentFocusReturn: false,
    toggleWhenOpen: true,
  });
  const handleOpenConfigChat = useQuickChatLauncher(activeWorkspaceId, "config", {
    silentFocusReturn: false,
  });

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

  const navigationCommands = useNavigationCommands(router.push, t);

  const commands = useMemo<CommandItem[]>(
    () => [
      ...navigationCommands,
      buildThemeCommand(resolvedTheme, setTheme, t),
      quickChatCommand,
      configChatCommand,
    ],
    [
      navigationCommands,
      router.push,
      resolvedTheme,
      setTheme,
      quickChatCommand,
      configChatCommand,
      t,
    ],
  );

  useRegisterCommands(commands);
  // Order matters: useAppShortcuts (core) must register its capture-phase
  // keydown listener before the Quick Chat toggle and plugin shortcuts. This
  // preserves core precedence, while the toggle still runs before xterm can
  // consume a configured shortcut.
  useAppShortcuts();
  useKeyboardShortcut(quickChatShortcut, handleOpenQuickChat, {
    capture: true,
    stopPropagation: true,
  });
  usePluginShortcuts();

  return <SettingsDiscoveryCommands />;
}
