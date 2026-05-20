"use client";

import { useEffect, useMemo, useRef } from "react";
import { useAvailableAgents } from "@/hooks/domains/settings/use-available-agents";
import { useAppStore } from "@/components/state-provider";
import { listAgents } from "@/lib/api";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";
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
  const setSettingsAgents = useAppStore((state) => state.setSettingsAgents);
  const setAgentProfiles = useAppStore((state) => state.setAgentProfiles);
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
        setSettingsAgents(response.agents);
        setAgentProfiles(
          response.agents.flatMap((item) =>
            item.profiles.map((itemProfile) => toAgentProfileOption(item, itemProfile)),
          ),
        );
      })
      .catch(() => {
        refreshKeyRef.current = null;
      });

    return () => {
      cancelled = true;
    };
  }, [agentKey, profile, profileId, setAgentProfiles, setSettingsAgents]);

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
