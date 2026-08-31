"use client";

import { useTranslation } from "react-i18next";
import { DestinationRows } from "@/components/navigation/destination-rows";
import { resolveDestinations } from "@/lib/navigation/resolve-destinations";
import { NO_WORKSPACE_CONTEXT } from "@/lib/navigation/surface-policy";
import { usePluginRegistry } from "@/lib/plugins/registry";

type MobilePluginNavSectionProps = {
  /** Closes the surrounding menu sheet once a plugin page is opened. */
  onNavigate: () => void;
  /** Generic placement after the Integrations group, for workspace agents. */
  section?: "plugins" | "workspace-agents";
  /** Omit when the surrounding product surface supplies its own label. */
  heading?: boolean;
};

/**
 * Mobile counterpart to the desktop sidebar's `<PluginNavItems/>`: the
 * hamburger-sheet surface for plugin-registered main items by default, or the
 * dedicated post-Integrations workspace-agent slot when requested. The desktop
 * rail is `hidden md:block`, so each static plugin destination needs a matching
 * phone entry point. "integrations"-section items keep rendering inside
 * `MobileIntegrationsSection`.
 */
export function MobilePluginNavSection({
  onNavigate,
  section = "plugins",
  heading = true,
}: MobilePluginNavSectionProps) {
  const { t } = useTranslation();
  const registry = usePluginRegistry();
  // Resolved directly rather than through `useStaticDestinations`: this group's
  // hrefs are static plugin paths, so it needs neither workspace context nor the
  // availability subscription. Matches the desktop rail in `plugin-nav-items.tsx`.
  const destinations = resolveDestinations({
    surface: "mobileMenu",
    section,
    ctx: NO_WORKSPACE_CONTEXT,
    pluginItems: registry.getNavRegistrations(),
  });

  if (destinations.length === 0) return null;

  return (
    <div className="space-y-3" data-testid="mobile-plugin-nav-section">
      {heading && <div className="text-sm font-medium">{t("common:plugins")}</div>}
      <DestinationRows
        destinations={destinations}
        onNavigate={onNavigate}
        pluginTestIdPrefix="mobile-plugin-nav-item-"
      />
    </div>
  );
}
