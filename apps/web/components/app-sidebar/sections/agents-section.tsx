"use client";

import Link from "@/components/routing/app-link";
import { usePathname, useRouter } from "@/lib/routing/client-router";
import { IconPlus, IconRobot, IconSitemap } from "@tabler/icons-react";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { useOfficeAgentsData } from "@/hooks/domains/office/use-office-data";
import { useInOffice } from "@/hooks/use-in-office";
import { officeInboxQueryOptions } from "@/lib/query/query-options/office";
import { cn } from "@/lib/utils";
import type { AgentProfile, InboxItem } from "@/lib/state/slices/office/types";
import { selectActiveSessionsForAgent } from "@/lib/state/slices/session/selectors";
import { AgentAvatar } from "@/app/office/components/agent-avatar";
import { AgentStatusDot } from "@/app/office/agents/components/agent-status-dot";
import { LiveAgentIndicator } from "@/app/office/agents/components/live-agent-indicator";
import {
  APP_SIDEBAR_SECTION_IDS,
  SIDEBAR_ITEM_ACTIVE,
  SIDEBAR_ITEM_INACTIVE,
} from "../app-sidebar-constants";
import { AppSidebarSection } from "../app-sidebar-section";

type AgentsSectionProps = {
  collapsed: boolean;
};

export function AgentsSection({ collapsed }: AgentsSectionProps) {
  const router = useRouter();
  const inOffice = useInOffice();
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const agentsQuery = useOfficeAgentsData(inOffice ? workspaceId : null);
  const inboxQuery = useQuery(officeInboxQueryOptions(inOffice ? (workspaceId ?? "") : ""));
  const agents = agentsQuery.data?.agents ?? [];
  const inboxItems = inboxQuery.data?.items ?? [];

  if (!inOffice) return null;

  const headerAction = (
    <div className="flex items-center gap-0.5">
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            asChild
            variant="ghost"
            size="icon"
            className="h-5 w-5 cursor-pointer"
            aria-label="Agent topology"
          >
            <Link href="/office/workspace/org">
              <IconSitemap className="h-3 w-3 text-muted-foreground/60" />
            </Link>
          </Button>
        </TooltipTrigger>
        <TooltipContent>Agent topology</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            className="h-5 w-5 cursor-pointer"
            aria-label="Add agent"
            onClick={() => router.push("/office/agents")}
          >
            <IconPlus className="h-3 w-3 text-muted-foreground/60" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>Add agent</TooltipContent>
      </Tooltip>
    </div>
  );

  return (
    <AppSidebarSection
      id={APP_SIDEBAR_SECTION_IDS.agents}
      label="Agents"
      collapsed={collapsed}
      icon={IconRobot}
      headerAction={headerAction}
      headerActionVisibility="always"
      defaultExpanded
    >
      {agents.length === 0 ? (
        <p className="px-3 py-2 text-xs text-muted-foreground">No agents yet</p>
      ) : (
        agents.map((agent) => <AgentRow key={agent.id} agent={agent} inboxItems={inboxItems} />)
      )}
    </AppSidebarSection>
  );
}

function AgentRow({ agent, inboxItems }: { agent: AgentProfile; inboxItems: InboxItem[] }) {
  const pathname = usePathname();
  const href = `/office/agents/${agent.id}`;
  const isActive = pathname === href;
  const liveCount = useAppStore((s) => selectActiveSessionsForAgent(s, agent.id));
  const errorCount = inboxItems.reduce((acc, item) => {
    if (item.type !== "agent_run_failed") return acc;
    const payloadAgent =
      typeof item.payload?.agent_profile_id === "string" ? item.payload.agent_profile_id : "";
    return payloadAgent === agent.id ? acc + 1 : acc;
  }, 0);
  const isAutoPaused = (agent.pauseReason ?? "").startsWith("Auto-paused:");

  return (
    <Link
      href={href}
      className={cn(
        "flex items-center gap-2.5 px-2.5 py-1.5 text-[13px] font-medium rounded-md cursor-pointer",
        isActive ? SIDEBAR_ITEM_ACTIVE : SIDEBAR_ITEM_INACTIVE,
      )}
    >
      <AgentAvatar role={agent.role} name={agent.name} size="sm" />
      <span className="flex-1 truncate">{agent.name}</span>
      {isAutoPaused ? (
        <span
          data-testid="sidebar-agent-paused-badge"
          title={agent.pauseReason}
          className="rounded-full bg-red-500/15 text-red-600 dark:text-red-400 px-1.5 py-0.5 text-[10px] font-medium"
        >
          paused
        </span>
      ) : null}
      {!isAutoPaused && errorCount > 0 ? (
        <span className="rounded-full bg-red-500/15 text-red-600 dark:text-red-400 px-1.5 py-0.5 text-[10px] font-medium">
          {errorCount} error{errorCount === 1 ? "" : "s"}
        </span>
      ) : null}
      {liveCount > 0 && <LiveAgentIndicator count={liveCount} />}
      {liveCount === 0 && !isAutoPaused && errorCount === 0 && (
        <AgentStatusDot status={agent.status} />
      )}
    </Link>
  );
}
