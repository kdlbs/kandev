"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { qk } from "@/lib/query/keys";
import { fetchDynamicModels } from "@/lib/api/domains/settings-api";
import type {
  CommandEntry,
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
  isLoading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
};

function buildStaticInitialData(initial: ModelConfig): DynamicModelsResponse {
  return {
    models: initial.available_models,
    modes: initial.available_modes ?? [],
    commands: initial.available_commands ?? [],
    current_model_id: initial.current_model_id,
    current_mode_id: initial.current_mode_id,
  } as DynamicModelsResponse;
}

function buildCapabilitiesState(
  data: DynamicModelsResponse | undefined,
  initial: ModelConfig,
  isLoading: boolean,
  errorMessage: string | null,
  refresh: () => Promise<void>,
): UseAgentCapabilitiesState {
  return {
    models: data?.models ?? initial.available_models,
    modes: data?.modes ?? initial.available_modes ?? [],
    commands: data?.commands ?? initial.available_commands ?? [],
    currentModelId: data?.current_model_id ?? initial.current_model_id,
    currentModeId: data?.current_mode_id ?? initial.current_mode_id,
    isLoading,
    error: errorMessage,
    refresh,
  };
}

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
  const qc = useQueryClient();

  const query = useQuery({
    queryKey: qk.settings.dynamicModels(agentName ?? ""),
    queryFn: () => fetchDynamicModels(agentName!, { refresh: false }),
    enabled: !!agentName && !!supportsDynamicModels,
    initialData: supportsDynamicModels ? undefined : buildStaticInitialData(initial),
    staleTime: 5 * 60_000,
  });

  const refresh = async () => {
    if (!agentName || !supportsDynamicModels) return;
    await qc.fetchQuery({
      queryKey: qk.settings.dynamicModels(agentName),
      queryFn: () => fetchDynamicModels(agentName, { refresh: true }),
    });
  };

  const errorMessage = query.data?.error ?? query.error?.message ?? null;
  return buildCapabilitiesState(query.data, initial, query.isFetching, errorMessage, refresh);
}
