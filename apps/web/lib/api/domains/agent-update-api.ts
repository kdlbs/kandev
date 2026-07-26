import { fetchJson, type ApiRequestOptions } from "../client";

export type AgentUpdateJobStatus =
  | "queued"
  | "resolving"
  | "updating"
  | "refreshing"
  | "succeeded"
  | "failed";

export type AgentUpdateJob = {
  job_id: string;
  agent_name: string;
  status: AgentUpdateJobStatus;
  current_version?: string;
  target_version?: string;
  output?: string;
  error?: string;
  refresh_error?: string;
  started_at: string;
  finished_at?: string;
};

export async function updateAgent(
  agentName: string,
  options?: ApiRequestOptions,
): Promise<AgentUpdateJob> {
  return fetchJson<AgentUpdateJob>(`/api/v1/agent-update/${agentName}`, {
    ...options,
    init: { method: "POST", ...(options?.init ?? {}) },
  });
}

export async function listAgentUpdateJobs(
  options?: ApiRequestOptions,
): Promise<{ jobs: AgentUpdateJob[] }> {
  return fetchJson<{ jobs: AgentUpdateJob[] }>("/api/v1/agent-update/jobs", options);
}

export async function getAgentUpdateJob(
  jobId: string,
  options?: ApiRequestOptions,
): Promise<AgentUpdateJob> {
  return fetchJson<AgentUpdateJob>(`/api/v1/agent-update/jobs/${jobId}`, options);
}
