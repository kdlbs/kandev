"use client";

import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { usePathname, useRouter } from "@/lib/routing/client-router";
import { IconPlus, IconRobot, IconSitemap } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import {
  selectOfficeAgentProfiles,
  selectOfficeInboxItems,
} from "@/lib/state/slices/office/selectors";
import { useInOffice } from "@/hooks/use-in-office";
import { cn } from "@/lib/utils";
import type { AgentProfile } from "@/lib/state/slices/office/types";
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

// This section owns no fetching: `useOfficeWorkspaceData` (mounted in
// `AppSidebar`) loads the agents, keyed by workspace, and the selector only
// ever exposes the active workspace's list — nothing to clear on a switch.

function AgentsSectionHeaderAction({ router }: { router: { push: (path: string) => void } }) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-0.5">
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            asChild
            variant="ghost"
            size="icon"
            className="h-5 w-5 cursor-pointer"
            aria-label={t("sidebar:agentTopology")}
          >
            <Link href="/office/workspace/org">
              <IconSitemap className="h-3 w-3 text-muted-foreground/60" />
            </Link>
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t("sidebar:agentTopology")}</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            className="h-5 w-5 cursor-pointer"
            aria-label={t("sidebar:addAgent")}
            onClick={() => router.push("/office/agents")}
          >
            <IconPlus className="h-3 w-3 text-muted-foreground/60" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t("sidebar:addAgent")}</TooltipContent>
      </Tooltip>
    </div>
  );
}

export function AgentsSection({ collapsed }: AgentsSectionProps) {
  const { t } = useTranslation();
  const router = useRouter();
  const inOffice = useInOffice();
  const visibleAgents = useAppStore(selectOfficeAgentProfiles);

  if (!inOffice) return null;

  return (
    <AppSidebarSection
      id={APP_SIDEBAR_SECTION_IDS.agents}
      label={t("common:agents")}
      collapsed={collapsed}
      icon={IconRobot}
      headerAction={<AgentsSectionHeaderAction router={router} />}
      headerActionVisibility="always"
      defaultExpanded
    >
      {visibleAgents.length === 0 ? (
        <p className="px-3 py-2 text-xs text-muted-foreground">{t("sidebar:noAgentsYet")}</p>
      ) : (
        visibleAgents.map((agent) => <AgentRow key={agent.id} agent={agent} />)
      )}
    </AppSidebarSection>
  );
}

function AgentRow({ agent }: { agent: AgentProfile }) {
  const { t } = useTranslation();
  const pathname = usePathname();
  const href = `/office/agents/${agent.id}`;
  const isActive = pathname === href;
  const liveCount = useAppStore((s) => selectActiveSessionsForAgent(s, agent.id));
  const errorCount = useAppStore((s) =>
    selectOfficeInboxItems(s).reduce((acc, item) => {
      if (item.type !== "agent_run_failed") return acc;
      const payloadAgent =
        typeof item.payload?.agent_profile_id === "string" ? item.payload.agent_profile_id : "";
      return payloadAgent === agent.id ? acc + 1 : acc;
    }, 0),
  );
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
          {t("sidebar:pausedBadge")}
        </span>
      ) : null}
      {!isAutoPaused && errorCount > 0 ? (
        <span className="rounded-full bg-red-500/15 text-red-600 dark:text-red-400 px-1.5 py-0.5 text-[10px] font-medium">
          {t("sidebar:runErrors", { count: errorCount })}
        </span>
      ) : null}
      {liveCount > 0 && <LiveAgentIndicator count={liveCount} />}
      {liveCount === 0 && !isAutoPaused && errorCount === 0 && (
        <AgentStatusDot status={agent.status} />
      )}
    </Link>
  );
}
