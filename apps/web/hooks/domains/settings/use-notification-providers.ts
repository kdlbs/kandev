"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { settingsQueryOptions } from "@/lib/query/query-options/settings";
import { qk } from "@/lib/query/keys";
import {
  createNotificationProvider,
  updateNotificationProvider as updateProviderApi,
  deleteNotificationProvider,
} from "@/lib/api/domains/settings-api";
import type { NotificationProvider } from "@/lib/types/http";

export function useNotificationProviders() {
  const query = useQuery(settingsQueryOptions.notificationProviders());
  return {
    providers: query.data?.items ?? [],
    events: query.data?.events ?? [],
    appriseAvailable: query.data?.appriseAvailable ?? false,
    loaded: query.isSuccess,
    loading: query.isFetching,
  };
}

export function useCreateNotificationProvider() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (payload: Parameters<typeof createNotificationProvider>[0]) =>
      createNotificationProvider(payload),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: qk.settings.notificationProviders() });
    },
  });
}

export function useUpdateNotificationProvider() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      payload,
    }: {
      id: string;
      payload: Parameters<typeof updateProviderApi>[1];
    }) => updateProviderApi(id, payload),
    onSuccess: (updated) => {
      qc.setQueryData(
        qk.settings.notificationProviders(),
        (
          prev:
            | { items: NotificationProvider[]; events: string[]; appriseAvailable: boolean }
            | undefined,
        ) => {
          if (!prev) return prev;
          return {
            ...prev,
            items: prev.items.map((p) => (p.id === updated.id ? { ...p, ...updated } : p)),
          };
        },
      );
    },
  });
}

export function useDeleteNotificationProvider() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteNotificationProvider(id),
    onSuccess: (_result, id) => {
      qc.setQueryData(
        qk.settings.notificationProviders(),
        (
          prev:
            | { items: NotificationProvider[]; events: string[]; appriseAvailable: boolean }
            | undefined,
        ) => {
          if (!prev) return prev;
          return { ...prev, items: prev.items.filter((p) => p.id !== id) };
        },
      );
    },
  });
}
