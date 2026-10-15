import { fetchJson, type ApiRequestOptions } from "@/lib/api/client";

export type DynamicTierPreview = {
  state: "selected" | "no_candidates" | "no_eligible" | "disabled";
  candidate_id?: string;
  observed_at: string;
  considered: Array<{
    candidate_id: string;
    tier: number;
    mode: string;
    cost_class: string;
    eligible: boolean;
    reason?: string;
    selected: boolean;
  }>;
};

export function previewDynamicTiers(
  profileId: string | undefined,
  dynamic: unknown,
  options?: ApiRequestOptions,
) {
  const path = profileId
    ? `/api/v1/agent-profiles/${profileId}/dynamic-preview`
    : "/api/v1/agent-profiles/dynamic-preview";
  return fetchJson<DynamicTierPreview>(path, {
    ...options,
    init: { ...options?.init, method: "POST", body: JSON.stringify({ dynamic }) },
  });
}
