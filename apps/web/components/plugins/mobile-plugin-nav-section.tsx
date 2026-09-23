"use client";

import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { DestinationRows } from "@/components/navigation/destination-rows";
import { resolveDestinations } from "@/lib/navigation/resolve-destinations";
import { NO_WORKSPACE_CONTEXT } from "@/lib/navigation/surface-policy";
import { usePluginRegistry } from "@/lib/plugins/registry";

type MobilePluginNavSectionProps = {
  /** Closes the surrounding menu sheet once a plugin page is opened. */
  onNavigate: () => void;
  /** Page-scoped plugin status and controls for this menu. */
  actions?: ReactNode;
  /** Custom sidebar layouts already place plugin destinations themselves. */
  includeDestinations?: boolean;
};

/**
 * Mobile counterpart to the desktop sidebar's `<PluginNavItems/>`: the
 * hamburger-sheet surface for plugin-registered "main"-section nav items
 * (`registerNavItem({ section: "main" })`, the default). The desktop rail is
 * `hidden md:block`, so without this section a plugin's own page has no phone
 * entry point at all. "integrations"-section items keep rendering inside
 * `MobileIntegrationsSection`.
 */
export function MobilePluginNavSection({
  onNavigate,
  actions,
  includeDestinations = true,
}: MobilePluginNavSectionProps) {
  const { t } = useTranslation();
  const registry = usePluginRegistry();
  // Resolved directly rather than through `useStaticDestinations`: this group's
  // hrefs are static plugin paths, so it needs neither workspace context nor the
  // availability subscription. Matches the desktop rail in `plugin-nav-items.tsx`.
  const destinations = includeDestinations
    ? resolveDestinations({
        surface: "mobileMenu",
        section: "plugins",
        ctx: NO_WORKSPACE_CONTEXT,
        pluginItems: registry.getNavRegistrations(),
      })
    : [];

  if (!actions && destinations.length === 0) return null;

  return (
    <div className="space-y-3" data-testid="mobile-plugin-nav-section">
      <div className="text-sm font-medium">{t("common:plugins")}</div>
      {actions}
      {destinations.length > 0 && (
        <DestinationRows
          destinations={destinations}
          onNavigate={onNavigate}
          pluginTestIdPrefix="mobile-plugin-nav-item-"
        />
      )}
    </div>
  );
}
