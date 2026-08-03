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
    if (!workspaceId || initialItems !== undefined) {
      setScopedLoaded(true);
      return;
    }
    setScopedLoading(true);
    listSecrets({ scope, workspaceId, cache: "no-store" })
      .then((response) => {
        setScopedItems(response ?? []);
        setScopedLoaded(true);
      })
      .catch(() => {
        setScopedItems([]);
        setScopedLoaded(true);
      })
      .finally(() => setScopedLoading(false));
  }, [
    globalLoaded,
    globalLoading,
    initialItems,
    scope,
    setSecrets,
    setSecretsLoading,
    workspaceId,
  ]);

  useEffect(() => {
    if (scope !== "workspace" || initialItems === undefined) return;
    setScopedItems(initialItems);
    setScopedLoaded(true);
  }, [initialItems, scope]);

  const items = scope === "global" ? filterGlobalSecrets(globalItems) : scopedItems;
  const loaded = scope === "global" ? globalLoaded : scopedLoaded;
  const loading = scope === "global" ? globalLoading : scopedLoading;

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
