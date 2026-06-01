import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { useSettingsAgents } from "@/hooks/domains/settings/use-settings-reads";
import { sessionModelsQueryOptions } from "@/lib/query/query-options/session-runtime";
import type { AgentProfile } from "@/lib/types/http";

export function useSessionModel(
  resolvedSessionId: string | null,
  agentProfileId: string | null | undefined,
) {
  // Get model from agent profile using agent_profile_id
  const settingsAgents = useSettingsAgents();
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
  const { data: sessionModelsData } = useQuery({
    ...sessionModelsQueryOptions(resolvedSessionId ?? ""),
    enabled: false,
  });
  const acpCurrentModel = resolvedSessionId ? sessionModelsData?.currentModelId || null : null;

  // Get active model state for this session (user's selected model)
  const activeModels = useAppStore((state) => state.activeModel.bySessionId);
  const activeModel = resolvedSessionId ? (activeModels[resolvedSessionId] ?? null) : null;

  const sessionModel = acpCurrentModel || profileModel;

  return {
    sessionModel,
    activeModel,
  };
}
