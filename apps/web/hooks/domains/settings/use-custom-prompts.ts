"use client";

import { useEffect } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { loadPrompts } from "@/lib/state/prompts-loader";

type UseCustomPromptsOptions = { enabled?: boolean };

export function useCustomPrompts({ enabled = true }: UseCustomPromptsOptions = {}) {
  const store = useAppStoreApi();
  const prompts = useAppStore((state) => state.prompts.items);
  const loaded = useAppStore((state) => state.prompts.loaded);
  const loading = useAppStore((state) => state.prompts.loading);
  useEffect(() => {
    if (enabled && !loaded) void loadPrompts(store);
  }, [enabled, loaded, store]);
  return { prompts, loaded, loading };
}
