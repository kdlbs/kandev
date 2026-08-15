import { fetchJson, type ApiRequestOptions } from "../client";

export type AgentUpdateJobStatus =
  | "queued"
  | "resolving"
  | "updating"
  | "refreshing"
  | "succeeded"
  | "failed";

export type AgentUpdateOperation = "update" | "rollback" | "repair" | "up_to_date";

export type AgentUpdateVersion = {
  version: string;
  latest: boolean;
};

export type AgentUpdateJob = {
  job_id: string;
  agent_name: string;
  status: AgentUpdateJobStatus;
  operation?: AgentUpdateOperation;
  current_version?: string;
  active_version?: string;
  target_version?: string;
  output?: string;
  error?: string;
  refresh_error?: string;
  started_at: string;
  finished_at?: string;
};

export type AgentUpdatePreview = {
  agent_name: string;
  package: string;
  current_version?: string;
  active_version?: string;
  target_version: string;
  operation?: AgentUpdateOperation;
  available_versions?: AgentUpdateVersion[];
  command: string[];
  command_string: string;
};

export async function previewAgentUpdate(
  agentName: string,
  targetVersion?: string,
  options?: ApiRequestOptions,
): Promise<AgentUpdatePreview> {
  const query = targetVersion ? `?${new URLSearchParams({ target_version: targetVersion })}` : "";
  return fetchJson<AgentUpdatePreview>(
    `/api/v1/agent-update/${encodeURIComponent(agentName)}/preview${query}`,
    options,
  );
}

export async function updateAgent(
  agentName: string,
  targetVersion: string,
  options?: ApiRequestOptions,
): Promise<AgentUpdateJob> {
  return fetchJson<AgentUpdateJob>(`/api/v1/agent-update/${encodeURIComponent(agentName)}`, {
    ...options,
    init: {
      method: "POST",
      body: JSON.stringify({ target_version: targetVersion }),
      ...(options?.init ?? {}),
    },
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
