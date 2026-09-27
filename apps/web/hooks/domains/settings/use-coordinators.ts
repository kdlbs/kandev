"use client";

import { useEffect, useCallback, useRef, useState } from "react";
import {
  listCoordinators,
  createCoordinator,
  patchCoordinator as apiPatchCoordinator,
  deleteCoordinator,
} from "@/lib/api/domains/coordinator-api";
import { useAppStore } from "@/components/state-provider";
import type {
  Coordinator,
  CreateCoordinatorRequest,
  PatchCoordinatorRequest,
} from "@/lib/api/domains/coordinator-api";

/**
 * The workspace's coordinators, cached in the store and refetched on
 * workspace switch. Mirrors `useAutomations`: a workspace switch drops the
 * previous workspace's rows rather than serving them under the new id.
 */
export function useCoordinators(workspaceId: string | null) {
  const items = useAppStore((state) => state.coordinators.items);
  const loading = useAppStore((state) => state.coordinators.loading);
  const setCoordinators = useAppStore((state) => state.setCoordinators);
  const setLoading = useAppStore((state) => state.setCoordinatorsLoading);
  const addToStore = useAppStore((state) => state.addCoordinator);
  const updateInStore = useAppStore((state) => state.updateCoordinator);
  const removeFromStore = useAppStore((state) => state.removeCoordinator);

  const loadedWorkspaceRef = useRef<string | null>(null);
  const inFlightWorkspaceRef = useRef<string | null>(null);
  const [loadedWorkspaceId, setLoadedWorkspaceId] = useState<string | null>(null);

  useEffect(() => {
    if (!workspaceId) return;
    if (loadedWorkspaceRef.current === workspaceId) return;
    inFlightWorkspaceRef.current = workspaceId;
    setLoading(true);
    listCoordinators(workspaceId)
      .then((result) => {
        if (inFlightWorkspaceRef.current !== workspaceId) return; // stale
        setCoordinators(result.coordinators ?? []);
        loadedWorkspaceRef.current = workspaceId;
        setLoadedWorkspaceId(workspaceId);
      })
      .catch(() => {
        if (inFlightWorkspaceRef.current !== workspaceId) return;
        setCoordinators([]);
        loadedWorkspaceRef.current = workspaceId;
        setLoadedWorkspaceId(workspaceId);
      })
      .finally(() => {
        if (inFlightWorkspaceRef.current === workspaceId) {
          setLoading(false);
        }
      });
  }, [workspaceId, setCoordinators, setLoading]);

  const create = useCallback(
    async (req: CreateCoordinatorRequest): Promise<Coordinator> => {
      if (!workspaceId) throw new Error("workspaceId is required");
      const coordinator = await createCoordinator(workspaceId, req);
      addToStore(coordinator);
      return coordinator;
    },
    [workspaceId, addToStore],
  );

  const patch = useCallback(
    async (coordinatorId: string, req: PatchCoordinatorRequest): Promise<Coordinator> => {
      if (!workspaceId) throw new Error("workspaceId is required");
      const coordinator = await apiPatchCoordinator(workspaceId, coordinatorId, req);
      updateInStore(coordinator);
      return coordinator;
    },
    [workspaceId, updateInStore],
  );

  const remove = useCallback(
    async (coordinatorId: string): Promise<void> => {
      if (!workspaceId) throw new Error("workspaceId is required");
      await deleteCoordinator(workspaceId, coordinatorId);
      removeFromStore(coordinatorId);
    },
    [workspaceId, removeFromStore],
  );

  const refresh = useCallback(() => {
    if (!workspaceId) return;
    inFlightWorkspaceRef.current = workspaceId;
    setLoading(true);
    listCoordinators(workspaceId)
      .then((result) => {
        if (inFlightWorkspaceRef.current !== workspaceId) return;
        setCoordinators(result.coordinators ?? []);
      })
      .catch(() => {})
      .finally(() => {
        if (inFlightWorkspaceRef.current === workspaceId) setLoading(false);
      });
  }, [workspaceId, setCoordinators, setLoading]);

  const loaded = loadedWorkspaceId === workspaceId;
  return { items, loaded, loading, create, patch, remove, refresh };
}
