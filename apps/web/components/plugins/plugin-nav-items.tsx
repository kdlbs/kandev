"use client";

import { IconPuzzle } from "@tabler/icons-react";
import { AppSidebarNavItem } from "@/components/app-sidebar/app-sidebar-nav-item";
import { usePluginRegistry } from "@/lib/plugins/registry";

type PluginNavItemsProps = {
  collapsed: boolean;
};

/**
 * Renders every plugin-registered "main" section nav item
 * (`registry.registerNavItem(item)`) in the app sidebar, styled and behaving
 * like a first-party `AppSidebarNavItem`. Plugin nav items don't carry a
 * host icon component (only an opaque `icon` name string in the frozen
 * contract), so every entry uses a generic puzzle-piece glyph.
 */
export function PluginNavItems({ collapsed }: PluginNavItemsProps) {
  const registry = usePluginRegistry();
  const items = registry.getNavItems().filter((item) => (item.section ?? "main") === "main");

  return (
    <>
      {items.map((item) => (
        <AppSidebarNavItem
          key={item.id}
          icon={IconPuzzle}
          label={item.label}
          href={item.path}
          collapsed={collapsed}
          testId={`plugin-nav-item-${item.id}`}
        />
      ))}
    </>
  );
}
