import { fetchJson, type ApiRequestOptions } from "../client";
import type { DynamicModelsResponse, ProfileCapabilityRequest } from "@/lib/types/http";

export async function probeAgentProfile(
  agentName: string,
  payload: ProfileCapabilityRequest,
  options?: ApiRequestOptions,
): Promise<DynamicModelsResponse> {
  return fetchJson<DynamicModelsResponse>(`/api/v1/agent-models/${agentName}/probe`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}
