import { useState, useEffect, useCallback } from 'react';
import { fetchDynamicModels } from '@/lib/api/domains/settings-api';
import type { ModelEntry, DynamicModelsResponse } from '@/lib/types/http';

type UseDynamicModelsState = {
  models: ModelEntry[];
  isLoading: boolean;
  error: string | null;
  cached: boolean;
  cachedAt: string | null;
  refresh: () => Promise<void>;
};

export function useDynamicModels(
  agentName: string | undefined,
  staticModels: ModelEntry[],
  supportsDynamicModels: boolean
): UseDynamicModelsState {
  const [models, setModels] = useState<ModelEntry[]>(staticModels);
  // Start in loading state if dynamic models are supported to avoid UI flash
  const [isLoading, setIsLoading] = useState(supportsDynamicModels && !!agentName);
  const [error, setError] = useState<string | null>(null);
  const [cached, setCached] = useState(false);
  const [cachedAt, setCachedAt] = useState<string | null>(null);

  const fetchModels = useCallback(
    async (forceRefresh = false) => {
      if (!agentName || !supportsDynamicModels) {
        setModels(staticModels);
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
          // Fall back to static models on error
          setModels(staticModels);
        } else {
          setModels(response.models);
        }

        setCached(response.cached);
        setCachedAt(response.cached_at ?? null);
      } catch (err) {
        const errorMessage = err instanceof Error ? err.message : 'Failed to fetch models';
        setError(errorMessage);
        // Fall back to static models on error
        setModels(staticModels);
      } finally {
        setIsLoading(false);
      }
    },
    [agentName, supportsDynamicModels, staticModels]
  );

  // Fetch on mount if dynamic models are supported
  useEffect(() => {
    if (supportsDynamicModels && agentName) {
      fetchModels(false);
    } else {
      setModels(staticModels);
    }
  }, [agentName, supportsDynamicModels, fetchModels, staticModels]);

  const refresh = useCallback(async () => {
    await fetchModels(true);
  }, [fetchModels]);

  return {
    models,
    isLoading,
    error,
    cached,
    cachedAt,
    refresh,
  };
}
