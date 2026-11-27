"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { ApiError } from "@/lib/api/client";
import {
  getCoordinator,
  patchCoordinator as apiPatchCoordinator,
  deleteCoordinator,
  type Coordinator,
  type PatchCoordinatorRequest,
} from "@/lib/api/domains/coordinator-api";

export type CoordinatorFetchStatus = "loading" | "ready" | "not-found" | "error";

/**
 * A single coordinator by id, fetched through GET (which is the only route
 * that carries `agent_profile_status` / `executor_profile_status` — the list
 * response never does, per the design's Validation section). A 404 is
 * reported as `not-found` rather than thrown, so the page can show the
 * "All coordinators" link (Build decision 3) instead of an error toast.
 */
export function useCoordinator(workspaceId: string | null, coordinatorId: string | null) {
  const [coordinator, setCoordinator] = useState<Coordinator | null>(null);
  const [status, setStatus] = useState<CoordinatorFetchStatus>("loading");
  const updateInStore = useAppStore((state) => state.updateCoordinator);
  const removeFromStore = useAppStore((state) => state.removeCoordinator);
  const inFlightRef = useRef<string | null>(null);

  const load = useCallback(() => {
    if (!workspaceId || !coordinatorId) return;
    const key = `${workspaceId}/${coordinatorId}`;
    inFlightRef.current = key;
    setStatus("loading");
    getCoordinator(workspaceId, coordinatorId)
      .then((result) => {
        if (inFlightRef.current !== key) return;
        setCoordinator(result);
        setStatus("ready");
        updateInStore(result);
      })
      .catch((error: unknown) => {
        if (inFlightRef.current !== key) return;
        if (error instanceof ApiError && error.status === 404) {
          setCoordinator(null);
          setStatus("not-found");
          return;
        }
        setStatus("error");
      });
  }, [workspaceId, coordinatorId, updateInStore]);

  useEffect(() => {
    load();
  }, [load]);

  const patch = useCallback(
    async (req: PatchCoordinatorRequest): Promise<Coordinator> => {
      if (!workspaceId || !coordinatorId) throw new Error("workspaceId and coordinatorId required");
      const updated = await apiPatchCoordinator(workspaceId, coordinatorId, req);
      setCoordinator(updated);
      updateInStore(updated);
      return updated;
    },
    [workspaceId, coordinatorId, updateInStore],
  );

  const remove = useCallback(async (): Promise<void> => {
    if (!workspaceId || !coordinatorId) throw new Error("workspaceId and coordinatorId required");
    try {
      await deleteCoordinator(workspaceId, coordinatorId);
    } catch (error) {
      if (!(error instanceof ApiError) || error.status !== 404) throw error;
    }
    removeFromStore(coordinatorId);
  }, [workspaceId, coordinatorId, removeFromStore]);

  return { coordinator, status, refresh: load, patch, remove };
}
