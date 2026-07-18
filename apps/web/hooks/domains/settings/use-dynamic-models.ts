import { useCallback, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { qk } from "@/lib/query/keys";
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

function capabilityError(
  refreshError: string | null,
  hasNewInitialStatus: boolean,
  initialError: string | null | undefined,
  queryDataError: string | null | undefined,
  queryError: string | null,
) {
  if (refreshError) return refreshError;
  if (hasNewInitialStatus) return initialError ?? null;
  return queryDataError ?? queryError;
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
  const [refreshError, setRefreshError] = useState<string | null>(null);
  const manualRefreshCompleted = useRef(false);
  const initialStatus = useRef({ status: initial.status, error: initial.error });
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
      manualRefreshCompleted.current = true;
      if (response.error) {
        const previous = query.data ?? initialResponse(initial);
        queryClient.setQueryData(qk.settings.dynamicModels(agentName), {
          ...response,
          models: previous.models,
          modes: previous.modes,
          commands: previous.commands,
          current_model_id: previous.current_model_id,
          current_mode_id: previous.current_mode_id,
        });
        setRefreshError(response.error);
      }
    } catch (err) {
      setRefreshError(err instanceof Error ? err.message : "Failed to fetch capabilities");
    }
  }, [agentName, initial, query.data, queryClient, supportsDynamicModels]);

  const capabilities = useMemo<DynamicModelsResponse>(() => {
    return query.data ?? initialResponse(initial);
  }, [initial, query.data]);
  const hasNewInitialStatus =
    !manualRefreshCompleted.current &&
    (initial.status !== initialStatus.current.status ||
      initial.error !== initialStatus.current.error);

  const queryError = query.error instanceof Error ? query.error.message : null;
  return {
    models: capabilities.models ?? [],
    modes: capabilities.modes ?? [],
    commands: capabilities.commands ?? [],
    currentModelId: capabilities.current_model_id,
    currentModeId: capabilities.current_mode_id,
    status: hasNewInitialStatus ? (initial.status ?? "ok") : capabilities.status,
    isLoading: query.isFetching,
    error: capabilityError(
      refreshError,
      hasNewInitialStatus,
      initial.error,
      query.data?.error,
      queryError,
    ),
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
