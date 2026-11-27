import { readExecutorCatalogue } from "../executor-catalogue";
import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  ListExecutorsResponse,
  ListExecutorProfilesResponse,
  ExecutorProfile,
  ProfileEnvVar,
} from "@/lib/types/http";

// Executors
export async function listExecutors(options?: ApiRequestOptions): Promise<ListExecutorsResponse> {
  return readExecutorCatalogue(() =>
    fetchJson<ListExecutorsResponse>("/api/v1/executors", options),
  );
}

export async function fetchExecutor(
  executorId: string,
  options?: ApiRequestOptions,
): Promise<{
  id: string;
  name: string;
  type: string;
  owner_user_id?: string | null;
  config?: Record<string, string>;
}> {
  return readExecutorCatalogue(() =>
    fetchJson<{
      id: string;
      name: string;
      type: string;
      owner_user_id?: string | null;
      config?: Record<string, string>;
    }>(`/api/v1/executors/${executorId}`, options),
  );
}

export async function createExecutor(
  payload: {
    name: string;
    owner_user_id?: string | null;
    type: string;
    config?: Record<string, string>;
  },
  options?: ApiRequestOptions,
): Promise<{
  id: string;
  name: string;
  type: string;
  owner_user_id?: string | null;
  config?: Record<string, string>;
}> {
  return fetchJson<{
    id: string;
    name: string;
    type: string;
    owner_user_id?: string | null;
    config?: Record<string, string>;
  }>("/api/v1/executors", {
    ...options,
    init: { method: "POST", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function updateExecutor(
  executorId: string,
  payload: { name?: string; owner_user_id?: string | null; config?: Record<string, string> },
  options?: ApiRequestOptions,
): Promise<void> {
  await fetchJson<void>(`/api/v1/executors/${executorId}`, {
    ...options,
    init: { method: "PATCH", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function deleteExecutor(
  executorId: string,
  options?: ApiRequestOptions,
): Promise<{ success: boolean }> {
  return fetchJson<{ success: boolean }>(`/api/v1/executors/${encodeURIComponent(executorId)}`, {
    ...options,
    init: { ...(options?.init ?? {}), method: "DELETE" },
  });
}

// Executor profiles
export async function listExecutorProfiles(
  executorId: string,
  options?: ApiRequestOptions,
): Promise<ListExecutorProfilesResponse> {
  return readExecutorCatalogue(() =>
    fetchJson<ListExecutorProfilesResponse>(`/api/v1/executors/${executorId}/profiles`, options),
  );
}

export async function listAllExecutorProfiles(
  options?: ApiRequestOptions,
): Promise<ListExecutorProfilesResponse> {
  return readExecutorCatalogue(() =>
    fetchJson<ListExecutorProfilesResponse>("/api/v1/executor-profiles", options),
  );
}

export type ScriptPlaceholder = {
  key: string;
  description: string;
  example: string;
  executor_types: string[];
};

export async function listScriptPlaceholders(
  options?: ApiRequestOptions,
): Promise<{ placeholders: ScriptPlaceholder[] }> {
  return fetchJson<{ placeholders: ScriptPlaceholder[] }>("/api/v1/script-placeholders", options);
}

export async function fetchDefaultScripts(
  executorType: string,
  options?: ApiRequestOptions,
): Promise<{ prepare_script: string; cleanup_script: string }> {
  return fetchJson<{ prepare_script: string; cleanup_script: string }>(
    `/api/v1/executor-profiles/default-script?type=${encodeURIComponent(executorType)}`,
    options,
  );
}

export async function createExecutorProfile(
  executorId: string,
  payload: {
    name: string;
    owner_user_id?: string | null;
    mcp_policy?: string;
    config?: Record<string, string>;
    prepare_script?: string;
    cleanup_script?: string;
    env_vars?: ProfileEnvVar[];
  },
  options?: ApiRequestOptions,
): Promise<ExecutorProfile> {
  return fetchJson<ExecutorProfile>(`/api/v1/executors/${executorId}/profiles`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function updateExecutorProfile(
  executorId: string,
  profileId: string,
  payload: {
    name?: string;
    owner_user_id?: string | null;
    mcp_policy?: string;
    config?: Record<string, string>;
    prepare_script?: string;
    cleanup_script?: string;
    env_vars?: ProfileEnvVar[];
  },
  options?: ApiRequestOptions,
): Promise<ExecutorProfile> {
  return fetchJson<ExecutorProfile>(`/api/v1/executors/${executorId}/profiles/${profileId}`, {
    ...options,
    init: { method: "PATCH", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function deleteExecutorProfile(
  executorId: string,
  profileId: string,
  options?: ApiRequestOptions,
): Promise<{ success: boolean }> {
  return fetchJson<{ success: boolean }>(`/api/v1/executors/${executorId}/profiles/${profileId}`, {
    ...options,
    init: { method: "DELETE", ...(options?.init ?? {}) },
  });
}
