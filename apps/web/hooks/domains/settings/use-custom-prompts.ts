"use client";

import { useEffect } from "react";
import { listPrompts } from "@/lib/api";
import { useAppStore } from "@/components/state-provider";
import type { CustomPrompt } from "@/lib/types/http";

type UseCustomPromptsOptions = {
  enabled?: boolean;
};

let inFlightPromptsRequest: Promise<CustomPrompt[]> | null = null;

function requestPrompts() {
  if (inFlightPromptsRequest) return inFlightPromptsRequest;

  const request = listPrompts({ cache: "no-store" }).then((response) => response.prompts ?? []);
  inFlightPromptsRequest = request;
  void request.then(
    () => {
      if (inFlightPromptsRequest === request) inFlightPromptsRequest = null;
    },
    () => {
      if (inFlightPromptsRequest === request) inFlightPromptsRequest = null;
    },
  );
  return request;
}

export function useCustomPrompts({ enabled = true }: UseCustomPromptsOptions = {}) {
  const prompts = useAppStore((state) => state.prompts.items);
  const loaded = useAppStore((state) => state.prompts.loaded);
  const loading = useAppStore((state) => state.prompts.loading);
  const setPrompts = useAppStore((state) => state.setPrompts);
  const setPromptsLoading = useAppStore((state) => state.setPromptsLoading);

  useEffect(() => {
    if (!enabled || loaded || loading) return;
    setPromptsLoading(true);
    requestPrompts()
      .then((nextPrompts) => {
        setPrompts(nextPrompts);
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
