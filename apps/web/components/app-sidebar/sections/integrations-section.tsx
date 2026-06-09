"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  IconBrandGithub,
  IconBrandGitlab,
  IconHexagon,
  IconPlugConnected,
  IconTicket,
} from "@tabler/icons-react";
import type { Icon as TablerIcon } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useConfiguredIntegrationLinks } from "@/components/integrations/integrations-menu";
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

const INTEGRATION_ICONS: Record<string, TablerIcon> = {
  github: IconBrandGithub,
  gitlab: IconBrandGitlab,
  jira: IconTicket,
  linear: IconHexagon,
};

const MAX_HEADER_SHORTCUTS = 4;

type ConfiguredIntegrationLink = ReturnType<typeof useConfiguredIntegrationLinks>[number];

function IntegrationHeaderShortcuts({ links }: { links: ConfiguredIntegrationLink[] }) {
  return (
    <div className="flex items-center gap-0.5">
      {links.slice(0, MAX_HEADER_SHORTCUTS).map(({ id, label, href }) => {
        const Icon = INTEGRATION_ICONS[id] ?? IconPlugConnected;
        return (
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
        );
      })}
    </div>
  );
}

export function IntegrationsSection({ collapsed }: IntegrationsSectionProps) {
  const pathname = usePathname();
  const links = useConfiguredIntegrationLinks();

  if (links.length === 0) return null;

  return (
    <AppSidebarSection
      id={APP_SIDEBAR_SECTION_IDS.integrations}
      label="Integrations"
      collapsed={collapsed}
      icon={IconPlugConnected}
      headerAction={<IntegrationHeaderShortcuts links={links} />}
      headerActionVisibility="always"
    >
      {links.map(({ id, label, href }) => {
        const Icon = INTEGRATION_ICONS[id] ?? IconPlugConnected;
        const isActive = pathname === href || pathname.startsWith(`${href}/`);
        return (
          <Link
            key={id}
            href={href}
            className={cn(
              "flex items-center gap-2.5 px-2.5 py-1.5 text-[13px] font-medium rounded-md cursor-pointer",
              isActive ? SIDEBAR_ITEM_ACTIVE : SIDEBAR_ITEM_INACTIVE,
            )}
          >
            <Icon className="h-4 w-4 shrink-0" />
            <span className="flex-1 truncate">{label}</span>
          </Link>
        );
      })}
    </AppSidebarSection>
  );
}
