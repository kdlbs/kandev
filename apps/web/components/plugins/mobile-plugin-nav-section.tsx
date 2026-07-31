"use client";

import { Button } from "@kandev/ui/button";
import Link from "@/components/routing/app-link";
import { resolvePluginIcon } from "@/lib/plugins/icons";
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
  const items = registry.getNavItems().filter((item) => (item.section ?? "main") === "main");

  if (items.length === 0) return null;

  return (
    <div className="space-y-3" data-testid="mobile-plugin-nav-section">
      <div className="text-sm font-medium">Plugins</div>
      {items.map((item) => {
        const Icon = resolvePluginIcon(item.icon);
        return (
          <Button
            key={item.id}
            asChild
            variant="outline"
            className="h-11 w-full cursor-pointer justify-start gap-2"
          >
            <Link
              href={item.path}
              onClick={onNavigate}
              data-testid={`mobile-plugin-nav-item-${item.id}`}
            >
              <Icon className="h-4 w-4 shrink-0" />
              <span className="flex-1 truncate text-left">{item.label}</span>
            </Link>
          </Button>
        );
      })}
    </div>
  );
}
