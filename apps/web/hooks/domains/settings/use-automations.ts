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
import type {
  CreateAutomationRequest,
  CreateAutomationResponse,
  UpdateAutomationRequest,
  Automation,
} from "@/lib/types/automation";

const EMPTY_AUTOMATIONS: Automation[] = [];

export function useAutomations(workspaceId: string | null) {
  const list = useAppStore((state) =>
    workspaceId ? state.automations.byWorkspace?.[workspaceId] : undefined,
  );
  const beginList = useAppStore((state) => state.beginAutomationsList);
  const finishList = useAppStore((state) => state.finishAutomationsList);
  const addToStore = useAppStore((state) => state.addAutomation);
  const updateInStore = useAppStore((state) => state.updateAutomation);
  const removeFromStore = useAppStore((state) => state.removeAutomation);

  const load = useCallback(
    (refresh = false) => {
      if (!workspaceId) return;
      const generation = beginList(workspaceId, refresh);
      if (generation === null) return;
      // The owning store keeps a shared read alive across consumer unmounts.
      listAutomations(workspaceId).then(
        (result) => finishList(workspaceId, generation, result ?? []),
        () => finishList(workspaceId, generation),
      );
    },
    [workspaceId, beginList, finishList],
  );
  useEffect(() => load(), [load]);

  const create = useCallback(
    async (req: CreateAutomationRequest): Promise<CreateAutomationResponse> => {
      const automation = await createAutomation(req);
      // Strip the one-time webhook_secret before persisting to the store so it
      // doesn't leak into devtools or error-reporting SDKs. The full response
      // (with secret) is still returned to the caller for the reveal dialog.
      const { webhook_secret: _secret, ...stored } = automation;
      addToStore(stored);
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

  const refresh = useCallback(() => load(true), [load]);
  const items = list?.items ?? EMPTY_AUTOMATIONS;
  const loaded = Boolean(workspaceId && list?.loaded);
  const loading = Boolean(workspaceId) && (list?.loading ?? true);
  return { items, loaded, loading, create, update, remove, enable, disable, trigger, refresh };
}
