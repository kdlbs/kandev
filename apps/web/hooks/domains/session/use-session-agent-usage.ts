import { useQuery } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { agentSubscriptionUsageQueryOptions } from "@/lib/query/query-options/settings";
import type { AgentSubscriptionUsage } from "@/lib/types/http";

/** Resolves the session's agent name (e.g. "claude-acp") from live session and settings data. */
export function useSessionAgentName(sessionId: string | null): string | null {
  const profileId = useAppStore((state) =>
    sessionId ? state.taskSessions.items[sessionId]?.agent_profile_id : undefined,
  );
  const { agentProfiles } = useSettingsData(Boolean(profileId));
  const agentName = profileId
    ? agentProfiles.find((profile) => profile.id === profileId)?.agent_name
    : undefined;
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
  const query = useQuery({
    ...agentSubscriptionUsageQueryOptions(true),
    enabled: Boolean(agentName),
    staleTime: 15_000,
  });
  if (!agentName) return null;
  return query.data?.agents.find((agent) => agent.agent_id === agentName) ?? null;
}
