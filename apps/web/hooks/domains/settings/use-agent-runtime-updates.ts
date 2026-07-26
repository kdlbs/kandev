"use client";

import { useCallback, useEffect } from "react";

import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { getAgentUpdateJob, getInstallJob, listAgentUpdateJobs, updateAgent } from "@/lib/api";
import { ApiError } from "@/lib/api/client";

type MaintenanceConflict = {
  active_job_id: string;
  active_kind: "install" | "update";
};

function maintenanceConflict(error: unknown): MaintenanceConflict | null {
  if (!(error instanceof ApiError) || error.status !== 409) return null;
  if (!error.body || typeof error.body !== "object") return null;
  const body = error.body as Record<string, unknown>;
  if (typeof body.active_job_id !== "string") return null;
  if (body.active_kind !== "install" && body.active_kind !== "update") return null;
  return {
    active_job_id: body.active_job_id,
    active_kind: body.active_kind,
  };
}

export function useAgentRuntimeUpdates() {
  const store = useAppStoreApi();
  const updateJobs = useAppStore((state) => state.updateJobs.byAgent);

  useEffect(() => {
    let cancelled = false;
    listAgentUpdateJobs({ cache: "no-store" })
      .then((response) => {
        if (cancelled) return;
        for (const job of response.jobs) {
          store.getState().upsertAgentUpdateJob(job);
        }
      })
      .catch(() => {
        // Retained jobs are an enhancement; live WebSocket events still populate state.
      });
    return () => {
      cancelled = true;
    };
  }, [store]);

  const hydrateConflict = useCallback(
    async (agentName: string, conflict: MaintenanceConflict) => {
      if (conflict.active_kind === "update") {
        const job = await getAgentUpdateJob(conflict.active_job_id, { cache: "no-store" });
        store.getState().upsertAgentUpdateJob(job);
        return;
      }
      const job = await getInstallJob(conflict.active_job_id, { cache: "no-store" });
      store.getState().upsertInstallJob(job.agent_name ? job : { ...job, agent_name: agentName });
    },
    [store],
  );

  const handleUpdate = useCallback(
    async (agentName: string) => {
      try {
        const job = await updateAgent(agentName);
        store.getState().upsertAgentUpdateJob(job);
      } catch (error) {
        const conflict = maintenanceConflict(error);
        if (conflict) {
          try {
            await hydrateConflict(agentName, conflict);
            return;
          } catch {
            // Surface a retryable error below if the retained conflict job vanished.
          }
        }
        const current = store.getState().updateJobs.byAgent[agentName];
        if (current && !["succeeded", "failed"].includes(current.status)) return;
        store.getState().upsertAgentUpdateJob({
          job_id: `local-error-${agentName}-${Date.now()}`,
          agent_name: agentName,
          status: "failed",
          error: error instanceof Error ? error.message : String(error),
          started_at: new Date().toISOString(),
        });
      }
    },
    [hydrateConflict, store],
  );

  return { updateJobs, handleUpdate };
}
