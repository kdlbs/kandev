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

  const profileModel = sessionProfile?.model ?? null;

  // ACP agents report their actual current model via session_models events.
  // Use that as the authoritative "session model" so comparisons in useMessageHandler
  // use the same ID space (ACP IDs) as the model selector dropdown.
  const acpCurrentModel = useAppStore((state) =>
    resolvedSessionId
      ? state.sessionModels.bySessionId[resolvedSessionId]?.currentModelId || null
      : null,
  );

  // Get active model state for this session (user's selected model)
  const activeModels = useAppStore((state) => state.activeModel.bySessionId);
  const activeModel = resolvedSessionId ? (activeModels[resolvedSessionId] ?? null) : null;

  const sessionModel = acpCurrentModel || profileModel;

  return {
    sessionModel,
    activeModel,
  };
}
