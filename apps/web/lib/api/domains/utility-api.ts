import { fetchJson, type ApiRequestOptions } from "../client";

// Types
export type UtilityAgent = {
  id: string;
  name: string;
  description: string;
  prompt: string;
  agent_id: string;
  model: string;
  builtin: boolean;
  enabled: boolean;
  created_at: string;
  updated_at: string;
};

export type UtilityAgentCall = {
  id: string;
  utility_id: string;
  session_id: string;
  resolved_prompt: string;
  response: string;
  model: string;
  prompt_tokens: number;
  response_tokens: number;
  duration_ms: number;
  status: string;
  error_message: string;
  created_at: string;
  completed_at?: string;
};

export type TemplateVariable = {
  name: string;
  description: string;
  example: string;
  category: string;
};

export type InferenceModel = {
  id: string;
  name: string;
  description: string;
  is_default: boolean;
};

export type InferenceAgent = {
  id: string;
  name: string;
  display_name: string;
  models: InferenceModel[] | null;
};

/**
 * AgentCapabilities represents the cached result of probing an agent type.
 * This comes from the host utility manager via /api/v1/agent-capabilities.
 */
export type AgentCapabilities = {
  agent_type: string;
  agent_name?: string;
  agent_version?: string;
  status: "probing" | "ok" | "auth_required" | "not_installed" | "failed" | "not_configured";
  error?: string;
  models?: Array<{ id: string; name: string; description?: string }>;
  current_model_id?: string;
  modes?: Array<{ id: string; name: string; description?: string }>;
  current_mode_id?: string;
  last_checked_at: string;
};

export type ExecutePromptRequest = {
  utility_agent_id: string;
  session_id?: string;
  git_diff?: string;
  commit_log?: string;
  changed_files?: string;
  diff_summary?: string;
  branch_name?: string;
  base_branch?: string;
  task_title?: string;
  task_description?: string;
  user_prompt?: string;
  conversation_history?: string;
};

export type ExecutePromptResponse = {
  success: boolean;
  call_id?: string;
  response?: string;
  model?: string;
  prompt_tokens?: number;
  response_tokens?: number;
  duration_ms?: number;
  error?: string;
};

// API Functions
export async function listUtilityAgents(
  options?: ApiRequestOptions,
): Promise<{ agents: UtilityAgent[] }> {
  return fetchJson<{ agents: UtilityAgent[] }>("/api/v1/utility/agents", options);
}

export async function getUtilityAgent(
  id: string,
  options?: ApiRequestOptions,
): Promise<UtilityAgent> {
  return fetchJson<UtilityAgent>(`/api/v1/utility/agents/${id}`, options);
}

export async function createUtilityAgent(
  data: Partial<UtilityAgent>,
  options?: ApiRequestOptions,
): Promise<UtilityAgent> {
  return fetchJson<UtilityAgent>("/api/v1/utility/agents", {
    ...options,
    init: { method: "POST", body: JSON.stringify(data), ...(options?.init ?? {}) },
  });
}

export async function updateUtilityAgent(
  id: string,
  data: Partial<UtilityAgent>,
  options?: ApiRequestOptions,
): Promise<UtilityAgent> {
  return fetchJson<UtilityAgent>(`/api/v1/utility/agents/${id}`, {
    ...options,
    init: { method: "PATCH", body: JSON.stringify(data), ...(options?.init ?? {}) },
  });
}

export async function deleteUtilityAgent(id: string, options?: ApiRequestOptions): Promise<void> {
  await fetchJson<{ success: boolean }>(`/api/v1/utility/agents/${id}`, {
    ...options,
    init: { method: "DELETE", ...(options?.init ?? {}) },
  });
}

export async function getTemplateVariables(
  options?: ApiRequestOptions,
): Promise<{ variables: TemplateVariable[] }> {
  return fetchJson<{ variables: TemplateVariable[] }>(
    "/api/v1/utility/template-variables",
    options,
  );
}

export async function listInferenceAgents(
  options?: ApiRequestOptions,
): Promise<{ agents: InferenceAgent[] }> {
  return fetchJson<{ agents: InferenceAgent[] }>("/api/v1/utility/inference-agents", options);
}

/**
 * Fetches all agent capabilities from the host utility capability cache.
 * This is the canonical source for models, modes, and other capabilities.
 */
export async function listAgentCapabilities(
  options?: ApiRequestOptions,
): Promise<{ agents: AgentCapabilities[] }> {
  return fetchJson<{ agents: AgentCapabilities[] }>("/api/v1/agent-capabilities", options);
}

export async function executeUtilityPrompt(
  req: ExecutePromptRequest,
  options?: ApiRequestOptions,
): Promise<ExecutePromptResponse> {
  return fetchJson<ExecutePromptResponse>("/api/v1/utility/execute", {
    ...options,
    init: { method: "POST", body: JSON.stringify(req), ...(options?.init ?? {}) },
  });
}

export async function listUtilityCalls(
  utilityId: string,
  limit = 50,
  options?: ApiRequestOptions,
): Promise<{ calls: UtilityAgentCall[] }> {
  return fetchJson<{ calls: UtilityAgentCall[] }>(
    `/api/v1/utility/agents/${utilityId}/calls?limit=${limit}`,
    options,
  );
}
