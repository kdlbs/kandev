"use client";

import Link from "@/components/routing/app-link";
import { useTranslation } from "react-i18next";
import { usePathname } from "@/lib/routing/client-router";
import { IconPlugConnected, IconSettings } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { workspaceSettingsHref } from "@/lib/settings/workspace-settings-tabs";
import { useAppDestinations } from "@/hooks/use-app-destinations";
import type { DestinationIcon } from "@/lib/navigation/types";
import { cn } from "@/lib/utils";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import {
  APP_SIDEBAR_SECTION_IDS,
  SIDEBAR_ITEM_ACTIVE,
  SIDEBAR_ITEM_INACTIVE,
} from "../app-sidebar-constants";
import { AppSidebarSection } from "../app-sidebar-section";

type IntegrationsSectionProps = {
  collapsed: boolean;
  includePluginItems?: boolean;
};

type IntegrationRowProps = {
  href: string;
  label: string;
  icon: DestinationIcon;
  active: boolean;
  testId?: string;
};

const FINE_POINTER_HEADER_CAPACITY = 4;
const COARSE_POINTER_HEADER_CAPACITY = 2;

function IntegrationHeaderShortcuts({
  links,
  pathname,
  capacity,
}: {
  links: ReturnType<typeof useAppDestinations>;
  pathname: string;
  capacity: number;
}) {
  return (
    <div className="flex items-center gap-0.5" data-testid="integration-header-shortcuts">
      {links.slice(0, capacity).map(({ id, label, href, icon: Icon }) => (
        <Tooltip key={id}>
          <TooltipTrigger asChild>
            <Link
              href={href}
              aria-label={label}
              aria-current={
                pathname === href || pathname.startsWith(`${href}/`) ? "page" : undefined
              }
              data-testid={`integration-header-shortcut-${id}`}
              data-destination-id={id}
              className="flex size-5 shrink-0 cursor-pointer items-center justify-center rounded text-muted-foreground/70 transition-colors hover:bg-muted/60 hover:text-foreground [@media(pointer:coarse)]:size-11"
            >
              <Icon className="size-3.5" aria-hidden="true" />
            </Link>
          </TooltipTrigger>
          <TooltipContent side="right">{label}</TooltipContent>
        </Tooltip>
      ))}
    </div>
  );
}

function IntegrationRow({ href, label, icon: Icon, active, testId }: IntegrationRowProps) {
  return (
    <Link
      href={href}
      data-testid={testId}
      aria-current={active ? "page" : undefined}
      className={cn(
        "flex min-h-8 items-center gap-2.5 px-2.5 py-1.5 [@media(pointer:coarse)]:min-h-11 text-[13px] font-medium rounded-md cursor-pointer",
        active ? SIDEBAR_ITEM_ACTIVE : SIDEBAR_ITEM_INACTIVE,
      )}
    >
      <Icon className="h-4 w-4 shrink-0" />
      <span className="flex-1 truncate">{label}</span>
    </Link>
  );
}

export function IntegrationsSection({
  collapsed,
  includePluginItems = true,
}: IntegrationsSectionProps) {
  const { t } = useTranslation();
  const pathname = usePathname();
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  // First-party integration links and plugin-registered nav items that target
  // this section (`registerNavItem({ section: "integrations" })`) both come from
  // the navigation manifest, already in render order.
  const destinations = useAppDestinations("sidebar", "integrations");
  const visibleDestinations = includePluginItems
    ? destinations
    : destinations.filter((destination) => destination.source !== "plugin");
  const firstPartyDestinations = visibleDestinations.filter(
    (destination) => destination.source !== "plugin",
  );
  const { isFinePointer } = useResponsiveBreakpoint();
  const shortcutCapacity = isFinePointer
    ? FINE_POINTER_HEADER_CAPACITY
    : COARSE_POINTER_HEADER_CAPACITY;
  if (!workspaceId && visibleDestinations.length === 0) return null;

  return (
    <AppSidebarSection
      id={APP_SIDEBAR_SECTION_IDS.integrations}
      label={t("common:integrations")}
      collapsed={collapsed}
      icon={IconPlugConnected}
      headerAction={
        firstPartyDestinations.length > 0 ? (
          <IntegrationHeaderShortcuts
            links={firstPartyDestinations}
            pathname={pathname}
            capacity={shortcutCapacity}
          />
        ) : undefined
      }
      headerActionVisibility="always"
      presentation="navigation"
    >
      {visibleDestinations.map((destination) => (
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
      {workspaceId && (
        <IntegrationRow
          href={workspaceSettingsHref(workspaceId, "integrations")}
          label={t("common:integrationSettings")}
          icon={IconSettings}
          active={pathname === workspaceSettingsHref(workspaceId, "integrations")}
        />
      )}
    </AppSidebarSection>
  );
}
