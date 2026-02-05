import { useEffect, useRef } from 'react';
import { useAppStore } from '@/components/state-provider';
import type { RepositoryScript } from '@/lib/types/http';
import { listRepositoryScripts } from '@/lib/api';

const EMPTY_SCRIPTS: RepositoryScript[] = [];

export function useRepositoryScripts(repositoryId: string | null, enabled = true) {
  const scripts = useAppStore((state) =>
    repositoryId
      ? state.repositoryScripts.itemsByRepositoryId[repositoryId] ?? EMPTY_SCRIPTS
      : EMPTY_SCRIPTS
  );
  const isLoading = useAppStore((state) =>
    repositoryId ? state.repositoryScripts.loadingByRepositoryId[repositoryId] ?? false : false
  );
  const isLoaded = useAppStore((state) =>
    repositoryId ? state.repositoryScripts.loadedByRepositoryId[repositoryId] ?? false : false
  );
  const setRepositoryScripts = useAppStore((state) => state.setRepositoryScripts);
  const setRepositoryScriptsLoading = useAppStore((state) => state.setRepositoryScriptsLoading);
  const inFlightRef = useRef(false);

  useEffect(() => {
    if (!enabled || !repositoryId) return;
    if (isLoaded && isLoading) {
      setRepositoryScriptsLoading(repositoryId, false);
    }
  }, [enabled, isLoaded, isLoading, setRepositoryScriptsLoading, repositoryId]);

  useEffect(() => {
    if (!enabled || !repositoryId) return;
    if (isLoaded || inFlightRef.current) return;

    let cancelled = false;
    inFlightRef.current = true;
    setRepositoryScriptsLoading(repositoryId, true);

    listRepositoryScripts(repositoryId, { cache: 'no-store' })
      .then((response) => {
        if (cancelled) return;
        setRepositoryScripts(repositoryId, response.scripts ?? []);
      })
      .catch((error) => {
        console.error('[useRepositoryScripts] Fetch error:', { repositoryId, error });
        if (cancelled) return;
        setRepositoryScripts(repositoryId, []);
      })
      .finally(() => {
        inFlightRef.current = false;
        if (cancelled) return;
        setRepositoryScriptsLoading(repositoryId, false);
      });

    return () => {
      cancelled = true;
      inFlightRef.current = false;
    };
  }, [enabled, isLoaded, setRepositoryScripts, setRepositoryScriptsLoading, repositoryId]);

  return { scripts, isLoading, isLoaded };
}
