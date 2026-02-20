"use client";

import { useEffect } from "react";
import { listSecrets } from "@/lib/api/domains/secrets-api";
import { useAppStore } from "@/components/state-provider";

export function useSecrets() {
  const items = useAppStore((state) => state.secrets.items);
  const loaded = useAppStore((state) => state.secrets.loaded);
  const loading = useAppStore((state) => state.secrets.loading);
  const setSecrets = useAppStore((state) => state.setSecrets);
  const setSecretsLoading = useAppStore((state) => state.setSecretsLoading);

  useEffect(() => {
    if (loaded || loading) return;
    setSecretsLoading(true);
    listSecrets({ cache: "no-store" })
      .then((response) => {
        setSecrets(response ?? []);
      })
      .catch(() => {
        setSecrets([]);
      })
      .finally(() => {
        setSecretsLoading(false);
      });
  }, [loaded, loading, setSecrets, setSecretsLoading]);

  return { items, loaded, loading };
}
