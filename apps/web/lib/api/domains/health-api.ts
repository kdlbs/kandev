import { fetchJson, type ApiRequestOptions } from "../client";
import type { SystemHealthResponse } from "@/lib/types/health";

export async function fetchSystemHealth(options?: ApiRequestOptions) {
  return fetchJson<SystemHealthResponse>("/api/v1/system/health", options);
}
