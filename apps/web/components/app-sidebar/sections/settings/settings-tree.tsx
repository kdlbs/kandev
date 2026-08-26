"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { PluginSlot } from "@/components/plugins/plugin-slot";
import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useIsAdmin } from "@/hooks/domains/auth/use-is-admin";
import { usePlugins } from "@/hooks/domains/plugins/use-plugins";
import { useSecrets } from "@/hooks/domains/settings/use-secrets";
import { useSettingsDiscovery } from "@/hooks/domains/settings/use-settings-discovery";
import { BUILT_IN_LAYOUT_PROFILES, isBuiltInLayoutOverride } from "@/lib/layout/layout-profiles";
import { SettingsBranch, SettingsLeaf, SettingsSectionHeader } from "./settings-nav-primitives";
import { SettingsSearch } from "./settings-search";
import { SettingsMenuNodeRow } from "./settings-menu-node";
import type { SettingsMenuNode } from "./settings-menu-branches";
import { useSettingsMenuBranches, useSettingsMenuForest } from "./use-settings-menu-branches";
import {
  useSettingsMenuExpansion,
  type SettingsMenuExpansion,
} from "./use-settings-menu-expansion";
import {
  SETTINGS_MENU_SECTIONS,
  settingsMenuItemIsActive,
  type SettingsMenuCountKey,
  type SettingsMenuItem,
} from "./settings-menu-sections";

// The menu itself is data — see `settings-menu-sections.ts`, which the settings
// breadcrumb reads too. Re-exported here so existing callers keep one import.
export {
  SETTINGS_MENU_SECTIONS,
  settingsMenuItemIsActive,
  settingsMenuOwnerOf,
  type SettingsMenuItem,
  type SettingsMenuSection,
} from "./settings-menu-sections";

/**
 * Item counts for rows whose page owns a list. `undefined` until the backing
 * data is loaded, so rows never flash a wrong zero. Secrets and plugins load
 * through their store-backed hooks; the rest is already hydrated by the
 * settings bootstrap.
 */
function useSettingsMenuCounts(): Partial<Record<SettingsMenuCountKey, number>> {
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
 * One menu row: a plain link, or — in a tree mode, for a row whose page owns
 * records — a disclosure holding them.
 *
 * The active rule is what keeps the two shapes honest. `settingsMenuItemIsActive`
 * marks a row for its own page *and* everything under it, which is right when
 * the sub-page has no row of its own. Once a branch renders that sub-page as a
 * row, the deeper row is the page you are on and the ancestor is merely the way
 * there — so the row defers whenever a node claims the route, and exactly one
 * row in the menu carries `data-active` in every mode.
 */
function SettingsMenuRow({
  item,
  pathname,
  count,
  branch,
  expansion,
}: {
  item: SettingsMenuItem;
  pathname: string;
  count: number | undefined;
  branch: SettingsMenuNode | undefined;
  expansion: SettingsMenuExpansion;
}) {
  const { t } = useTranslation();
  const label = t(item.labelKey);
  const labelSuffix = count !== undefined ? <MenuCountBadge count={count} /> : undefined;

  if (!branch) {
    return (
      <SettingsLeaf
        href={item.href}
        label={label}
        icon={item.icon}
        isActive={settingsMenuItemIsActive(item, pathname) && expansion.activeKey === null}
        labelSuffix={labelSuffix}
      />
    );
  }

  return (
    <SettingsBranch
      label={label}
      icon={item.icon}
      labelSuffix={labelSuffix}
      href={item.href}
      isActive={expansion.activeKey === branch.key}
      expanded={expansion.isExpanded(branch.key)}
      onToggle={() => expansion.toggle(branch.key)}
    >
      {(branch.children ?? []).map((node) => (
        <SettingsMenuNodeRow
          key={node.key}
          node={node}
          depth={1}
          activeKey={expansion.activeKey}
          expansion={expansion}
        />
      ))}
    </SettingsBranch>
  );
}

/**
 * The settings nav. Group labels are static section headers — not clickable, no
 * expand/collapse — and every row is a page.
 *
 * How deep it goes is a per-device preference (Settings → Appearance):
 * `flat` is the fixed two-level menu and the default; `accordion` and
 * `persistent` additionally grow the Workspaces, Agents and Executors rows into
 * their records, differing only in whether opening one branch closes the others.
 * See `settings-menu-branches.ts` for what those branches contain.
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
  const mode = useAppStore((s) => s.settingsMenu.mode);
  const branches = useSettingsMenuBranches(mode);
  const forest = useSettingsMenuForest(branches);
  const expansion = useSettingsMenuExpansion(mode, forest, pathname);

  const itemVisible = (item: SettingsMenuItem) => {
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
                {items.map((item) => (
                  <SettingsMenuRow
                    key={item.href}
                    item={item}
                    pathname={pathname}
                    count={item.countKey ? counts[item.countKey] : undefined}
                    branch={branches[item.href]}
                    expansion={expansion}
                  />
                ))}
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
