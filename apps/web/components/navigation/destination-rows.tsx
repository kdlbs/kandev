"use client";

import { Button } from "@kandev/ui/button";
import { IconChevronRight } from "@tabler/icons-react";
import Link from "@/components/routing/app-link";
import type { ResolvedDestination } from "@/lib/navigation/types";
import { usePathname } from "@/lib/routing/client-router";
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
  homeCoversListings?: boolean;
  showChevron?: boolean;
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
  homeCoversListings,
  showChevron,
}: DestinationRowsProps) {
  return (
    <>
      {destinations.map((destination) => (
        <DestinationRow
          key={destination.id}
          destination={destination}
          homeCoversListings={homeCoversListings}
          showChevron={showChevron}
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
  homeCoversListings?: boolean;
  showChevron?: boolean;
};

function isDestinationCurrent(
  destination: ResolvedDestination,
  pathname: string,
  homeCoversListings?: boolean,
) {
  const hrefPath = destination.href.split("?")[0];
  if (destination.id === "home") {
    return (
      pathname === "/" ||
      (hrefPath === "/office" && pathname === hrefPath) ||
      (homeCoversListings === true && ["/tasks", "/threads"].includes(pathname))
    );
  }
  return (
    (destination.id === "tasks" && /^\/(?:t|tasks)\/[^/]+/.test(pathname)) ||
    pathname === hrefPath ||
    (hrefPath !== "/" && pathname.startsWith(`${hrefPath}/`))
  );
}

export function DestinationRow({
  destination,
  onNavigate,
  pluginTestIdPrefix,
  className,
  homeCoversListings,
  showChevron,
}: DestinationRowProps) {
  const Icon = destination.icon;
  const pathname = usePathname();
  const current = isDestinationCurrent(destination, pathname, homeCoversListings);
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
      className={cn(
        "h-11 w-full cursor-pointer justify-start gap-2",
        current && "border-primary/40 bg-accent",
        className,
      )}
    >
      <Link
        href={destination.href}
        onClick={onNavigate}
        data-testid={testId}
        aria-current={current ? "page" : undefined}
      >
        <Icon className="h-4 w-4 shrink-0" />
        <span className="flex-1 truncate text-left">{destination.label}</span>
        {showChevron && (
          <IconChevronRight
            className="size-3.5 shrink-0 text-muted-foreground"
            aria-hidden="true"
          />
        )}
      </Link>
    </Button>
  );
}
