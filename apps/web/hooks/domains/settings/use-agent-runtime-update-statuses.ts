"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { listAgentUpdateStatuses, type AgentUpdateJob, type AgentUpdateStatus } from "@/lib/api";

type StatusByAgent = Record<string, AgentUpdateStatus>;

function indexStatuses(statuses: AgentUpdateStatus[]): StatusByAgent {
  return Object.fromEntries(statuses.map((status) => [status.agent_name, status]));
}

export function useAgentRuntimeUpdateStatuses(updateJobs: Record<string, AgentUpdateJob>) {
  const [statusByAgent, setStatusByAgent] = useState<StatusByAgent>({});
  const observedSuccessfulJobs = useRef(new Set<string>());
  const pendingSuccessfulJobs = useRef(new Set<string>());

  const refresh = useCallback(async (): Promise<boolean> => {
    try {
      const response = await listAgentUpdateStatuses({ cache: "no-store" });
      setStatusByAgent(indexStatuses(response.statuses));
      return true;
    } catch {
      // A status hint is advisory. Keep the last good map so a transient
      // registry failure does not remove an already visible hint or disable
      // the authoritative update control.
      return false;
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    for (const job of Object.values(updateJobs)) {
      if (job.status !== "succeeded" || observedSuccessfulJobs.current.has(job.job_id)) {
        continue;
      }
      if (pendingSuccessfulJobs.current.has(job.job_id)) {
        continue;
      }
      pendingSuccessfulJobs.current.add(job.job_id);
      void refresh().then((succeeded) => {
        pendingSuccessfulJobs.current.delete(job.job_id);
        if (succeeded) {
          observedSuccessfulJobs.current.add(job.job_id);
        }
      });
    }
  }, [refresh, updateJobs]);

  return { refresh, statusByAgent };
}
