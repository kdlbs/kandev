"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { settingsQueryOptions } from "@/lib/query/query-options/settings";
import { qk } from "@/lib/query/keys";
import {
  createPrompt,
  updatePrompt as updatePromptApi,
  deletePrompt,
} from "@/lib/api/domains/settings-api";
import type { CustomPrompt } from "@/lib/types/http";

export function useCustomPrompts() {
  const query = useQuery(settingsQueryOptions.prompts());
  return {
    prompts: query.data ?? [],
    loaded: query.isSuccess,
    loading: query.isFetching,
  };
}

export function useCreatePrompt() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (payload: { name: string; content: string }) => createPrompt(payload),
    onSuccess: (item) => {
      qc.setQueryData(qk.settings.prompts(), (prev: CustomPrompt[] | undefined) => {
        const items = prev ?? [];
        return [...items.filter((p) => p.id !== item.id), item];
      });
    },
  });
}

export function useUpdatePrompt() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, payload }: { id: string; payload: { name?: string; content?: string } }) =>
      updatePromptApi(id, payload),
    onSuccess: (item) => {
      qc.setQueryData(qk.settings.prompts(), (prev: CustomPrompt[] | undefined) => {
        if (!prev) return prev;
        return prev.map((p) => (p.id === item.id ? { ...p, ...item } : p));
      });
    },
  });
}

export function useDeletePrompt() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deletePrompt(id),
    onSuccess: (_result, id) => {
      qc.setQueryData(qk.settings.prompts(), (prev: CustomPrompt[] | undefined) => {
        if (!prev) return prev;
        return prev.filter((p) => p.id !== id);
      });
    },
  });
}
