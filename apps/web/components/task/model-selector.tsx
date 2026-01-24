'use client';

import { IconCpu } from '@tabler/icons-react';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@kandev/ui/select';
import { useAppStore } from '@/components/state-provider';
import { useAvailableAgents } from '@/hooks/domains/settings/use-available-agents';
import { useSettingsData } from '@/hooks/domains/settings/use-settings-data';
import type { Agent, AgentProfile, AvailableAgent } from '@/lib/types/http';

type ModelSelectorProps = {
  sessionId: string | null;
};

export function ModelSelector({ sessionId }: ModelSelectorProps) {
  // Ensure settings data (agents with profiles) is loaded
  useSettingsData(true);

  const settingsAgents = useAppStore((state) => state.settingsAgents.items);
  const taskSessions = useAppStore((state) => state.taskSessions.items);
  const activeModels = useAppStore((state) => state.activeModel.bySessionId);
  const setActiveModel = useAppStore((state) => state.setActiveModel);

  // Ensure available agents are loaded (contains model_config)
  // Note: We don't block on this loading - it only affects the dropdown options
  const { items: availableAgents } = useAvailableAgents();

  // Find the session from taskSessions
  const session = sessionId ? (taskSessions[sessionId] ?? null) : null;

  // Get model from agent_profile_snapshot (guaranteed to be non-empty since model is required)
  const snapshotModel =
    session?.agent_profile_snapshot && typeof session.agent_profile_snapshot === 'object'
      ? (session.agent_profile_snapshot as Record<string, unknown>).model
      : null;
  const snapshotModelStr = typeof snapshotModel === 'string' && snapshotModel ? snapshotModel : null;

  // Resolve the agent profile from settings (for getting available models list)
  let sessionProfile: { profile: (typeof settingsAgents)[0]['profiles'][0]; agent: (typeof settingsAgents)[0] } | null = null;
  if (session?.agent_profile_id) {
    for (const agent of settingsAgents as Agent[]) {
      const profile = agent.profiles.find((p: AgentProfile) => p.id === session.agent_profile_id);
      if (profile) {
        sessionProfile = { profile, agent };
        break;
      }
    }
  }

  // Get available models from the agent discovery data
  // Agent.name is the registry ID (e.g., "claude-code") which matches AvailableAgent.name
  let availableModels: { id: string; name: string; provider: string; context_window: number; is_default: boolean }[] = [];
  if (sessionProfile?.agent) {
    const agentName = sessionProfile.agent.name;
    const availableAgent = availableAgents.find((a: AvailableAgent) => a.name === agentName);
    if (availableAgent?.model_config?.available_models) {
      availableModels = availableAgent.model_config.available_models;
    }
  }

  // Priority: activeModel (user selection) > snapshot model (from backend)
  const activeModel = sessionId ? (activeModels[sessionId] || null) : null;
  const currentModel = activeModel || snapshotModelStr;

  // Ensure current model is included in the list (even if not in available models)
  const modelOptions = [...availableModels];
  if (currentModel && !modelOptions.some((m) => m.id === currentModel)) {
    modelOptions.unshift({
      id: currentModel,
      name: currentModel,
      provider: 'unknown',
      context_window: 0,
      is_default: false,
    });
  }

  const handleModelChange = (modelId: string) => {
    if (!sessionId) return;
    // Immediately update the active model - the actual switch happens on next message
    setActiveModel(sessionId, modelId);
  };

  // If no session or no model available, show disabled placeholder
  if (!sessionId || !currentModel) {
    return (
      <Select disabled>
        <SelectTrigger className="w-[180px] cursor-not-allowed opacity-50">
          <SelectValue placeholder="No model" />
        </SelectTrigger>
      </Select>
    );
  }

  return (
    <Select value={currentModel} onValueChange={handleModelChange}>
      <SelectTrigger className="w-[180px] cursor-pointer">
        <div className="flex items-center gap-1.5">
          <IconCpu className="h-3.5 w-3.5 text-muted-foreground" />
          <SelectValue placeholder="Select model" />
        </div>
      </SelectTrigger>
      <SelectContent>
        {modelOptions.map((model) => (
          <SelectItem key={model.id} value={model.id}>
            {model.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

