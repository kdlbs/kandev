import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  SecretListItem,
  CreateSecretRequest,
  UpdateSecretRequest,
  RevealSecretResponse,
  SecretScope,
} from "@/lib/types/http-secrets";

export type SecretListOptions = ApiRequestOptions & {
  scope?: SecretScope;
  workspaceId?: string;
  includeGlobal?: boolean;
};

export type SecretScopedOptions = ApiRequestOptions & {
  workspaceId?: string;
};

function withSecretQuery(
  path: string,
  options?: { scope?: SecretScope; workspaceId?: string; includeGlobal?: boolean },
) {
  const query = new URLSearchParams();
  if (options?.scope) query.set("scope", options.scope);
  if (options?.workspaceId) query.set("workspace_id", options.workspaceId);
  if (options?.includeGlobal) query.set("include_global", "true");
  const suffix = query.toString();
  return suffix ? `${path}?${suffix}` : path;
}

export async function listSecrets(options?: SecretListOptions): Promise<SecretListItem[]> {
  const { scope, workspaceId, includeGlobal, ...requestOptions } = options ?? {};
  return fetchJson<SecretListItem[]>(
    withSecretQuery("/api/v1/secrets", { scope, workspaceId, includeGlobal }),
    requestOptions,
  );
}

export async function createSecret(
  payload: CreateSecretRequest,
  options?: ApiRequestOptions,
): Promise<SecretListItem> {
  return fetchJson<SecretListItem>("/api/v1/secrets", {
    ...options,
    init: { method: "POST", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function updateSecret(
  id: string,
  payload: UpdateSecretRequest,
  options?: SecretScopedOptions,
): Promise<SecretListItem> {
  return fetchJson<SecretListItem>(withSecretQuery(`/api/v1/secrets/${id}`, options), {
    ...options,
    init: { method: "PUT", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function deleteSecret(id: string, options?: SecretScopedOptions): Promise<void> {
  return fetchJson<void>(withSecretQuery(`/api/v1/secrets/${id}`, options), {
    ...options,
    init: { method: "DELETE", ...(options?.init ?? {}) },
  });
}

export async function revealSecret(
  id: string,
  options?: SecretScopedOptions,
): Promise<RevealSecretResponse> {
  return fetchJson<RevealSecretResponse>(withSecretQuery(`/api/v1/secrets/${id}/reveal`, options), {
    ...options,
    init: { method: "POST", ...(options?.init ?? {}) },
  });
}
