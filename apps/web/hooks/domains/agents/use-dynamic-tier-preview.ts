import { useCallback, useEffect, useRef, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { stateReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import {
  previewDynamicTiers,
  type DynamicTierPreview,
} from "@/lib/api/domains/dynamic-tier-preview-api";

export function useDynamicTierPreview(
  enabled: boolean,
  profileId: string | undefined,
  document: unknown,
) {
  const store = useAppStoreApi();
  const identity = useAppStore(stateReadScopeIdentity);
  const key = JSON.stringify([enabled, identity, profileId, document]);
  const latest = useRef(key);
  latest.current = key;
  const request = useRef<AbortController | null>(null);
  const [result, setResult] = useState<{
    key: string;
    status: "loading" | "failed" | "ready";
    preview?: DynamicTierPreview;
  }>();
  useEffect(() => {
    request.current?.abort();
  }, [key]);
  useEffect(() => () => request.current?.abort(), []);
  const preview = useCallback(async () => {
    if (!enabled) return;
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    const current = () =>
      !controller.signal.aborted &&
      latest.current === key &&
      stateReadScopeIdentity(store.getState()) === identity;
    setResult({ key, status: "loading" });
    try {
      const value = await previewDynamicTiers(profileId, document, {
        init: { signal: controller.signal },
      });
      if (current()) setResult({ key, status: "ready", preview: value });
    } catch {
      if (current()) setResult({ key, status: "failed" });
    }
  }, [enabled, profileId, document, identity, key, store]);
  return { result: result?.key === key ? result : undefined, preview };
}
