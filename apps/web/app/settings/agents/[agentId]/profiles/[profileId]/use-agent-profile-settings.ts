'use client';

import { useMemo } from 'react';
import { useAvailableAgents } from '@/hooks/domains/settings/use-available-agents';
import { useAppStore } from '@/components/state-provider';
import type { Agent, AgentProfile, ModelConfig, AvailableAgent, PermissionSetting } from '@/lib/types/http';

type AgentProfileSettingsResult = {
  agent: Agent | null;
  profile: AgentProfile | null;
  modelConfig: ModelConfig;
  permissionSettings: Record<string, PermissionSetting>;
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

  const availableAgent = useMemo(() => {
    return availableAgents.find((item: AvailableAgent) => item.name === agent?.name) ?? null;
  }, [availableAgents, agent?.name]);

  const modelConfig = useMemo(() => {
    return availableAgent?.model_config ?? { default_model: '', available_models: [] };
  }, [availableAgent]);

  const permissionSettings = useMemo(() => {
    return availableAgent?.permission_settings ?? {};
  }, [availableAgent]);

  return { agent, profile, modelConfig, permissionSettings };
}
