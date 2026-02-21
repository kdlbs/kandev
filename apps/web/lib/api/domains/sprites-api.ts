import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  SpritesStatus,
  SpritesInstance,
  SpritesTestResult,
} from "@/lib/types/http-sprites";

export async function getSpritesStatus(
  options?: ApiRequestOptions,
): Promise<SpritesStatus> {
  return fetchJson<SpritesStatus>("/api/v1/sprites/status", options);
}

export async function listSpritesInstances(
  options?: ApiRequestOptions,
): Promise<SpritesInstance[]> {
  return fetchJson<SpritesInstance[]>("/api/v1/sprites/instances", options);
}

export async function destroySprite(
  name: string,
  options?: ApiRequestOptions,
): Promise<void> {
  return fetchJson<void>(`/api/v1/sprites/instances/${encodeURIComponent(name)}`, {
    ...options,
    init: { method: "DELETE", ...(options?.init ?? {}) },
  });
}

export async function destroyAllSprites(
  options?: ApiRequestOptions,
): Promise<{ success: boolean; destroyed: number }> {
  return fetchJson<{ success: boolean; destroyed: number }>("/api/v1/sprites/instances", {
    ...options,
    init: { method: "DELETE", ...(options?.init ?? {}) },
  });
}

export async function testSpritesConnection(
  options?: ApiRequestOptions,
): Promise<SpritesTestResult> {
  return fetchJson<SpritesTestResult>("/api/v1/sprites/test", {
    ...options,
    init: { method: "POST", ...(options?.init ?? {}) },
  });
}
