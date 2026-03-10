"use client";

import { useEffect, useCallback } from "react";
import {
  listAutomations,
  createAutomation,
  updateAutomation as apiUpdateAutomation,
  deleteAutomation,
  enableAutomation,
  disableAutomation,
  triggerAutomation,
} from "@/lib/api/domains/automation-api";
import { useAppStore } from "@/components/state-provider";
import type { CreateAutomationRequest, UpdateAutomationRequest } from "@/lib/types/automation";

export function useAutomations(workspaceId: string | null) {
  const items = useAppStore((state) => state.automations.items);
  const loaded = useAppStore((state) => state.automations.loaded);
  const loading = useAppStore((state) => state.automations.loading);
  const setAutomations = useAppStore((state) => state.setAutomations);
  const setLoading = useAppStore((state) => state.setAutomationsLoading);
  const addToStore = useAppStore((state) => state.addAutomation);
  const updateInStore = useAppStore((state) => state.updateAutomation);
  const removeFromStore = useAppStore((state) => state.removeAutomation);

  useEffect(() => {
    if (!workspaceId || loaded || loading) return;
    setLoading(true);
    listAutomations(workspaceId)
      .then((result) => {
        setAutomations(result ?? []);
      })
      .catch(() => {
        setAutomations([]);
      })
      .finally(() => {
        setLoading(false);
      });
  }, [workspaceId, loaded, loading, setAutomations, setLoading]);

  const create = useCallback(
    async (req: CreateAutomationRequest) => {
      const automation = await createAutomation(req);
      addToStore(automation);
      return automation;
    },
    [addToStore],
  );

  const update = useCallback(
    async (id: string, req: UpdateAutomationRequest) => {
      const automation = await apiUpdateAutomation(id, req);
      updateInStore(automation);
      return automation;
    },
    [updateInStore],
  );

  const remove = useCallback(
    async (id: string) => {
      await deleteAutomation(id);
      removeFromStore(id);
    },
    [removeFromStore],
  );

  const enable = useCallback(
    async (id: string) => {
      const automation = await enableAutomation(id);
      updateInStore(automation);
      return automation;
    },
    [updateInStore],
  );

  const disable = useCallback(
    async (id: string) => {
      const automation = await disableAutomation(id);
      updateInStore(automation);
      return automation;
    },
    [updateInStore],
  );

  const trigger = useCallback(async (id: string) => {
    return triggerAutomation(id);
  }, []);

  const refresh = useCallback(() => {
    if (!workspaceId) return;
    setLoading(true);
    listAutomations(workspaceId)
      .then((result) => {
        setAutomations(result ?? []);
      })
      .catch(() => {})
      .finally(() => {
        setLoading(false);
      });
  }, [workspaceId, setAutomations, setLoading]);

  return { items, loaded, loading, create, update, remove, enable, disable, trigger, refresh };
}
