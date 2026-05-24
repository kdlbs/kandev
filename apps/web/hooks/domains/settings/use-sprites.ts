"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { settingsQueryOptions } from "@/lib/query/query-options/settings";
import { qk } from "@/lib/query/keys";
import { destroySprite } from "@/lib/api/domains/sprites-api";
import type { SpritesStatus, SpritesInstance } from "@/lib/types/http-sprites";

export function useSprites(secretId?: string) {
  const query = useQuery(settingsQueryOptions.sprites(secretId));
  return {
    status: query.data?.status ?? null,
    instances: query.data?.instances ?? [],
    loaded: query.isSuccess,
    loading: query.isFetching,
  };
}

export function useDestroySprite(secretId?: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => destroySprite(name, secretId),
    onSuccess: (_result, name) => {
      qc.setQueryData(
        qk.settings.sprites(secretId),
        (prev: { status: SpritesStatus; instances: SpritesInstance[] } | undefined) => {
          if (!prev) return prev;
          const instances = prev.instances.filter((i) => i.name !== name);
          return {
            ...prev,
            instances,
            status: prev.status
              ? { ...prev.status, instance_count: instances.length }
              : prev.status,
          };
        },
      );
    },
  });
}
