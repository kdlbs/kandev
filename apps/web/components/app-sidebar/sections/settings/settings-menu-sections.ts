import type { ComponentType } from "react";
import {
  IconActivity,
  IconArchive,
  IconBell,
  IconCommand,
  IconCpu,
  IconDatabase,
  IconBuilding,
  IconFlask,
  IconFolder,
  IconInfoCircle,
  IconKey,
  IconLayoutDashboard,
  IconMessageCircle,
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
  WORKSPACES_SETTINGS_HREF,
} from "@/lib/settings-discovery/catalog";

/**
 * The settings menu as data.
 *
 * This is the settings information architecture: which pages exist, what they
 * are called, and which detail routes belong to which page. Two surfaces read
 * it and must never disagree — the sidebar tree (`settings-tree.tsx`, which
 * highlights the owning row) and the settings breadcrumb
 * (`settings-breadcrumbs.ts`, which names the owning row as a parent crumb).
 * Keeping it in a data-only module is what lets the second one import it
 * without pulling in the sidebar's hooks and plugin slots.
 */

type MenuCountKey = "workspaces" | "agents" | "executors" | "layouts" | "secrets" | "plugins";

export type SettingsMenuItem = {
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

export type SettingsMenuSection = {
  id: string;
  labelKey: string;
  items: SettingsMenuItem[];
};

export type SettingsMenuCountKey = MenuCountKey;

/**
 * The settings menu: static section headers, one row per page, exactly two
 * levels. Menu length is constant regardless of how many profiles, executors,
 * workspaces or plugins exist — those live as lists inside their pages.
 */
export const SETTINGS_MENU_SECTIONS: SettingsMenuSection[] = [
  {
    id: "preferences",
    labelKey: "settings:preferences",
    items: [
      { href: APPEARANCE_SETTINGS_HREF, labelKey: "settings:appearance", icon: IconPalette },
      {
        href: LAYOUTS_SETTINGS_HREF,
        labelKey: "settings:layouts",
        icon: IconLayoutDashboard,
        countKey: "layouts",
      },
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
      {
        href: SECRETS_SETTINGS_HREF,
        labelKey: "settings:globalSecrets",
        icon: IconKey,
        countKey: "secrets",
      },
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
      {
        href: "/settings/system/organizations",
        labelKey: "orgs:navOrganizations",
        icon: IconBuilding,
      },
      { href: "/settings/system/updates", labelKey: "system:navUpdates", icon: IconRefresh },
      { href: SYSTEM_ABOUT_SETTINGS_HREF, labelKey: "system:navAbout", icon: IconInfoCircle },
    ],
  },
];

/** True when `pathname` is the item's page or one of its owned sub-routes. */
export function settingsMenuItemIsActive(
  item: Pick<SettingsMenuItem, "href" | "activePrefixes">,
  pathname: string,
): boolean {
  if (pathname === item.href) return true;
  return (item.activePrefixes ?? []).some((prefix) => pathname.startsWith(prefix));
}

/**
 * The menu row whose page owns `pathname` as a *sub-route*, or null.
 *
 * Deliberately excludes the row's own page: this answers "which settings page
 * should the breadcrumb name as this page's parent", and a page is not its own
 * parent. Every `activePrefixes` entry ends in `/`, so a row's own href can
 * never match one — no extra guard needed.
 *
 * `requires` gating is not consulted. It hides rows the current user has no
 * page for, and every gated row is a leaf, so no crumb is ever built from one.
 */
export function settingsMenuOwnerOf(pathname: string): SettingsMenuItem | null {
  for (const section of SETTINGS_MENU_SECTIONS) {
    for (const item of section.items) {
      if ((item.activePrefixes ?? []).some((prefix) => pathname.startsWith(prefix))) return item;
    }
  }
  return null;
}
