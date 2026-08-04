"use client";

import { useEffect, useRef, useState } from "react";
import Link from "@/components/routing/app-link";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { IconPlugConnected } from "@tabler/icons-react";
import { DestinationRows } from "@/components/navigation/destination-rows";
import { useAppDestinations } from "@/hooks/use-app-destinations";

type MobileIntegrationsSectionProps = {
  onNavigate: () => void;
};

const HOVER_CLOSE_DELAY_MS = 180;

/**
 * Configured integration destinations, in manifest order. Ids, labels, hrefs and
 * icons come from `lib/navigation/core-destinations.ts`; availability comes from
 * `useNavAvailability` through `useAppDestinations`.
 */
function useIntegrationDestinations() {
  return useAppDestinations("sidebar", "integrations");
}

export function IntegrationsMenu() {
  const links = useIntegrationDestinations();
  const [open, setOpen] = useState(false);
  const closeTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (closeTimeoutRef.current) clearTimeout(closeTimeoutRef.current);
    };
  }, []);

  const clearCloseTimeout = () => {
    if (!closeTimeoutRef.current) return;
    clearTimeout(closeTimeoutRef.current);
    closeTimeoutRef.current = null;
  };

  const openOnHover = () => {
    clearCloseTimeout();
    setOpen((current) => (current ? current : true));
  };

  const closeAfterHover = () => {
    clearCloseTimeout();
    closeTimeoutRef.current = setTimeout(() => setOpen(false), HOVER_CLOSE_DELAY_MS);
  };

  const handleOpenChange = (nextOpen: boolean) => {
    clearCloseTimeout();
    setOpen(nextOpen);
  };

  if (links.length === 0) return null;

  return (
    <DropdownMenu open={open} onOpenChange={handleOpenChange} modal={false}>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="icon-lg"
          className="cursor-pointer text-muted-foreground hover:text-foreground"
          aria-label="Integrations"
          onPointerEnter={openOnHover}
          onPointerLeave={closeAfterHover}
        >
          <IconPlugConnected className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="end"
        className="w-48"
        onPointerEnter={openOnHover}
        onPointerLeave={closeAfterHover}
      >
        <DropdownMenuLabel>Integrations</DropdownMenuLabel>
        {links.map((link) => {
          const Icon = link.icon;
          return (
            <DropdownMenuItem key={link.id} asChild className="cursor-pointer">
              <Link href={link.href}>
                <Icon className="h-4 w-4 text-muted-foreground" />
                {link.label}
              </Link>
            </DropdownMenuItem>
          );
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export function IntegrationsTopbarLinks() {
  const links = useIntegrationDestinations();
  if (links.length === 0) return null;

  return (
    <>
      {links.map((link) => {
        const Icon = link.icon;
        return (
          <Tooltip key={link.id}>
            <TooltipTrigger asChild>
              <Button asChild variant="outline" size="icon-lg" className="cursor-pointer">
                <Link href={link.href} aria-label={link.label}>
                  <Icon className="h-4 w-4" />
                </Link>
              </Button>
            </TooltipTrigger>
            <TooltipContent>{link.label}</TooltipContent>
          </Tooltip>
        );
      })}
    </>
  );
}

/**
 * Mobile counterpart to the desktop sidebar Integrations section: the
 * hamburger-sheet surface that exposes the first-party integration links plus
 * any plugin-registered nav items targeting this section
 * (`registerNavItem({ section: "integrations" })`), matching the desktop
 * `IntegrationsSection`. Both come from the navigation manifest, so the two
 * surfaces cannot drift apart.
 */
export function MobileIntegrationsSection({ onNavigate }: MobileIntegrationsSectionProps) {
  const destinations = useAppDestinations("mobileMenu", "integrations");

  if (destinations.length === 0) return null;

  return (
    <div className="space-y-3">
      <div className="text-sm font-medium">Integrations</div>
      <DestinationRows
        destinations={destinations}
        onNavigate={onNavigate}
        pluginTestIdPrefix="plugin-nav-item-"
      />
    </div>
  );
}
