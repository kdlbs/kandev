"use client";

import { Button } from "@kandev/ui/button";
import Link from "@/components/routing/app-link";
import type { ResolvedDestination } from "@/lib/navigation/types";
import { cn } from "@/lib/utils";

type DestinationRowsProps = {
  destinations: ResolvedDestination[];
  /** Closes the surrounding sheet/drawer once a destination is opened. */
  onNavigate: () => void;
  /**
   * Test-id prefix applied to plugin-registered entries only. Surfaces use
   * different prefixes (`plugin-nav-item-` in the sidebar and the mobile
   * integrations group, `mobile-plugin-nav-item-` in the mobile plugins group);
   * both are referenced by e2e specs, so the caller passes the one it owns.
   */
  pluginTestIdPrefix?: string;
  /** Extra classes so a surface can keep its own row spacing. */
  className?: string;
};

/**
 * Touch rows for a resolved destination list — the shared renderer behind every
 * mobile-menu navigation group. Data comes from the navigation manifest (via
 * `useAppDestinations` / `useStaticDestinations`), so a destination added to the
 * manifest shows up here without touching this file.
 */
export function DestinationRows({
  destinations,
  onNavigate,
  pluginTestIdPrefix,
  className,
}: DestinationRowsProps) {
  return (
    <>
      {destinations.map((destination) => (
        <DestinationRow
          key={destination.id}
          destination={destination}
          onNavigate={onNavigate}
          {...(pluginTestIdPrefix ? { pluginTestIdPrefix } : {})}
          {...(className ? { className } : {})}
        />
      ))}
    </>
  );
}

type DestinationRowProps = {
  destination: ResolvedDestination;
  onNavigate: () => void;
  pluginTestIdPrefix?: string;
  className?: string;
};

export function DestinationRow({
  destination,
  onNavigate,
  pluginTestIdPrefix,
  className,
}: DestinationRowProps) {
  const Icon = destination.icon;
  // Built from the raw `NavItem.id`, not the namespaced destination id — the
  // `plugin-nav-item-<id>` / `mobile-plugin-nav-item-<id>` ids are public contract.
  const testId =
    destination.source === "plugin" && pluginTestIdPrefix
      ? `${pluginTestIdPrefix}${destination.pluginItemId ?? destination.id}`
      : undefined;

  return (
    <Button
      asChild
      variant="outline"
      className={cn("h-11 w-full cursor-pointer justify-start gap-2", className)}
    >
      <Link href={destination.href} onClick={onNavigate} data-testid={testId}>
        <Icon className="h-4 w-4 shrink-0" />
        <span className="flex-1 truncate text-left">{destination.label}</span>
      </Link>
    </Button>
  );
}
