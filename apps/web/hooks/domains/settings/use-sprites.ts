"use client";

import { useEffect } from "react";
import { getSpritesStatus, listSpritesInstances } from "@/lib/api/domains/sprites-api";
import { useAppStore } from "@/components/state-provider";

export function useSprites(secretId?: string) {
  const status = useAppStore((state) => state.sprites.status);
  const instances = useAppStore((state) => state.sprites.instances);
  const loaded = useAppStore((state) => state.sprites.loaded);
  const loading = useAppStore((state) => state.sprites.loading);
  const setSpritesStatus = useAppStore((state) => state.setSpritesStatus);
  const setSpritesInstances = useAppStore((state) => state.setSpritesInstances);
  const setSpritesLoading = useAppStore((state) => state.setSpritesLoading);

  useEffect(() => {
    if (loaded || loading) return;
    setSpritesLoading(true);

    Promise.all([
      getSpritesStatus(secretId, { cache: "no-store" }),
      listSpritesInstances(secretId, { cache: "no-store" }),
    ])
      .then(([statusRes, instancesRes]) => {
        setSpritesStatus(statusRes);
        setSpritesInstances(instancesRes ?? []);
      })
      .catch(() => {
        setSpritesStatus({ connected: false, token_configured: false, instance_count: 0 });
        setSpritesInstances([]);
      })
      .finally(() => {
        setSpritesLoading(false);
      });
  }, [loaded, loading, secretId, setSpritesStatus, setSpritesInstances, setSpritesLoading]);

  return { status, instances, loaded, loading };
}
