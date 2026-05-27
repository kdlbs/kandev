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
import { useJiraAvailable } from "@/hooks/domains/jira/use-jira-availability";
import { useLinearAvailable } from "@/hooks/domains/linear/use-linear-availability";
import { useGitHubStatus } from "@/hooks/domains/github/use-github-status";
import { useGitLabAvailable } from "@/hooks/domains/gitlab/use-task-mr";
import {
  getAvailableIntegrationLinks,
  getGitHubIntegrationStatus,
} from "@/components/integrations/integrations-menu";
import { cn } from "@/lib/utils";
import { APP_SIDEBAR_SECTION_IDS } from "../app-sidebar-constants";
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

export function IntegrationsSection({ collapsed }: IntegrationsSectionProps) {
  const pathname = usePathname();
  const { status: githubStatusRaw, loading: githubLoading } = useGitHubStatus();
  const gitlabAvailable = useGitLabAvailable();
  const jiraAvailable = useJiraAvailable();
  const linearAvailable = useLinearAvailable();
  const githubStatus = getGitHubIntegrationStatus(githubStatusRaw, githubLoading);

  const links = getAvailableIntegrationLinks({
    githubReady: githubStatus.ready,
    gitlabReady: gitlabAvailable,
    jiraAvailable,
    linearAvailable,
  });

  if (links.length === 0) return null;

  return (
    <AppSidebarSection
      id={APP_SIDEBAR_SECTION_IDS.integrations}
      label="Integrations"
      collapsed={collapsed}
      icon={IconPlugConnected}
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
              isActive ? "bg-accent text-foreground" : "text-foreground/80 hover:bg-muted/60",
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
