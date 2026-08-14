import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  SecretListItem,
  CreateSecretRequest,
  UpdateSecretRequest,
  CopyMoveSecretRequest,
  RevealSecretResponse,
  SecretScope,
} from "@/lib/types/http-secrets";

/** Options for listing secrets: optional scope, workspace, and include-global filter. */
export type SecretListOptions = ApiRequestOptions & {
  scope?: SecretScope;
  workspaceId?: string;
  includeGlobal?: boolean;
};

/** Common options that scope a secrets request to a workspace. */
export type SecretScopedOptions = ApiRequestOptions & {
  workspaceId?: string;
};

/** Appends scope/workspace/includeGlobal query parameters to a secrets API path. */
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

/** Lists secrets, optionally filtered by scope, workspace, and global inclusion. */
export async function listSecrets(options?: SecretListOptions): Promise<SecretListItem[]> {
  const { scope, workspaceId, includeGlobal, ...requestOptions } = options ?? {};
  return fetchJson<SecretListItem[]>(
    withSecretQuery("/api/v1/secrets", { scope, workspaceId, includeGlobal }),
    requestOptions,
  );
}

/** Creates a secret from the given payload and returns the new item. */
export async function createSecret(
  payload: CreateSecretRequest,
  options?: ApiRequestOptions,
): Promise<SecretListItem> {
  return fetchJson<SecretListItem>("/api/v1/secrets", {
    ...options,
    init: { method: "POST", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

/** Updates the secret with the given id and returns the updated item. */
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

/** Deletes the secret with the given id. */
export async function deleteSecret(id: string, options?: SecretScopedOptions): Promise<void> {
  return fetchJson<void>(withSecretQuery(`/api/v1/secrets/${id}`, options), {
    ...options,
    init: { method: "DELETE", ...(options?.init ?? {}) },
  });
}

/** Reveals the value of the secret with the given id. */
export async function revealSecret(
  id: string,
  options?: SecretScopedOptions,
): Promise<RevealSecretResponse> {
  return fetchJson<RevealSecretResponse>(withSecretQuery(`/api/v1/secrets/${id}/reveal`, options), {
    ...options,
    init: { method: "POST", ...(options?.init ?? {}) },
  });
}

/**
 * Copies a secret into another scope/workspace. The `workspaceId` option is
 * the SOURCE workspace for a workspace-scoped source. Errors surface as
 * `ApiError` with the HTTP status (409 = target name conflict).
 */
export async function copySecret(
  id: string,
  payload: CopyMoveSecretRequest,
  options?: SecretScopedOptions,
): Promise<SecretListItem> {
  return fetchJson<SecretListItem>(withSecretQuery(`/api/v1/secrets/${id}/copy`, options), {
    ...options,
    init: { method: "POST", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

/** Moves a secret into another scope/workspace, removing the source. */
export async function moveSecret(
  id: string,
  payload: CopyMoveSecretRequest,
  options?: SecretScopedOptions,
): Promise<SecretListItem> {
  return fetchJson<SecretListItem>(withSecretQuery(`/api/v1/secrets/${id}/move`, options), {
    ...options,
    init: { method: "POST", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}
