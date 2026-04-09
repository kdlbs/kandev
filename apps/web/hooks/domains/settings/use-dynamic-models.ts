import { useState, useEffect, useCallback } from "react";
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
  const [models, setModels] = useState<ModelEntry[]>(initial.available_models);
  const [modes, setModes] = useState<ModeEntry[]>(initial.available_modes ?? []);
  const [commands, setCommands] = useState<CommandEntry[]>(initial.available_commands ?? []);
  const [currentModelId, setCurrentModelId] = useState<string | undefined>(
    initial.current_model_id,
  );
  const [currentModeId, setCurrentModeId] = useState<string | undefined>(initial.current_mode_id);
  const [isLoading, setIsLoading] = useState(supportsDynamicModels && !!agentName);
  const [error, setError] = useState<string | null>(null);

  const fetchCaps = useCallback(
    async (forceRefresh: boolean) => {
      if (!agentName || !supportsDynamicModels) {
        return;
      }
      setIsLoading(true);
      setError(null);
      try {
        const response: DynamicModelsResponse = await fetchDynamicModels(agentName, {
          refresh: forceRefresh,
        });
        if (response.error) {
          setError(response.error);
          return;
        }
        setModels(response.models ?? []);
        setModes(response.modes ?? []);
        setCommands(response.commands ?? []);
        setCurrentModelId(response.current_model_id);
        setCurrentModeId(response.current_mode_id);
      } catch (err) {
        setError(err instanceof Error ? err.message : "Failed to fetch capabilities");
      } finally {
        setIsLoading(false);
      }
    },
    [agentName, supportsDynamicModels],
  );

  useEffect(() => {
    if (supportsDynamicModels && agentName) {
      void fetchCaps(false);
    }
  }, [agentName, supportsDynamicModels, fetchCaps]);

  const refresh = useCallback(() => fetchCaps(true), [fetchCaps]);

  return { models, modes, commands, currentModelId, currentModeId, isLoading, error, refresh };
}
