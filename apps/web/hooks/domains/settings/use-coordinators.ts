"use client";

import { useEffect, useLayoutEffect, useCallback, useMemo, useRef, useState } from "react";
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

type ListIdentity = {
  workspaceId: string;
  setCoordinators: (items: Coordinator[]) => void;
  setLoading: (loading: boolean) => void;
};

const EMPTY_COORDINATORS: Coordinator[] = [];

function useCoordinatorList(
  workspaceId: string | null,
  setCoordinators: ListIdentity["setCoordinators"],
  setLoading: ListIdentity["setLoading"],
) {
  const accepted = useRef<ListIdentity | null>(null);
  const [loadedIdentity, setLoadedIdentity] = useState<ListIdentity | null>(null);
  const [loadError, setLoadError] = useState(false);
  const lifetime = useMemo(
    () => ({ active: false, request: null as symbol | null }),
    [workspaceId, setCoordinators, setLoading],
  );

  // Retire callbacks at commit, before another layout effect can invoke them.
  useLayoutEffect(() => {
    lifetime.active = true;
    setLoadError(false);
    return () => {
      lifetime.active = false;
      if (lifetime.request !== null) {
        lifetime.request = null;
        setLoading(false);
      }
    };
  }, [lifetime, setLoading]);

  const refresh = useCallback(() => {
    if (!lifetime.active || !workspaceId) return;
    const token = Symbol();
    lifetime.request = token;
    const current = () => lifetime.active && lifetime.request === token;
    setLoading(true);
    listCoordinators(workspaceId)
      .then((result) => {
        if (!current()) return;
        setCoordinators(result.coordinators ?? []);
        setLoadError(false);
        const identity = { workspaceId, setCoordinators, setLoading };
        accepted.current = identity;
        setLoadedIdentity(identity);
      })
      .catch(() => {
        if (current()) setLoadError(true);
      })
      .finally(() => {
        if (!current()) return;
        lifetime.request = null;
        setLoading(false);
      });
  }, [lifetime, workspaceId, setCoordinators, setLoading]);

  useEffect(() => {
    const cached = accepted.current;
    if (
      cached?.workspaceId === workspaceId &&
      cached?.setCoordinators === setCoordinators &&
      cached?.setLoading === setLoading
    )
      return;
    refresh();
  }, [workspaceId, setCoordinators, setLoading, refresh]);

  const loaded =
    loadedIdentity?.workspaceId === workspaceId &&
    loadedIdentity?.setCoordinators === setCoordinators &&
    loadedIdentity?.setLoading === setLoading;
  return { loaded, loadError, refresh };
}

/** The workspace's coordinator rows, with list reads owned by this hook lifetime. */
export function useCoordinators(workspaceId: string | null) {
  const items = useAppStore((state) => state.coordinators.items);
  const loading = useAppStore((state) => state.coordinators.loading);
  const setCoordinators = useAppStore((state) => state.setCoordinators);
  const setLoading = useAppStore((state) => state.setCoordinatorsLoading);
  const addToStore = useAppStore((state) => state.addCoordinator);
  const updateInStore = useAppStore((state) => state.updateCoordinator);
  const removeFromStore = useAppStore((state) => state.removeCoordinator);

  const { loaded, loadError, refresh } = useCoordinatorList(
    workspaceId,
    setCoordinators,
    setLoading,
  );

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

  return {
    items: loaded ? items : EMPTY_COORDINATORS,
    loaded,
    loading,
    loadError,
    create,
    patch,
    remove,
    refresh,
  };
}
