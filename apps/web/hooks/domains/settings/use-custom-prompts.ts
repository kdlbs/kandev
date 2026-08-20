"use client";

import { useEffect } from "react";
import { listPrompts } from "@/lib/api";
import { useAppStore } from "@/components/state-provider";

type UseCustomPromptsOptions = {
  enabled?: boolean;
};

export function useCustomPrompts({ enabled = true }: UseCustomPromptsOptions = {}) {
  const prompts = useAppStore((state) => state.prompts.items);
  const loaded = useAppStore((state) => state.prompts.loaded);
  const loading = useAppStore((state) => state.prompts.loading);
  const setPrompts = useAppStore((state) => state.setPrompts);
  const setPromptsLoading = useAppStore((state) => state.setPromptsLoading);

  useEffect(() => {
    if (!enabled || loaded || loading) return;
    setPromptsLoading(true);
    listPrompts({ cache: "no-store" })
      .then((response) => {
        setPrompts(response.prompts ?? []);
      })
      .catch(() => {
        setPrompts([]);
      })
      .finally(() => {
        setPromptsLoading(false);
      });
  }, [enabled, loaded, loading, setPrompts, setPromptsLoading]);

  return {
    prompts,
    loaded,
    loading,
  };
}
