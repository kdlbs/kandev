"use client";

import { useEffect, useMemo, useRef } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useAvailableAgents } from "@/hooks/domains/settings/use-available-agents";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { listAgents } from "@/lib/api";
import { qk } from "@/lib/query/keys";
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

const EMPTY_MODEL_CONFIG: ModelConfig = {
  default_model: "",
  available_models: [],
  available_modes: [],
  available_commands: [],
  config_options: [],
  supports_dynamic_models: false,
};

function normalizeModelConfig(raw: AvailableAgent["model_config"] | undefined): ModelConfig {
  if (!raw) return EMPTY_MODEL_CONFIG;
  return {
    ...raw,
    default_model: raw.default_model ?? "",
    available_models: raw.available_models ?? [],
    available_modes: raw.available_modes ?? [],
    available_commands: raw.available_commands ?? [],
    config_options: raw.config_options ?? [],
    supports_dynamic_models: raw.supports_dynamic_models ?? false,
  };
}

export function useAgentProfileSettings(
  agentKey: string,
  profileId: string,
): AgentProfileSettingsResult {
  const queryClient = useQueryClient();
  const { settingsAgents } = useSettingsData(true);
  const availableAgents = useAvailableAgents().items;
  const refreshKeyRef = useRef<string | null>(null);

  const agent = useMemo(() => {
    return settingsAgents.find((item: Agent) => item.name === agentKey) ?? null;
  }, [agentKey, settingsAgents]);

  const profile = useMemo(() => {
    return agent?.profiles.find((item: AgentProfile) => item.id === profileId) ?? null;
  }, [agent?.profiles, profileId]);

  useEffect(() => {
    if (profile) {
      refreshKeyRef.current = null;
      return;
    }

    const refreshKey = `${agentKey}:${profileId}`;
    if (refreshKeyRef.current === refreshKey) return;
    refreshKeyRef.current = refreshKey;

    let cancelled = false;
    listAgents({ cache: "no-store" })
      .then((response) => {
        if (cancelled) return;
        queryClient.setQueryData(qk.settings.agents(), response);
      })
      .catch(() => {
        refreshKeyRef.current = null;
      });

    return () => {
      cancelled = true;
    };
  }, [agentKey, profile, profileId, queryClient]);

  const availableAgent = useMemo(() => {
    return availableAgents.find((item: AvailableAgent) => item.name === agent?.name) ?? null;
  }, [availableAgents, agent?.name]);

  const modelConfig = useMemo(() => {
    // Defensive normalization: the backend may marshal nil slices as null.
    // Ensure arrays are always arrays so consumers can call .some()/.map().
    return normalizeModelConfig(availableAgent?.model_config);
  }, [availableAgent]);

  const permissionSettings = useMemo(() => {
    return availableAgent?.permission_settings ?? {};
  }, [availableAgent]);

  const passthroughConfig = useMemo(() => {
    return availableAgent?.passthrough_config ?? null;
  }, [availableAgent]);

  return { agent, profile, modelConfig, permissionSettings, passthroughConfig };
}
