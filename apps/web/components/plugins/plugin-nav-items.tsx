"use client";

import { AppSidebarNavItem } from "@/components/app-sidebar/app-sidebar-nav-item";
import { resolveDestinations } from "@/lib/navigation/resolve-destinations";
import { NO_WORKSPACE_CONTEXT } from "@/lib/navigation/surface-policy";
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
 *
 * Section routing and icon resolution come from the navigation manifest, so this
 * rail and the mobile menu's plugin group cannot disagree about which items
 * belong here. Resolved directly rather than through `useStaticDestinations`:
 * plugin paths are static, so there is no reason to read workspace context.
 */
export function PluginNavItems({ collapsed }: PluginNavItemsProps) {
  const registry = usePluginRegistry();
  const destinations = resolveDestinations({
    surface: "sidebar",
    section: "plugins",
    ctx: NO_WORKSPACE_CONTEXT,
    pluginItems: registry.getNavRegistrations(),
  });

  return (
    <>
      {destinations.map((destination) => (
        <AppSidebarNavItem
          key={destination.id}
          icon={destination.icon}
          label={destination.label}
          href={destination.href}
          collapsed={collapsed}
          testId={`plugin-nav-item-${destination.pluginItemId ?? destination.id}`}
        />
      ))}
    </>
  );
}
