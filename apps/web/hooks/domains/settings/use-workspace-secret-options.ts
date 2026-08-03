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
      return;
    }

    let cancelled = false;
    setLoading(true);
    setLoaded(false);
    listSecrets({
      scope: "workspace",
      workspaceId,
      includeGlobal: true,
      cache: "no-store",
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
    };
  }, [workspaceId]);

  return { items, loaded, loading };
}
