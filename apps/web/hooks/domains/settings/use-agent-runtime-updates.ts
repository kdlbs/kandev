"use client";

import { useCallback } from "react";

import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import {
  getAgentUpdateJob,
  getInstallJob,
  previewAgentUpdate,
  previewAgentUpdateToFamily,
  previewAgentUpdateUseDefault,
  updateAgent,
  updateAgentToFamily,
  updateAgentUseDefault,
  type AgentUpdatePreview,
  type AgentUpdateJob,
  type AgentUpdateMode,
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

async function requestRuntimeUpdate(
  agentName: string,
  targetVersion: string,
  useDefault: boolean,
  targetFamily?: "v2" | AgentUpdateMode,
  expectedRuntimeRevision?: number,
): Promise<AgentUpdateJob> {
  if (targetFamily === "self_update") return updateAgent(agentName, { update_mode: "self_update" });
  if (useDefault) return updateAgentUseDefault(agentName);
  if (targetFamily === "v2") {
    return updateAgentToFamily(
      agentName,
      targetVersion,
      targetFamily,
      expectedRuntimeRevision ?? 0,
    );
  }
  return updateAgent(agentName, { update_mode: "pinned", target_version: targetVersion });
}

function requestRuntimeUpdatePreview(
  agentName: string,
  targetVersion?: string,
  useDefault = false,
  targetFamily?: "v2",
): Promise<AgentUpdatePreview> {
  if (useDefault) return previewAgentUpdateUseDefault(agentName, { cache: "no-store" });
  if (targetFamily === "v2") {
    return previewAgentUpdateToFamily(agentName, targetFamily, targetVersion, {
      cache: "no-store",
    });
  }
  return previewAgentUpdate(agentName, targetVersion, { cache: "no-store" });
}

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
    async (
      agentName: string,
      targetVersion: string,
      useDefault = false,
      targetFamily?: "v2" | AgentUpdateMode,
      expectedRuntimeRevision?: number,
    ) => {
      try {
        const job = await requestRuntimeUpdate(
          agentName,
          targetVersion,
          useDefault,
          targetFamily,
          expectedRuntimeRevision,
        );
        if (job.job_id) store.getState().upsertAgentUpdateJob(job);
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
    (agentName: string, targetVersion?: string, useDefault = false, targetFamily?: "v2") =>
      requestRuntimeUpdatePreview(agentName, targetVersion, useDefault, targetFamily),
    [],
  );

  return { updateJobs, previewUpdate, startUpdate };
}
