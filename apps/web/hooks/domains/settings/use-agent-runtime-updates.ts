"use client";

import { useCallback } from "react";

import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import {
  getAgentUpdateJob,
  getInstallJob,
  previewAgentUpdate,
  updateAgent,
  type AgentUpdateJob,
} from "@/lib/api";
import { ApiError } from "@/lib/api/client";
// Thrown from a callback rather than rendered as a literal, so this uses the
// module-level `t`, which resolves at call time. The message surfaces in the
// runtime-update dialog through `agents:unableToStartUpdate`.
import { t } from "@/lib/i18n";

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

  const hydrateConflict = useCallback(
    async (agentName: string, conflict: MaintenanceConflict): Promise<AgentUpdateJob> => {
      if (conflict.active_kind === "update") {
        const job = await getAgentUpdateJob(conflict.active_job_id, { cache: "no-store" });
        store.getState().upsertAgentUpdateJob(job);
        return job;
      }
      const job = await getInstallJob(conflict.active_job_id, { cache: "no-store" });
      store.getState().upsertInstallJob(job.agent_name ? job : { ...job, agent_name: agentName });
      throw new Error(t("agents:agentInstallAlreadyInProgress"));
    },
    [store],
  );

  const startUpdate = useCallback(
    async (agentName: string) => {
      try {
        const job = await updateAgent(agentName);
        store.getState().upsertAgentUpdateJob(job);
        return job;
      } catch (error) {
        const conflict = maintenanceConflict(error);
        if (conflict) {
          return hydrateConflict(agentName, conflict);
        }
        throw error;
      }
    },
    [hydrateConflict, store],
  );

  const previewUpdate = useCallback(
    (agentName: string) => previewAgentUpdate(agentName, { cache: "no-store" }),
    [],
  );

  return { updateJobs, previewUpdate, startUpdate };
}
