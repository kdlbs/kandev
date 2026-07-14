import { useEffect, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { listAgentSubscriptionUsage } from "@/lib/api";
import type { AgentSubscriptionUsage } from "@/lib/types/http";

// Module-level memo: the last usage listing, served instantly on the next
// tooltip open while a fresh fetch is in flight. One in-flight request is
// shared between concurrent consumers.
let lastAgents: AgentSubscriptionUsage[] | null = null;
let inflight: Promise<AgentSubscriptionUsage[]> | null = null;

function fetchFreshAgentUsage(): Promise<AgentSubscriptionUsage[]> {
  if (!inflight) {
    inflight = listAgentSubscriptionUsage({ cache: "no-store", fresh: true })
      .then((response) => {
        lastAgents = response.agents ?? [];
        return lastAgents;
      })
      .finally(() => {
        inflight = null;
      });
  }
  return inflight;
}

/** Test-only: clear the module-level memo between tests. */
export function resetSessionAgentUsageCacheForTest() {
  lastAgents = null;
  inflight = null;
}

/** Resolves the session's agent name (e.g. "claude-acp") from the store. */
export function useSessionAgentName(sessionId: string | null): string | null {
  const profileId = useAppStore((state) =>
    sessionId ? state.taskSessions.items[sessionId]?.agent_profile_id : undefined,
  );
  const agentName = useAppStore((state) =>
    profileId
      ? state.agentProfiles.items.find((profile) => profile.id === profileId)?.agent_name
      : undefined,
  );
  return agentName ?? null;
}

/**
 * Live subscription usage for the session's agent. Fetches fresh provider
 * data on mount — mount the consuming component lazily (e.g. inside a tooltip
 * content) so hovering triggers the fetch. The previous listing renders
 * immediately while the fresh one is in flight. Returns null while the agent
 * is unknown or has no subscription usage.
 */
export function useSessionAgentUsage(sessionId: string | null): AgentSubscriptionUsage | null {
  const agentName = useSessionAgentName(sessionId);
  const [agents, setAgents] = useState<AgentSubscriptionUsage[]>(lastAgents ?? []);

  useEffect(() => {
    if (!agentName) return;
    let active = true;
    fetchFreshAgentUsage()
      .then((list) => {
        if (active) setAgents(list);
      })
      .catch(() => {
        // Keep whatever we had; the tooltip simply shows no usage rows.
      });
    return () => {
      active = false;
    };
  }, [agentName]);

  if (!agentName) return null;
  return agents.find((agent) => agent.agent_id === agentName) ?? null;
}
