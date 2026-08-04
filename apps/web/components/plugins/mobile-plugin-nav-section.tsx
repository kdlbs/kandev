"use client";

import { DestinationRows } from "@/components/navigation/destination-rows";
import { resolveDestinations } from "@/lib/navigation/resolve-destinations";
import { NO_WORKSPACE_CONTEXT } from "@/lib/navigation/surface-policy";
import { usePluginRegistry } from "@/lib/plugins/registry";

type MobilePluginNavSectionProps = {
  /** Closes the surrounding menu sheet once a plugin page is opened. */
  onNavigate: () => void;
};

/**
 * Mobile counterpart to the desktop sidebar's `<PluginNavItems/>`: the
 * hamburger-sheet surface for plugin-registered "main"-section nav items
 * (`registerNavItem({ section: "main" })`, the default). The desktop rail is
 * `hidden md:block`, so without this section a plugin's own page has no phone
 * entry point at all. "integrations"-section items keep rendering inside
 * `MobileIntegrationsSection`.
 */
export function MobilePluginNavSection({ onNavigate }: MobilePluginNavSectionProps) {
  const registry = usePluginRegistry();
  // Resolved directly rather than through `useStaticDestinations`: this group's
  // hrefs are static plugin paths, so it needs neither workspace context nor the
  // availability subscription. Matches the desktop rail in `plugin-nav-items.tsx`.
  const destinations = resolveDestinations({
    surface: "mobileMenu",
    section: "plugins",
    ctx: NO_WORKSPACE_CONTEXT,
    pluginItems: registry.getNavRegistrations(),
  });

  if (destinations.length === 0) return null;

  return (
    <div className="space-y-3" data-testid="mobile-plugin-nav-section">
      <div className="text-sm font-medium">Plugins</div>
      <DestinationRows
        destinations={destinations}
        onNavigate={onNavigate}
        pluginTestIdPrefix="mobile-plugin-nav-item-"
      />
    </div>
  );
}
