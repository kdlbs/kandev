"use client";

import { useCallback, useEffect } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useAppStore } from "@/components/state-provider";
import { useOfficeRefetch } from "@/hooks/use-office-refetch";
import { listAgentProfiles } from "@/lib/api/domains/office-api";
import { cn } from "@/lib/utils";
import type { AgentProfile } from "@/lib/state/slices/office/types";
import { selectActiveSessionsForAgent } from "@/lib/state/slices/session/selectors";
import { SidebarCollapsibleSection } from "./sidebar-collapsible-section";
import { AgentAvatar } from "./agent-avatar";
import { AgentStatusDot } from "../agents/components/agent-status-dot";
import { LiveAgentIndicator } from "../agents/components/live-agent-indicator";

export function SidebarAgentsList() {
  const router = useRouter();
  const agents = useAppStore((s) => s.office.agentProfiles);
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const setOfficeAgentProfiles = useAppStore((s) => s.setOfficeAgentProfiles);

  // Refetch agents on mount and on WS "agents" events. This ensures the
  // sidebar (and any page that reads agentProfiles from the store, such
  // as the org chart and agent detail layout) recovers from stale SSR
  // hydration without waiting for a user action or WS event to arrive.
  const refetchAgents = useCallback(async () => {
    if (!workspaceId) return;
    const res = await listAgentProfiles(workspaceId).catch(() => ({ agents: [] }));
    setOfficeAgentProfiles(res.agents ?? []);
  }, [workspaceId, setOfficeAgentProfiles]);

  useEffect(() => {
    refetchAgents();
  }, [refetchAgents]);

  useOfficeRefetch("agents", refetchAgents);

  return (
    <SidebarCollapsibleSection label="Agents" onAdd={() => router.push("/office/agents")}>
      {agents.length === 0 ? (
        <p className="px-3 py-2 text-xs text-muted-foreground">No agents yet</p>
      ) : (
        agents.map((agent) => <SidebarAgentRow key={agent.id} agent={agent} />)
      )}
    </SidebarCollapsibleSection>
  );
}

// Row is its own component so each one can subscribe to the live-session
// count selector independently. For typical office workspaces (under ~20
// agents) the per-row subscription is cheap; if the list ever grows much
// larger, refactor into a single bulk selector that returns a map keyed
// by agent id.
function SidebarAgentRow({ agent }: { agent: AgentProfile }) {
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
        "flex items-center gap-2.5 px-3 py-1.5 text-[13px] font-medium rounded-md cursor-pointer",
        isActive ? "bg-accent text-foreground" : "text-foreground/80 hover:bg-muted/60",
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
        <span
          data-testid="sidebar-agent-error-count"
          className="rounded-full bg-red-500/15 text-red-600 dark:text-red-400 px-1.5 py-0.5 text-[10px] font-medium"
        >
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
