"use client";

import { useCallback, useEffect } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { IconPlus, IconRobot } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useOfficeRefetch } from "@/hooks/use-office-refetch";
import { listAgentProfiles } from "@/lib/api/domains/office-api";
import { cn } from "@/lib/utils";
import type { AgentProfile } from "@/lib/state/slices/office/types";
import { selectActiveSessionsForAgent } from "@/lib/state/slices/session/selectors";
import { AgentAvatar } from "@/app/office/components/agent-avatar";
import { AgentStatusDot } from "@/app/office/agents/components/agent-status-dot";
import { LiveAgentIndicator } from "@/app/office/agents/components/live-agent-indicator";
import { APP_SIDEBAR_SECTION_IDS } from "../app-sidebar-constants";
import { AppSidebarSection } from "../app-sidebar-section";

type AgentsSectionProps = {
  collapsed: boolean;
};

export function AgentsSection({ collapsed }: AgentsSectionProps) {
  const router = useRouter();
  const officeEnabled = useFeature("office");
  const agents = useAppStore((s) => s.office.agentProfiles);
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const setOfficeAgentProfiles = useAppStore((s) => s.setOfficeAgentProfiles);

  const refetchAgents = useCallback(async () => {
    if (!workspaceId || !officeEnabled) return;
    const res = await listAgentProfiles(workspaceId).catch(() => ({ agents: [] }));
    setOfficeAgentProfiles(res.agents ?? []);
  }, [workspaceId, officeEnabled, setOfficeAgentProfiles]);

  useEffect(() => {
    refetchAgents();
  }, [refetchAgents]);

  useOfficeRefetch("agents", refetchAgents);

  if (!officeEnabled) return null;

  const headerAction = (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="h-5 w-5 cursor-pointer"
          onClick={() => router.push("/office/agents")}
        >
          <IconPlus className="h-3 w-3 text-muted-foreground/60" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>Add agent</TooltipContent>
    </Tooltip>
  );

  return (
    <AppSidebarSection
      id={APP_SIDEBAR_SECTION_IDS.agents}
      label="Agents"
      collapsed={collapsed}
      icon={IconRobot}
      headerAction={headerAction}
    >
      {agents.length === 0 ? (
        <p className="px-3 py-2 text-xs text-muted-foreground">No agents yet</p>
      ) : (
        agents.map((agent) => <AgentRow key={agent.id} agent={agent} />)
      )}
    </AppSidebarSection>
  );
}

function AgentRow({ agent }: { agent: AgentProfile }) {
  const pathname = usePathname();
  const href = `/office/agents/${agent.id}`;
  const isActive = pathname === href;
  const liveCount = useAppStore((s) => selectActiveSessionsForAgent(s, agent.id));
  const errorCount = useAppStore((s) =>
    s.office.inboxItems.reduce((acc, item) => {
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
        isActive ? "bg-accent text-foreground" : "text-foreground/80 hover:bg-muted/60",
      )}
    >
      <AgentAvatar role={agent.role} name={agent.name} size="sm" />
      <span className="flex-1 truncate">{agent.name}</span>
      {isAutoPaused ? (
        <span
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
