import { useMemo } from 'react';
import { useAppStore } from '@/components/state-provider';
import type { AgentProfile } from '@/lib/types/http';

export function useSessionModel(
  resolvedSessionId: string | null,
  agentProfileId: string | null | undefined
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

  // Get pending model state for this session
  const pendingModels = useAppStore((state) => state.pendingModel.bySessionId);
  const clearPendingModel = useAppStore((state) => state.clearPendingModel);
  const setActiveModel = useAppStore((state) => state.setActiveModel);
  const pendingModel = resolvedSessionId ? pendingModels[resolvedSessionId] : null;

  return {
    sessionModel,
    pendingModel,
    clearPendingModel,
    setActiveModel,
  };
}
