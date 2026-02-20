import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  SecretListItem,
  CreateSecretRequest,
  UpdateSecretRequest,
  RevealSecretResponse,
} from "@/lib/types/http-secrets";

export async function listSecrets(options?: ApiRequestOptions): Promise<SecretListItem[]> {
  return fetchJson<SecretListItem[]>("/api/v1/secrets", options);
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
  options?: ApiRequestOptions,
): Promise<SecretListItem> {
  return fetchJson<SecretListItem>(`/api/v1/secrets/${id}`, {
    ...options,
    init: { method: "PUT", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function deleteSecret(id: string, options?: ApiRequestOptions): Promise<void> {
  return fetchJson<void>(`/api/v1/secrets/${id}`, {
    ...options,
    init: { method: "DELETE", ...(options?.init ?? {}) },
  });
}

export async function revealSecret(
  id: string,
  options?: ApiRequestOptions,
): Promise<RevealSecretResponse> {
  return fetchJson<RevealSecretResponse>(`/api/v1/secrets/${id}/reveal`, {
    ...options,
    init: { method: "POST", ...(options?.init ?? {}) },
  });
}
