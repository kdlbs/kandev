import { fetchJson, fetchJsonWithRetry, type ApiRequestOptions } from "../client";
import type { SleepInhibitionResponse, SleepInhibitionSettings } from "@/lib/types/system";

const SLEEP_INHIBITION_PATH = "/api/v1/system/sleep-inhibition";

export async function fetchSleepInhibitionSettings(
  options?: ApiRequestOptions,
): Promise<SleepInhibitionResponse> {
  return fetchJsonWithRetry<SleepInhibitionResponse>(SLEEP_INHIBITION_PATH, {
    ...options,
    cache: "no-store",
  });
}

export async function updateSleepInhibitionSettings(
  payload: SleepInhibitionSettings,
  options?: ApiRequestOptions,
): Promise<SleepInhibitionResponse> {
  return fetchJson<SleepInhibitionResponse>(SLEEP_INHIBITION_PATH, {
    ...options,
    init: {
      ...(options?.init ?? {}),
      method: "PATCH",
      body: JSON.stringify(payload),
    },
  });
}
