"use client";

import Link from "@/components/routing/app-link";
import { useTranslation } from "react-i18next";
import { usePathname } from "@/lib/routing/client-router";
import { IconPlugConnected, IconSettings } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { workspaceSettingsHref } from "@/lib/settings/workspace-settings-tabs";
import { useAppDestinations } from "@/hooks/use-app-destinations";
import type { DestinationIcon } from "@/lib/navigation/types";
import { cn } from "@/lib/utils";
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
  if (!workspaceId && visibleDestinations.length === 0) return null;

  return (
    <AppSidebarSection
      id={APP_SIDEBAR_SECTION_IDS.integrations}
      label={t("common:integrations")}
      collapsed={collapsed}
      icon={IconPlugConnected}
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
