import { fetchJson, type ApiRequestOptions } from "../client";

export type AgentUpdateJobStatus =
  | "queued"
  | "resolving"
  | "updating"
  | "probing"
  | "saving"
  | "refreshing"
  | "succeeded"
  | "failed";

export type AgentUpdateOperation =
  | "update"
  | "rollback"
  | "repair"
  | "up_to_date"
  | "use_default"
  | "migrate";

export type AgentUpdateCheckState = "update_available" | "up_to_date" | "unknown";

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
  default_version?: string;
  active_version?: string;
  effective_version?: string;
  target_version?: string;
  target_family?: "v1" | "v2";
  runtime_revision?: number;
  migration?: boolean;
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
  default_version?: string;
  active_version?: string;
  effective_version?: string;
  target_version: string;
  family?: "v1" | "v2";
  source?: "managed" | "native";
  target_family?: "v1" | "v2";
  runtime_revision?: number;
  migration_available?: boolean;
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

export async function previewAgentUpdateToFamily(
  agentName: string,
  family: "v2",
  targetVersion?: string,
  options?: ApiRequestOptions,
): Promise<AgentUpdatePreview> {
  const params = new URLSearchParams({ target_family: family });
  if (targetVersion) params.set("target_version", targetVersion);
  return fetchJson<AgentUpdatePreview>(
    `/api/v1/agent-update/${encodeURIComponent(agentName)}/preview?${params}`,
    options,
  );
}

export async function previewAgentUpdateUseDefault(
  agentName: string,
  options?: ApiRequestOptions,
): Promise<AgentUpdatePreview> {
  return fetchJson<AgentUpdatePreview>(
    `/api/v1/agent-update/${encodeURIComponent(agentName)}/preview?use_default=true`,
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

export async function updateAgentToFamily(
  agentName: string,
  targetVersion: string,
  family: "v2",
  expectedRuntimeRevision: number,
  options?: ApiRequestOptions,
): Promise<AgentUpdateJob> {
  return fetchJson<AgentUpdateJob>(`/api/v1/agent-update/${encodeURIComponent(agentName)}`, {
    ...options,
    init: {
      method: "POST",
      body: JSON.stringify({
        target_version: targetVersion,
        target_family: family,
        expected_runtime_revision: expectedRuntimeRevision,
      }),
      ...(options?.init ?? {}),
    },
  });
}

export async function updateAgentUseDefault(
  agentName: string,
  options?: ApiRequestOptions,
): Promise<AgentUpdateJob> {
  return fetchJson<AgentUpdateJob>(`/api/v1/agent-update/${encodeURIComponent(agentName)}`, {
    ...options,
    init: {
      method: "POST",
      body: JSON.stringify({ use_default: true }),
      ...(options?.init ?? {}),
    },
  });
}

export type AgentUpdateStatus = {
  agent_name: string;
  package: string;
  default_version: string;
  active_version?: string;
  effective_version: string;
  latest_version?: string;
  checked_at?: string;
  check_state: AgentUpdateCheckState;
  family?: "v1" | "v2";
  source?: "managed" | "native";
  runtime_revision?: number;
  migration_available?: boolean;
};

export async function listAgentUpdateStatuses(
  options?: ApiRequestOptions,
): Promise<{ statuses: AgentUpdateStatus[] }> {
  return fetchJson<{ statuses: AgentUpdateStatus[] }>("/api/v1/agent-update/status", options);
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
