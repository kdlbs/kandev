"use client";

import { useEffect } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { settingsQueryOptions } from "@/lib/query/query-options/settings";
import { qk } from "@/lib/query/keys";
import {
  createEditor,
  updateEditor as updateEditorApi,
  deleteEditor,
} from "@/lib/api/domains/settings-api";
import type { EditorOption } from "@/lib/types/http";
import { getWebSocketClient } from "@/lib/ws/connection";

export function useEditors() {
  // Subscribe to user WS events so editor changes from other sessions are
  // reflected (mirrors the effect that was in the old Zustand hook).
  useEffect(() => {
    const client = getWebSocketClient();
    if (client) client.subscribeUser();
  }, []);

  const query = useQuery(settingsQueryOptions.editors());
  return {
    editors: query.data ?? [],
    loaded: query.isSuccess,
    loading: query.isFetching,
  };
}

export function useCreateEditor() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (payload: Parameters<typeof createEditor>[0]) => createEditor(payload),
    onSuccess: (item) => {
      qc.setQueryData(qk.settings.editors(), (prev: EditorOption[] | undefined) => {
        const items = prev ?? [];
        return [...items.filter((e) => e.id !== item.id), item];
      });
    },
  });
}

export function useUpdateEditor() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, payload }: { id: string; payload: Parameters<typeof updateEditorApi>[1] }) =>
      updateEditorApi(id, payload),
    onSuccess: (item) => {
      qc.setQueryData(qk.settings.editors(), (prev: EditorOption[] | undefined) => {
        if (!prev) return prev;
        return prev.map((e) => (e.id === item.id ? { ...e, ...item } : e));
      });
    },
  });
}

export function useDeleteEditor() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteEditor(id),
    onSuccess: (_result, id) => {
      qc.setQueryData(qk.settings.editors(), (prev: EditorOption[] | undefined) => {
        if (!prev) return prev;
        return prev.filter((e) => e.id !== id);
      });
    },
  });
}
