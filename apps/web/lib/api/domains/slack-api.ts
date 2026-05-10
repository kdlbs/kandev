import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  SetSlackConfigRequest,
  SlackConfig,
  TestSlackConnectionResult,
} from "@/lib/types/slack";

// getSlackConfig returns null when the backend responds 204 (no config yet).
export async function getSlackConfig(options?: ApiRequestOptions): Promise<SlackConfig | null> {
  const res = await fetchJson<SlackConfig | undefined>(`/api/v1/slack/config`, options);
  return res ?? null;
}

export async function setSlackConfig(payload: SetSlackConfigRequest, options?: ApiRequestOptions) {
  return fetchJson<SlackConfig>(`/api/v1/slack/config`, {
    ...options,
    init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify(payload) },
  });
}

export async function deleteSlackConfig(options?: ApiRequestOptions) {
  return fetchJson<{ deleted: boolean }>(`/api/v1/slack/config`, {
    ...options,
    init: { ...(options?.init ?? {}), method: "DELETE" },
  });
}

export async function testSlackConnection(
  payload: SetSlackConfigRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<TestSlackConnectionResult>(`/api/v1/slack/config/test`, {
    ...options,
    init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify(payload) },
  });
}
