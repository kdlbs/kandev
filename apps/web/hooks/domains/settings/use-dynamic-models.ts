import { useCallback, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { dynamicModelsQueryOptions } from "@/lib/query/query-options/settings";
import type {
  CommandEntry,
  CapabilityStatus,
  ModeEntry,
  ModelEntry,
  DynamicModelsResponse,
  ModelConfig,
} from "@/lib/types/http";

type UseAgentCapabilitiesState = {
  models: ModelEntry[];
  modes: ModeEntry[];
  commands: CommandEntry[];
  currentModelId: string | undefined;
  currentModeId: string | undefined;
  status: CapabilityStatus | undefined;
  isLoading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
};

/**
 * useAgentCapabilities fetches the full ACP probe cache for an agent
 * (models, modes, current defaults) and keeps it in sync. Refresh triggers
 * a live re-probe against the host utility and updates both models and
 * modes atomically — so the profile page's refresh button covers the whole
 * agent surface, not just models.
 */
export function useAgentCapabilities(
  agentName: string | undefined,
  initial: ModelConfig,
): UseAgentCapabilitiesState {
  const supportsDynamicModels = initial.supports_dynamic_models;
  const [refreshError, setRefreshError] = useState<string | null>(null);
  const queryClient = useQueryClient();
  const query = useQuery({
    ...dynamicModelsQueryOptions(agentName ?? ""),
    enabled: supportsDynamicModels && Boolean(agentName),
  });

  const refresh = useCallback(async () => {
    setRefreshError(null);
    try {
      if (!agentName || !supportsDynamicModels) {
        return;
      }
      const response = await queryClient.fetchQuery({
        ...dynamicModelsQueryOptions(agentName, { refresh: true }),
        staleTime: 0,
      });
      if (response.error) {
        setRefreshError(response.error);
      }
    } catch (err) {
      setRefreshError(err instanceof Error ? err.message : "Failed to fetch capabilities");
    }
  }, [agentName, queryClient, supportsDynamicModels]);

  const capabilities = useMemo<DynamicModelsResponse>(() => {
    if (query.data?.error) return initialResponse(initial);
    return query.data ?? initialResponse(initial);
  }, [initial, query.data]);

  const queryError = query.error instanceof Error ? query.error.message : null;
  return {
    models: capabilities.models ?? [],
    modes: capabilities.modes ?? [],
    commands: capabilities.commands ?? [],
    currentModelId: capabilities.current_model_id,
    currentModeId: capabilities.current_mode_id,
    status: capabilities.status,
    isLoading: query.isFetching,
    error: refreshError ?? query.data?.error ?? queryError,
    refresh,
  };
}

function initialResponse(initial: ModelConfig): DynamicModelsResponse {
  return {
    agent_name: "",
    status: initial.status ?? "ok",
    models: initial.available_models,
    modes: initial.available_modes ?? [],
    commands: initial.available_commands ?? [],
    current_model_id: initial.current_model_id,
    current_mode_id: initial.current_mode_id,
    error: initial.error ?? null,
  };
}
