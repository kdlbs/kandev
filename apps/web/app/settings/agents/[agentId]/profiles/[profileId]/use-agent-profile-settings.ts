"use client";

import { useMemo } from "react";
import { useAvailableAgents } from "@/hooks/domains/settings/use-available-agents";
import { useAppStore } from "@/components/state-provider";
import type {
  Agent,
  AgentProfile,
  ModelConfig,
  AvailableAgent,
  PermissionSetting,
  PassthroughConfig,
} from "@/lib/types/http";

type AgentProfileSettingsResult = {
  agent: Agent | null;
  profile: AgentProfile | null;
  modelConfig: ModelConfig;
  permissionSettings: Record<string, PermissionSetting>;
  passthroughConfig: PassthroughConfig | null;
};

export function useAgentProfileSettings(
  agentKey: string,
  profileId: string,
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
    const raw = availableAgent?.model_config;
    // Defensive normalization: the backend may marshal nil slices as null.
    // Ensure arrays are always arrays so consumers can call .some()/.map().
    return {
      default_model: raw?.default_model ?? "",
      available_models: raw?.available_models ?? [],
      current_model_id: raw?.current_model_id,
      available_modes: raw?.available_modes ?? [],
      current_mode_id: raw?.current_mode_id,
      supports_dynamic_models: raw?.supports_dynamic_models ?? false,
      status: raw?.status,
      error: raw?.error,
    };
  }, [availableAgent]);

  const permissionSettings = useMemo(() => {
    return availableAgent?.permission_settings ?? {};
  }, [availableAgent]);

  const passthroughConfig = useMemo(() => {
    return availableAgent?.passthrough_config ?? null;
  }, [availableAgent]);

  return { agent, profile, modelConfig, permissionSettings, passthroughConfig };
}
