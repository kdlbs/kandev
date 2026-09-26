"use client";

import { useEffect, useId, useRef, useState } from "react";
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
import { IconChevronDown, IconChevronRight, IconPlugConnected } from "@tabler/icons-react";
import { DestinationRows } from "@/components/navigation/destination-rows";
import { useAppDestinations } from "@/hooks/use-app-destinations";
import { useAppStore } from "@/components/state-provider";
import { workspaceSettingsHref } from "@/lib/settings/workspace-settings-tabs";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";

type MobileIntegrationsSectionProps = {
  onNavigate: () => void;
  showSetup?: boolean;
  collapsible?: boolean;
  includePlugins?: boolean;
  omitDestinations?: string[];
};

const HOVER_CLOSE_DELAY_MS = 180;
const INTEGRATIONS_LABEL = "common:integrations";

/**
 * Configured integration destinations, in manifest order. Ids, labels, hrefs and
 * icons come from `lib/navigation/core-destinations.ts`; availability comes from
 * `useNavAvailability` through `useAppDestinations`.
 */
function useIntegrationDestinations() {
  return useAppDestinations("sidebar", "integrations");
}

export function IntegrationsMenu() {
  const { t } = useTranslation();
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
          aria-label={t(INTEGRATIONS_LABEL)}
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
        <DropdownMenuLabel>{t(INTEGRATIONS_LABEL)}</DropdownMenuLabel>
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
export function MobileIntegrationsSection({
  onNavigate,
  showSetup = false,
  collapsible = false,
  includePlugins,
  omitDestinations,
}: MobileIntegrationsSectionProps) {
  const { t } = useTranslation();
  const destinations = useAppDestinations("mobileMenu", "integrations").filter(
    (destination) =>
      (includePlugins !== false || destination.source !== "plugin") &&
      !omitDestinations?.includes(destination.id),
  );
  const [expanded, setExpanded] = useState(false);
  const bodyId = useId();

  if (destinations.length === 0 && !showSetup) return null;

  return (
    <div className={showSetup ? "space-y-2 border-t border-border pt-2" : "space-y-3"}>
      {collapsible ? (
        <Button
          variant="ghost"
          className="cursor-pointer h-11 w-full justify-start gap-2 px-0 text-sm font-medium hover:bg-transparent aria-expanded:bg-transparent"
          aria-expanded={expanded}
          aria-controls={bodyId}
          onClick={() => setExpanded(!expanded)}
        >
          {t(INTEGRATIONS_LABEL)}
          {expanded ? (
            <IconChevronDown className="size-3.5 text-muted-foreground" />
          ) : (
            <IconChevronRight className="size-3.5 text-muted-foreground" />
          )}
        </Button>
      ) : (
        <div className="text-sm font-medium">{t(INTEGRATIONS_LABEL)}</div>
      )}
      {(!collapsible || expanded) && (
        <div
          id={bodyId}
          className={
            collapsible
              ? "divide-y divide-border/60 overflow-hidden rounded-xl border border-border/70 bg-muted/20"
              : "space-y-3"
          }
        >
          <DestinationRows
            destinations={destinations}
            onNavigate={onNavigate}
            pluginTestIdPrefix="plugin-nav-item-"
            showChevron={collapsible}
            className={
              collapsible
                ? "rounded-none border-0 bg-transparent px-3 shadow-none aria-[current=page]:bg-primary/10"
                : undefined
            }
          />
          {showSetup && (
            <MobileIntegrationSettingsLink onNavigate={onNavigate} grouped={collapsible} />
          )}
        </div>
      )}
    </div>
  );
}

function MobileIntegrationSettingsLink({
  onNavigate,
  grouped,
}: {
  onNavigate: () => void;
  grouped: boolean;
}) {
  const { t } = useTranslation();
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  return (
    <Button
      asChild
      variant="outline"
      className={cn(
        "cursor-pointer h-11 w-full justify-start gap-3 px-3 text-sm",
        grouped && "rounded-none border-0 bg-transparent shadow-none",
      )}
    >
      <Link
        href={
          workspaceId ? workspaceSettingsHref(workspaceId, "integrations") : "/settings/workspaces"
        }
        onClick={onNavigate}
        data-testid="mobile-integration-settings"
      >
        <IconPlugConnected className="size-4 shrink-0" />
        <span className="min-w-0 flex-1 truncate text-left">{t("common:integrationSettings")}</span>
        {grouped && (
          <IconChevronRight
            className="size-3.5 shrink-0 text-muted-foreground"
            aria-hidden="true"
          />
        )}
      </Link>
    </Button>
  );
}
