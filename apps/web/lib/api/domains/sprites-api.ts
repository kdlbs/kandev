import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  SpritesStatus,
  SpritesInstance,
  SpritesTestResult,
} from "@/lib/types/http-sprites";

function withSecretId(url: string, secretId?: string): string {
  if (!secretId) return url;
  const sep = url.includes("?") ? "&" : "?";
  return `${url}${sep}secret_id=${encodeURIComponent(secretId)}`;
}

export async function getSpritesStatus(
  secretId?: string,
  options?: ApiRequestOptions,
): Promise<SpritesStatus> {
  return fetchJson<SpritesStatus>(withSecretId("/api/v1/sprites/status", secretId), options);
}

export async function listSpritesInstances(
  secretId?: string,
  options?: ApiRequestOptions,
): Promise<SpritesInstance[]> {
  return fetchJson<SpritesInstance[]>(withSecretId("/api/v1/sprites/instances", secretId), options);
}

export async function destroySprite(
  name: string,
  secretId?: string,
  options?: ApiRequestOptions,
): Promise<void> {
  return fetchJson<void>(withSecretId(`/api/v1/sprites/instances/${encodeURIComponent(name)}`, secretId), {
    ...options,
    init: { method: "DELETE", ...(options?.init ?? {}) },
  });
}

export async function destroyAllSprites(
  secretId?: string,
  options?: ApiRequestOptions,
): Promise<{ success: boolean; destroyed: number }> {
  return fetchJson<{ success: boolean; destroyed: number }>(withSecretId("/api/v1/sprites/instances", secretId), {
    ...options,
    init: { method: "DELETE", ...(options?.init ?? {}) },
  });
}

export async function testSpritesConnection(
  secretId?: string,
  options?: ApiRequestOptions,
): Promise<SpritesTestResult> {
  return fetchJson<SpritesTestResult>(withSecretId("/api/v1/sprites/test", secretId), {
    ...options,
    init: { method: "POST", ...(options?.init ?? {}) },
  });
}
