"use client";

import { AppSidebarNavItem } from "@/components/app-sidebar/app-sidebar-nav-item";
import { resolvePluginIcon } from "@/lib/plugins/icons";
import { usePluginRegistry } from "@/lib/plugins/registry";

type PluginNavItemsProps = {
  collapsed: boolean;
};

/**
 * Renders every plugin-registered "main" section nav item
 * (`registry.registerNavItem(item)`) in the app sidebar, styled and behaving
 * like a first-party `AppSidebarNavItem`. The contract's opaque `icon` name
 * string resolves against the curated map in `lib/plugins/icons.ts`;
 * unknown/missing names fall back to a generic puzzle-piece glyph. Renders
 * nothing while the registry holds no "main"-section items.
 */
export function PluginNavItems({ collapsed }: PluginNavItemsProps) {
  const registry = usePluginRegistry();
  const items = registry.getNavItems().filter((item) => (item.section ?? "main") === "main");

  return (
    <>
      {items.map((item) => (
        <AppSidebarNavItem
          key={item.id}
          icon={resolvePluginIcon(item.icon)}
          label={item.label}
          href={item.path}
          collapsed={collapsed}
          testId={`plugin-nav-item-${item.id}`}
        />
      ))}
    </>
  );
}
