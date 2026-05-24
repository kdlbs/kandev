"use client";

import { useEffect, useCallback, useRef, useState } from "react";
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
} from "@/lib/types/automation";

export function useAutomations(workspaceId: string | null) {
  const items = useAppStore((state) => state.automations.items);
  const loading = useAppStore((state) => state.automations.loading);
  const setAutomations = useAppStore((state) => state.setAutomations);
  const setLoading = useAppStore((state) => state.setAutomationsLoading);
  const addToStore = useAppStore((state) => state.addAutomation);
  const updateInStore = useAppStore((state) => state.updateAutomation);
  const removeFromStore = useAppStore((state) => state.removeAutomation);

  // Track which workspace the current store contents belong to so a
  // workspace switch refetches instead of serving stale data from the
  // previous workspace. Also gate the response apply behind the in-flight
  // workspace id to drop late responses that arrive after a quick switch.
  //
  // loadedWorkspaceRef is a ref (not state) so the effect guard on line
  // below does not create a stale-closure problem. loadedWorkspaceId is
  // the parallel state copy used only for the render-time `loaded` flag.
  const loadedWorkspaceRef = useRef<string | null>(null);
  const inFlightWorkspaceRef = useRef<string | null>(null);
  const [loadedWorkspaceId, setLoadedWorkspaceId] = useState<string | null>(null);

  useEffect(() => {
    if (!workspaceId) return;
    if (loadedWorkspaceRef.current === workspaceId) return;
    inFlightWorkspaceRef.current = workspaceId;
    setLoading(true);
    listAutomations(workspaceId)
      .then((result) => {
        if (inFlightWorkspaceRef.current !== workspaceId) return; // stale
        setAutomations(result ?? []);
        loadedWorkspaceRef.current = workspaceId;
        setLoadedWorkspaceId(workspaceId);
      })
      .catch(() => {
        if (inFlightWorkspaceRef.current !== workspaceId) return;
        setAutomations([]);
        loadedWorkspaceRef.current = workspaceId;
        setLoadedWorkspaceId(workspaceId);
      })
      .finally(() => {
        if (inFlightWorkspaceRef.current === workspaceId) {
          setLoading(false);
        }
      });
  }, [workspaceId, setAutomations, setLoading]);

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

  const refresh = useCallback(() => {
    if (!workspaceId) return;
    inFlightWorkspaceRef.current = workspaceId;
    setLoading(true);
    listAutomations(workspaceId)
      .then((result) => {
        if (inFlightWorkspaceRef.current !== workspaceId) return;
        setAutomations(result ?? []);
      })
      .catch(() => {})
      .finally(() => {
        if (inFlightWorkspaceRef.current === workspaceId) setLoading(false);
      });
  }, [workspaceId, setAutomations, setLoading]);

  // loaded mirrors "are we on the workspace we've fetched at least once?"
  const loaded = loadedWorkspaceId === workspaceId;
  return { items, loaded, loading, create, update, remove, enable, disable, trigger, refresh };
}
