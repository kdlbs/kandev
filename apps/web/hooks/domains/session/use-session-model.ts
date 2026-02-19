import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import type { AgentProfile } from "@/lib/types/http";

export function useSessionModel(
  resolvedSessionId: string | null,
  agentProfileId: string | null | undefined,
) {
  // Get model from agent profile using agent_profile_id
  const settingsAgents = useAppStore((state) => state.settingsAgents.items);
  const sessionProfile = useMemo(() => {
    if (!agentProfileId) return null;
    for (const agent of settingsAgents) {
      const profile = agent.profiles.find((p: AgentProfile) => p.id === agentProfileId);
      if (profile) return profile;
    }
    return null;
  }, [agentProfileId, settingsAgents]);

  const sessionModel = sessionProfile?.model ?? null;

  // Get active model state for this session (user's selected model)
  const activeModels = useAppStore((state) => state.activeModel.bySessionId);
  const activeModel = resolvedSessionId ? (activeModels[resolvedSessionId] ?? null) : null;

  return {
    sessionModel,
    activeModel,
  };
}
