"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { settingsQueryOptions } from "@/lib/query/query-options/settings";
import { qk } from "@/lib/query/keys";
import {
  createSecret,
  updateSecret as updateSecretApi,
  deleteSecret,
} from "@/lib/api/domains/secrets-api";
import type { SecretListItem, CreateSecretRequest, UpdateSecretRequest } from "@/lib/types/http-secrets";

export function useSecrets() {
  const query = useQuery(settingsQueryOptions.secrets());
  return {
    items: query.data ?? [],
    loaded: query.isSuccess,
    loading: query.isFetching,
  };
}

export function useCreateSecret() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (payload: CreateSecretRequest) => createSecret(payload),
    onSuccess: (item) => {
      qc.setQueryData(qk.settings.secrets(), (prev: SecretListItem[] | undefined) => {
        const items = prev ?? [];
        return [...items.filter((s) => s.id !== item.id), item];
      });
    },
  });
}

export function useUpdateSecret() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, payload }: { id: string; payload: UpdateSecretRequest }) =>
      updateSecretApi(id, payload),
    onSuccess: (item) => {
      qc.setQueryData(qk.settings.secrets(), (prev: SecretListItem[] | undefined) => {
        if (!prev) return prev;
        return prev.map((s) => (s.id === item.id ? { ...s, ...item } : s));
      });
    },
  });
}

export function useDeleteSecret() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteSecret(id),
    onSuccess: (_result, id) => {
      qc.setQueryData(qk.settings.secrets(), (prev: SecretListItem[] | undefined) => {
        if (!prev) return prev;
        return prev.filter((s) => s.id !== id);
      });
    },
  });
}
