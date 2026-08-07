"use client";

import { useEffect, useState } from "react";
import { listSecrets } from "@/lib/api/domains/secrets-api";
import type { SecretListItem } from "@/lib/types/http-secrets";

export function useWorkspaceSecretOptions(workspaceId?: string) {
  const [items, setItems] = useState<SecretListItem[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!workspaceId) {
      setItems([]);
      setLoaded(true);
      setLoading(false);
      return;
    }

    let cancelled = false;
    const controller = new AbortController();
    setItems([]);
    setLoading(true);
    setLoaded(false);
    listSecrets({
      scope: "workspace",
      workspaceId,
      includeGlobal: true,
      cache: "no-store",
      init: { signal: controller.signal },
    })
      .then((response) => {
        if (!cancelled) setItems(response ?? []);
      })
      .catch(() => {
        if (!cancelled) setItems([]);
      })
      .finally(() => {
        if (!cancelled) {
          setLoaded(true);
          setLoading(false);
        }
      });

    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [workspaceId]);

  return { items, loaded, loading };
}
