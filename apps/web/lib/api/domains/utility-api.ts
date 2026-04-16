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
  /**
   * Agent-specific extras from ACP's `_meta` field. GitHub Copilot exposes
   * `copilotUsage` (e.g. "1x", "0.33x", "0x" — the premium-request
   * multiplier) which the model combobox renders as a cost badge. Shape
   * matches `ModelEntry.meta` so the same `<ModelCombobox>` accepts this.
   */
  meta?: Record<string, unknown>;
};

export type InferenceAgent = {
  id: string;
  name: string;
  display_name: string;
  models: InferenceModel[];
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
