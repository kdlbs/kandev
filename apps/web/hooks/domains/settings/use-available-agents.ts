import { useEffect } from 'react';
import { useAppStore } from '@/components/state-provider';
import { listAvailableAgents } from '@/lib/api';

export function useAvailableAgents(enabled = true) {
  const availableAgents = useAppStore((state) => state.availableAgents);
  const setAvailableAgents = useAppStore((state) => state.setAvailableAgents);
  const setAvailableAgentsLoading = useAppStore((state) => state.setAvailableAgentsLoading);

  useEffect(() => {
    if (!enabled) return;
    if (availableAgents.loaded || availableAgents.loading) return;
    setAvailableAgentsLoading(true);
    listAvailableAgents({ cache: 'no-store' })
      .then((response) => {
        setAvailableAgents(response.agents);
      })
      .catch(() => setAvailableAgents([]));
  }, [
    availableAgents.loaded,
    availableAgents.loading,
    enabled,
    setAvailableAgents,
    setAvailableAgentsLoading,
  ]);

  return availableAgents;
}
