import { fetchJson, type ApiRequestOptions } from "../client";

export type AgentUpdateJobStatus =
  | "queued"
  | "resolving"
  | "updating"
  | "refreshing"
  | "succeeded"
  | "failed";

export type AgentUpdateOperation = "update" | "rollback" | "repair" | "up_to_date" | "use_default";
export type AgentUpdateMode = "pinned" | "self_update";

export type AgentUpdateCheckState = "update_available" | "up_to_date" | "unknown";

export type AgentUpdateVersion = {
  version: string;
  latest: boolean;
};

export type AgentUpdateJob = {
  update_mode?: AgentUpdateMode;
  runtime_id?: string;
  job_id: string;
  automatic?: boolean;
  previous_version?: string;
  agent_name: string;
  status: AgentUpdateJobStatus;
  operation?: AgentUpdateOperation | "";
  current_version?: string;
  default_version?: string;
  active_version?: string;
  effective_version?: string;
  target_version?: string;
  output?: string;
  error?: string;
  refresh_error?: string;
  started_at: string;
  finished_at?: string;
};

export type AgentUpdatePreview = {
  update_mode: AgentUpdateMode;
  managed_fallback?: boolean;
  agent_name: string;
  package: string;
  current_version?: string;
  default_version?: string;
  active_version?: string;
  effective_version?: string;
  target_version: string;
  stable_latest_version?: string;
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

export async function previewAgentUpdateUseDefault(
  agentName: string,
  options?: ApiRequestOptions,
): Promise<AgentUpdatePreview> {
  return fetchJson<AgentUpdatePreview>(
    `/api/v1/agent-update/${encodeURIComponent(agentName)}/preview?use_default=true`,
    options,
  );
}

type AgentUpdateRequest =
  | { update_mode: "pinned"; target_version: string }
  | { update_mode: "self_update" };

export async function updateAgent(
  agentName: string,
  request: AgentUpdateRequest,
  options?: ApiRequestOptions,
): Promise<AgentUpdateJob> {
  return fetchJson<AgentUpdateJob>(`/api/v1/agent-update/${encodeURIComponent(agentName)}`, {
    ...options,
    init: {
      method: "POST",
      body: JSON.stringify(
        request.update_mode === "self_update" ? {} : { target_version: request.target_version },
      ),
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

export type AgentRuntimeOutcome = {
  id: string;
  status: "running" | "succeeded" | "failed" | "interrupted";
  previous_version: string;
  target_version: string;
  finished_at: string;
};

export type AgentUpdateStatus = {
  update_mode?: AgentUpdateMode;
  managed_fallback?: boolean;
  display_name: string;
  runtime_id: string;
  owner: "kandev" | "external" | "none";
  mechanism: string;
  management: "managed" | "manual" | "unsupported";
  source?: string;
  guidance_url?: string;
  current_version?: string;
  available: boolean;
  enabled: boolean;
  auto_update_supported: boolean;
  auto_update: boolean;
  last_outcome?: AgentRuntimeOutcome;
  agent_name: string;
  package: string;
  default_version: string;
  active_version?: string;
  effective_version: string;
  latest_version?: string;
  checked_at?: string;
  check_state: AgentUpdateCheckState;
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

export async function setAgentAutomaticUpdates(agentName: string, enabled: boolean): Promise<void> {
  await fetchJson(`/api/v1/agent-update/${encodeURIComponent(agentName)}/automatic`, {
    init: { method: "PATCH", body: JSON.stringify({ enabled }) },
  });
}
