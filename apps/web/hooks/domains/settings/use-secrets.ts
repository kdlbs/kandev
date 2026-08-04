"use client";

import { useEffect, useState } from "react";
import { listSecrets } from "@/lib/api/domains/secrets-api";
import { useAppStore } from "@/components/state-provider";
import type { SecretListItem, SecretScope } from "@/lib/types/http-secrets";

export function filterGlobalSecrets(items: SecretListItem[]): SecretListItem[] {
  return items.filter((item) => item.scope !== "workspace");
}

export function useSecrets(
  scope: SecretScope = "global",
  workspaceId?: string,
  initialItems?: SecretListItem[],
) {
  const globalItems = useAppStore((state) => state.secrets.items);
  const globalLoaded = useAppStore((state) => state.secrets.loaded);
  const globalLoading = useAppStore((state) => state.secrets.loading);
  const setSecrets = useAppStore((state) => state.setSecrets);
  const setSecretsLoading = useAppStore((state) => state.setSecretsLoading);
  const addGlobalSecret = useAppStore((state) => state.addSecret);
  const updateGlobalSecret = useAppStore((state) => state.updateSecret);
  const removeGlobalSecret = useAppStore((state) => state.removeSecret);

  const [scopedItems, setScopedItems] = useState<SecretListItem[]>(initialItems ?? []);
  const [scopedLoaded, setScopedLoaded] = useState(initialItems !== undefined);
  const [scopedLoading, setScopedLoading] = useState(false);
  const scopedKey = `${scope}:${workspaceId ?? ""}`;
  const [loadedScopedKey, setLoadedScopedKey] = useState(scopedKey);

  useEffect(() => {
    if (scope === "global") {
      if (globalLoaded || globalLoading) return;
      setSecretsLoading(true);
      listSecrets({ cache: "no-store" })
        .then((response) => setSecrets(response ?? []))
        .catch(() => setSecrets([]))
        .finally(() => setSecretsLoading(false));
      return;
    }

    const controller = new AbortController();
    let cancelled = false;
    setScopedItems(initialItems ?? []);
    setLoadedScopedKey(scopedKey);
    setScopedLoaded(initialItems !== undefined);
    setScopedLoading(initialItems === undefined && Boolean(workspaceId));

    if (!workspaceId || initialItems !== undefined) {
      setScopedLoading(false);
      return () => controller.abort();
    }

    listSecrets({ scope, workspaceId, cache: "no-store", init: { signal: controller.signal } })
      .then((response) => {
        if (cancelled) return;
        setScopedItems(response ?? []);
        setScopedLoaded(true);
      })
      .catch(() => {
        if (cancelled) return;
        setScopedItems([]);
        setScopedLoaded(true);
      })
      .finally(() => {
        if (!cancelled) setScopedLoading(false);
      });

    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [
    globalLoaded,
    globalLoading,
    initialItems,
    scope,
    setSecrets,
    setSecretsLoading,
    scopedKey,
    workspaceId,
  ]);

  const scopedCurrent = loadedScopedKey === scopedKey;
  let items: SecretListItem[] = [];
  if (scope === "global") {
    items = filterGlobalSecrets(globalItems);
  } else if (scopedCurrent) {
    items = scopedItems;
  }
  const loaded = scope === "global" ? globalLoaded : scopedCurrent && scopedLoaded;
  const loading = scope === "global" ? globalLoading : scopedCurrent && scopedLoading;

  const addSecret = (item: SecretListItem) => {
    if (scope === "global") {
      addGlobalSecret(item);
      return;
    }
    setScopedItems((current) => [...current, item]);
  };

  const updateSecret = (item: SecretListItem) => {
    if (scope === "global") {
      updateGlobalSecret(item);
      return;
    }
    setScopedItems((current) =>
      current.map((value) => (value.id === item.id ? { ...value, ...item } : value)),
    );
  };

  const removeSecret = (id: string) => {
    if (scope === "global") {
      removeGlobalSecret(id);
      return;
    }
    setScopedItems((current) => current.filter((item) => item.id !== id));
  };

  return { items, loaded, loading, addSecret, updateSecret, removeSecret };
}
