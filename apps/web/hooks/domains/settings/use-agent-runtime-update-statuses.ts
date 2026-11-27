"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import type { AgentUpdateJob } from "@/lib/api";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { refreshRuntimeUpdateStatuses } from "@/lib/agents/runtime-update-statuses";

const NO_JOBS: Record<string, AgentUpdateJob> = {};

export function useAgentRuntimeUpdateStatuses(
  updateJobs: Record<string, AgentUpdateJob> = NO_JOBS,
) {
  const store = useAppStoreApi();
  const statusByAgent = useAppStore((s) => s.agentRuntimeUpdates.byAgent);
  const loading = useAppStore((s) => s.agentRuntimeUpdates.loading);
  const [refreshFailed, setRefreshFailed] = useState(false);
  const observedTerminalJobs = useRef(new Set<string>());
  const pendingTerminalJobs = useRef(new Set<string>());

  const refresh = useCallback(
    () =>
      refreshRuntimeUpdateStatuses(store, true).then((succeeded) => {
        setRefreshFailed(!succeeded);
        return succeeded;
      }),
    [store],
  );
  useEffect(() => {
    void refreshRuntimeUpdateStatuses(store).then((succeeded) => {
      if (!succeeded) setRefreshFailed(true);
    });
  }, [store]);

  useEffect(() => {
    for (const job of Object.values(updateJobs)) {
      if (
        (job.status !== "succeeded" && job.status !== "failed") ||
        observedTerminalJobs.current.has(job.job_id)
      ) {
        continue;
      }
      if (pendingTerminalJobs.current.has(job.job_id)) {
        continue;
      }
      pendingTerminalJobs.current.add(job.job_id);
      void refresh().then((succeeded) => {
        pendingTerminalJobs.current.delete(job.job_id);
        if (succeeded) {
          observedTerminalJobs.current.add(job.job_id);
        }
      });
    }
  }, [refresh, updateJobs]);

  return { refresh, statusByAgent, loading, refreshFailed };
}
