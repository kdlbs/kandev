'use client';

import { useMemo } from 'react';
import { useAvailableAgents } from '@/hooks/domains/settings/use-available-agents';
import { useAppStore } from '@/components/state-provider';
import type { Agent, AgentProfile, ModelConfig, AvailableAgent } from '@/lib/types/http';

type AgentProfileSettingsResult = {
  agent: Agent | null;
  profile: AgentProfile | null;
  modelConfig: ModelConfig;
};

export function useAgentProfileSettings(
  agentKey: string,
  profileId: string
): AgentProfileSettingsResult {
  const settingsAgents = useAppStore((state) => state.settingsAgents.items);
  const availableAgents = useAvailableAgents().items;

  const agent = useMemo(() => {
    return settingsAgents.find((item: Agent) => item.name === agentKey) ?? null;
  }, [agentKey, settingsAgents]);

  const profile = useMemo(() => {
    return agent?.profiles.find((item: AgentProfile) => item.id === profileId) ?? null;
  }, [agent?.profiles, profileId]);

  const modelConfig = useMemo(() => {
    const availableAgent = availableAgents.find((item: AvailableAgent) => item.name === agent?.name);
    return availableAgent?.model_config ?? { default_model: '', available_models: [] };
  }, [availableAgents, agent?.name]);

  return { agent, profile, modelConfig };
}
