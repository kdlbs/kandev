import { fetchJson, type ApiRequestOptions } from "../client";
import type { RuntimeFlagsResponse } from "@/lib/types/runtime-flags";

const RUNTIME_FLAGS_BASE = "/api/v1/runtime-flags";

export function fetchRuntimeFlags(options?: ApiRequestOptions): Promise<RuntimeFlagsResponse> {
  return fetchJson<RuntimeFlagsResponse>(RUNTIME_FLAGS_BASE, {
    cache: "no-store",
    ...options,
  });
}

export function updateRuntimeFlag(
  key: string,
  override: boolean | null,
  options?: ApiRequestOptions,
): Promise<RuntimeFlagsResponse> {
  return fetchJson<RuntimeFlagsResponse>(`${RUNTIME_FLAGS_BASE}/${encodeURIComponent(key)}`, {
    ...options,
    init: {
      method: "PATCH",
      body: JSON.stringify({ override }),
      ...(options?.init ?? {}),
    },
  });
}
