"use client";

import Link from "@/components/routing/app-link";
import { useTranslation } from "react-i18next";
import { usePathname } from "@/lib/routing/client-router";
import { IconPlugConnected } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppDestinations } from "@/hooks/use-app-destinations";
import type { DestinationIcon, ResolvedDestination } from "@/lib/navigation/types";
import { cn } from "@/lib/utils";
import {
  APP_SIDEBAR_SECTION_IDS,
  SIDEBAR_ITEM_ACTIVE,
  SIDEBAR_ITEM_INACTIVE,
} from "../app-sidebar-constants";
import { AppSidebarSection } from "../app-sidebar-section";

type IntegrationsSectionProps = {
  collapsed: boolean;
};

const MAX_HEADER_SHORTCUTS = 4;

function IntegrationHeaderShortcuts({ links }: { links: ResolvedDestination[] }) {
  return (
    <div className="flex items-center gap-0.5">
      {links.slice(0, MAX_HEADER_SHORTCUTS).map(({ id, label, href, icon: Icon }) => (
        <Tooltip key={id}>
          <TooltipTrigger asChild>
            <Link
              href={href}
              aria-label={label}
              data-testid="integration-header-shortcut"
              className="flex h-5 w-5 items-center justify-center rounded text-muted-foreground/70 hover:bg-muted/60 hover:text-foreground cursor-pointer transition-colors"
            >
              <Icon className="h-3.5 w-3.5" />
            </Link>
          </TooltipTrigger>
          <TooltipContent side="right">{label}</TooltipContent>
        </Tooltip>
      ))}
    </div>
  );
}

type IntegrationRowProps = {
  href: string;
  label: string;
  icon: DestinationIcon;
  active: boolean;
  testId?: string;
};

function IntegrationRow({ href, label, icon: Icon, active, testId }: IntegrationRowProps) {
  return (
    <Link
      href={href}
      data-testid={testId}
      className={cn(
        "flex items-center gap-2.5 px-2.5 py-1.5 text-[13px] font-medium rounded-md cursor-pointer",
        active ? SIDEBAR_ITEM_ACTIVE : SIDEBAR_ITEM_INACTIVE,
      )}
    >
      <Icon className="h-4 w-4 shrink-0" />
      <span className="flex-1 truncate">{label}</span>
    </Link>
  );
}

export function IntegrationsSection({ collapsed }: IntegrationsSectionProps) {
  const { t } = useTranslation();
  const pathname = usePathname();
  // First-party integration links and plugin-registered nav items that target
  // this section (`registerNavItem({ section: "integrations" })`) both come from
  // the navigation manifest, already in render order.
  const destinations = useAppDestinations("sidebar", "integrations");
  // Header shortcuts stay first-party: they are a fixed-width strip, and a
  // plugin should not push a configured integration out of it.
  const firstPartyLinks = destinations.filter((destination) => destination.source !== "plugin");

  if (destinations.length === 0) return null;

  return (
    <AppSidebarSection
      id={APP_SIDEBAR_SECTION_IDS.integrations}
      label={t("common:integrations")}
      collapsed={collapsed}
      icon={IconPlugConnected}
      headerAction={
        firstPartyLinks.length > 0 ? (
          <IntegrationHeaderShortcuts links={firstPartyLinks} />
        ) : undefined
      }
      headerActionVisibility="always"
    >
      {destinations.map((destination) => (
        <IntegrationRow
          key={destination.id}
          href={destination.href}
          label={destination.label}
          icon={destination.icon}
          active={pathname === destination.href || pathname.startsWith(`${destination.href}/`)}
          {...(destination.source === "plugin"
            ? { testId: `plugin-nav-item-${destination.pluginItemId ?? destination.id}` }
            : {})}
        />
      ))}
    </AppSidebarSection>
  );
}
