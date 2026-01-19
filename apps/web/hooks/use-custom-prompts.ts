'use client';

import { useEffect } from 'react';
import { listPrompts } from '@/lib/http';
import { useAppStore } from '@/components/state-provider';
import type { CustomPrompt } from '@/lib/types/http';

export function useCustomPrompts(initialPrompts?: CustomPrompt[]) {
  const prompts = useAppStore((state) => state.prompts.items);
  const loaded = useAppStore((state) => state.prompts.loaded);
  const loading = useAppStore((state) => state.prompts.loading);
  const setPrompts = useAppStore((state) => state.setPrompts);
  const setPromptsLoading = useAppStore((state) => state.setPromptsLoading);

  useEffect(() => {
    if (!initialPrompts || loaded) return;
    setPrompts(initialPrompts);
  }, [initialPrompts, loaded, setPrompts]);

  useEffect(() => {
    if (loaded || loading) return;
    setPromptsLoading(true);
    listPrompts({ cache: 'no-store' })
      .then((response) => {
        setPrompts(response.prompts ?? []);
      })
      .catch(() => {
        setPrompts([]);
      })
      .finally(() => {
        setPromptsLoading(false);
      });
  }, [loaded, loading, setPrompts, setPromptsLoading]);

  return {
    prompts,
    loaded,
    loading,
  };
}
