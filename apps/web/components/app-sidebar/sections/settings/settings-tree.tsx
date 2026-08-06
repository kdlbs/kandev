"use client";

import { useState, type ComponentType } from "react";
import { useTranslation } from "react-i18next";
import {
  IconActivity,
  IconArchive,
  IconBell,
  IconCommand,
  IconCpu,
  IconDatabase,
  IconFlask,
  IconFolder,
  IconInfoCircle,
  IconKey,
  IconLayoutDashboard,
  IconMessageCircle,
  IconMicrophone,
  IconPalette,
  IconPlugConnected,
  IconPuzzle,
  IconRefresh,
  IconRobot,
  IconShieldLock,
  IconTerminal2,
  IconUsers,
  IconWand,
} from "@tabler/icons-react";
import { PluginSlot } from "@/components/plugins/plugin-slot";
import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { usePlugins } from "@/hooks/domains/plugins/use-plugins";
import { useSecrets } from "@/hooks/domains/settings/use-secrets";
import { useSettingsDiscovery } from "@/hooks/domains/settings/use-settings-discovery";
import { BUILT_IN_LAYOUT_PROFILES, isBuiltInLayoutOverride } from "@/lib/layout/layout-profiles";
import {
  ACCOUNT_SECURITY_SETTINGS_HREF,
  AGENTS_SETTINGS_HREF,
  APPEARANCE_SETTINGS_HREF,
  EXECUTORS_SETTINGS_HREF,
  EXTERNAL_MCP_SETTINGS_HREF,
  KEYBOARD_SHORTCUTS_SETTINGS_HREF,
  LAYOUTS_SETTINGS_HREF,
  NOTIFICATIONS_SETTINGS_HREF,
  PLUGINS_SETTINGS_HREF,
  PROMPTS_SETTINGS_HREF,
  SECRETS_SETTINGS_HREF,
  SYSTEM_ABOUT_SETTINGS_HREF,
  SYSTEM_DATA_STORAGE_SETTINGS_HREF,
  SYSTEM_STATUS_SETTINGS_HREF,
  TASK_BEHAVIOR_SETTINGS_HREF,
  TERMINAL_EDITORS_SETTINGS_HREF,
  UTILITY_AGENTS_SETTINGS_HREF,
  VOICE_MODE_SETTINGS_HREF,
  WORKSPACES_SETTINGS_HREF,
} from "@/lib/settings-discovery/catalog";
import { SettingsLeaf, SettingsSectionHeader } from "./settings-nav-primitives";
import { SettingsSearch } from "./settings-search";

type MenuCountKey = "workspaces" | "agents" | "executors" | "layouts" | "secrets" | "plugins";

type MenuItem = {
  href: string;
  labelKey: string;
  icon: ComponentType<{ className?: string }>;
  /**
   * Route prefixes whose pages belong to this row (detail routes, sub-routes).
   * The row is active for `href` and anything under a prefix — user-created
   * items never get menu rows of their own, so their detail routes highlight
   * the page that owns them.
   */
  activePrefixes?: string[];
  requires?: "account" | "users";
  /** Rows whose page owns a list show its size as a trailing badge. */
  countKey?: MenuCountKey;
};

export type SettingsMenuSection = MenuSection;

type MenuSection = {
  id: string;
  labelKey: string;
  items: MenuItem[];
};

/**
 * The settings menu: static section headers, one row per page, exactly two
 * levels. Menu length is constant regardless of how many profiles, executors,
 * workspaces or plugins exist — those live as lists inside their pages.
 */
export const SETTINGS_MENU_SECTIONS: MenuSection[] = [
  {
    id: "preferences",
    labelKey: "settings:preferences",
    items: [
      { href: APPEARANCE_SETTINGS_HREF, labelKey: "settings:appearance", icon: IconPalette },
      { href: LAYOUTS_SETTINGS_HREF, labelKey: "settings:layouts", icon: IconLayoutDashboard, countKey: "layouts" },
      {
        href: TERMINAL_EDITORS_SETTINGS_HREF,
        labelKey: "settings:terminalAndEditors",
        icon: IconTerminal2,
      },
      { href: NOTIFICATIONS_SETTINGS_HREF, labelKey: "settings:notifications", icon: IconBell },
      {
        href: KEYBOARD_SHORTCUTS_SETTINGS_HREF,
        labelKey: "settings:keyboardShortcuts",
        icon: IconCommand,
      },
      { href: TASK_BEHAVIOR_SETTINGS_HREF, labelKey: "settings:taskBehavior", icon: IconArchive },
    ],
  },
  {
    id: "workspaces",
    labelKey: "settings:workspacesAndAccess",
    items: [
      {
        href: WORKSPACES_SETTINGS_HREF,
        labelKey: "common:workspaces",
        icon: IconFolder,
        activePrefixes: ["/settings/workspaces/", "/settings/workspace/"],
        countKey: "workspaces",
      },
      { href: SECRETS_SETTINGS_HREF, labelKey: "settings:globalSecrets", icon: IconKey, countKey: "secrets" },
      { href: EXTERNAL_MCP_SETTINGS_HREF, labelKey: "common:externalMcp", icon: IconPlugConnected },
      {
        href: PLUGINS_SETTINGS_HREF,
        labelKey: "common:plugins",
        icon: IconPuzzle,
        activePrefixes: ["/settings/plugins/"],
        countKey: "plugins",
      },
    ],
  },
  {
    id: "agents",
    labelKey: "common:agents",
    items: [
      {
        href: AGENTS_SETTINGS_HREF,
        labelKey: "common:agents",
        icon: IconRobot,
        activePrefixes: ["/settings/agents/", "/settings/agent/"],
        countKey: "agents",
      },
      {
        href: EXECUTORS_SETTINGS_HREF,
        labelKey: "common:executors",
        icon: IconCpu,
        activePrefixes: ["/settings/executors/", "/settings/executor/"],
        countKey: "executors",
      },
      { href: PROMPTS_SETTINGS_HREF, labelKey: "common:prompts", icon: IconMessageCircle },
      { href: UTILITY_AGENTS_SETTINGS_HREF, labelKey: "settings:utilityAgents", icon: IconWand },
      { href: VOICE_MODE_SETTINGS_HREF, labelKey: "settings:voiceMode", icon: IconMicrophone },
    ],
  },
  // Only rendered when the auth feature exposes at least one of its rows.
  {
    id: "access",
    labelKey: "settings:accessControl",
    items: [
      {
        href: "/settings/system/users",
        labelKey: "system:navUsers",
        icon: IconUsers,
        requires: "users",
      },
      {
        href: ACCOUNT_SECURITY_SETTINGS_HREF,
        labelKey: "sidebar:profileAndPassword",
        icon: IconShieldLock,
        requires: "account",
      },
      {
        href: "/settings/account/tokens",
        labelKey: "sidebar:apiTokens",
        icon: IconKey,
        requires: "account",
      },
    ],
  },
  {
    id: "system",
    labelKey: "common:system",
    items: [
      { href: SYSTEM_STATUS_SETTINGS_HREF, labelKey: "common:status", icon: IconActivity },
      {
        href: SYSTEM_DATA_STORAGE_SETTINGS_HREF,
        labelKey: "system:navDataStorage",
        icon: IconDatabase,
      },
      {
        href: "/settings/system/feature-toggles",
        labelKey: "system:navFeatureToggles",
        icon: IconFlask,
      },
      { href: "/settings/system/updates", labelKey: "system:navUpdates", icon: IconRefresh },
      { href: SYSTEM_ABOUT_SETTINGS_HREF, labelKey: "system:navAbout", icon: IconInfoCircle },
    ],
  },
];

/** True when `pathname` is the item's page or one of its owned sub-routes. */
export function settingsMenuItemIsActive(
  item: Pick<MenuItem, "href" | "activePrefixes">,
  pathname: string,
): boolean {
  if (pathname === item.href) return true;
  return (item.activePrefixes ?? []).some((prefix) => pathname.startsWith(prefix));
}

/** null user (disabled/synthetic single-user mode) counts as admin for gating. */
function useIsAdmin(): boolean {
  const role = useAppStore((s) => s.auth.user?.role);
  return role === undefined || role === "admin";
}

/**
 * Item counts for rows whose page owns a list. `undefined` until the backing
 * data is loaded, so rows never flash a wrong zero. Secrets and plugins load
 * through their store-backed hooks; the rest is already hydrated by the
 * settings bootstrap.
 */
function useSettingsMenuCounts(): Partial<Record<MenuCountKey, number>> {
  const hydrated = useAppStore((s) => s.settingsData.executorsLoaded);
  const workspaceCount = useAppStore((s) => s.workspaces.items.length);
  const agentProfileCount = useAppStore((s) =>
    s.settingsAgents.items.reduce((sum, agent) => sum + agent.profiles.length, 0),
  );
  const executorProfileCount = useAppStore((s) =>
    s.executors.items.reduce((sum, executor) => sum + (executor.profiles?.length ?? 0), 0),
  );
  const userSettingsLoaded = useAppStore((s) => s.userSettings.loaded);
  // Built-ins always exist; overrides replace a built-in, so only customs add.
  const layoutCount = useAppStore(
    (s) =>
      BUILT_IN_LAYOUT_PROFILES.length +
      s.userSettings.savedLayouts.filter((layout) => !isBuiltInLayoutOverride(layout)).length,
  );
  const secrets = useSecrets();
  const plugins = usePlugins();

  return {
    ...(hydrated
      ? {
          workspaces: workspaceCount,
          agents: agentProfileCount,
          executors: executorProfileCount,
        }
      : {}),
    ...(userSettingsLoaded ? { layouts: layoutCount } : {}),
    ...(secrets.loaded ? { secrets: secrets.items.length } : {}),
    ...(plugins.loaded ? { plugins: plugins.items.length } : {}),
  };
}

function MenuCountBadge({ count }: { count: number }) {
  return (
    <span className="shrink-0 text-[11px] leading-none text-muted-foreground/70 tabular-nums">
      {count}
    </span>
  );
}

/**
 * The settings nav: a fixed two-level menu. Group labels are static section
 * headers — not clickable, no expand/collapse — and every row is a page.
 *
 * Rendered both inside the sidebar settings takeover and, on a phone, as the
 * `/settings` index page body.
 */
export function SettingsTree({
  pathname,
  searchLayout,
}: {
  pathname: string;
  /** `floating` pins the search field in thumb reach — see `SettingsSearch`. */
  searchLayout?: "inline" | "floating";
}) {
  const { t } = useTranslation();
  const authEnabled = useFeature("auth");
  const authMode = useAppStore((s) => s.auth.mode);
  const isAdmin = useIsAdmin();
  const showAccountItems = authEnabled && authMode === "enabled";
  const showUsersItem = authEnabled && isAdmin;
  const discoveryItems = useSettingsDiscovery();
  const counts = useSettingsMenuCounts();
  const [query, setQuery] = useState("");

  const itemVisible = (item: MenuItem) => {
    if (item.requires === "account") return showAccountItems;
    if (item.requires === "users") return showUsersItem;
    return true;
  };

  return (
    <>
      <SettingsSearch
        items={discoveryItems}
        query={query}
        onQueryChange={setQuery}
        onSelect={() => setQuery("")}
        {...(searchLayout ? { layout: searchLayout } : {})}
      />
      {query.trim() ? null : (
        <>
          {SETTINGS_MENU_SECTIONS.map((section) => {
            const items = section.items.filter(itemVisible);
            if (items.length === 0) return null;
            return (
              <div key={section.id} className="flex flex-col gap-0.5">
                <SettingsSectionHeader label={t(section.labelKey)} />
                {items.map((item) => {
                  const count = item.countKey ? counts[item.countKey] : undefined;
                  return (
                    <SettingsLeaf
                      key={item.href}
                      href={item.href}
                      label={t(item.labelKey)}
                      icon={item.icon}
                      isActive={settingsMenuItemIsActive(item, pathname)}
                      {...(count !== undefined
                        ? { labelSuffix: <MenuCountBadge count={count} /> }
                        : {})}
                    />
                  );
                })}
                {/* Plugins may add rows here, directly below the Plugins page. */}
                {section.id === "workspaces" && <PluginSlot name="settings-nav" />}
              </div>
            );
          })}
        </>
      )}
    </>
  );
}
