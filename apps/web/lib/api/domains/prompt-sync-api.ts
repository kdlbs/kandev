import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  PromptSyncConfig,
  PromptSyncRunResponse,
  PromptSyncSetConfigRequest,
} from "@/lib/types/prompt-sync";

const CONFIG_PATH = "/api/v1/prompt-sync/config";

// getPromptSyncConfig returns null when the backend responds 204 (not
// configured); fetchJson maps 204 to undefined.
export async function getPromptSyncConfig(
  options?: ApiRequestOptions,
): Promise<PromptSyncConfig | null> {
  const res = await fetchJson<PromptSyncConfig | undefined>(CONFIG_PATH, options);
  return res ?? null;
}

export async function setPromptSyncConfig(
  payload: PromptSyncSetConfigRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<PromptSyncConfig>(CONFIG_PATH, {
    ...options,
    init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify(payload) },
  });
}

export async function deletePromptSyncConfig(options?: ApiRequestOptions) {
  return fetchJson<{ deleted: boolean }>(CONFIG_PATH, {
    ...options,
    init: { ...(options?.init ?? {}), method: "DELETE" },
  });
}

// runPromptSync triggers an immediate sync. It rejects (404) only when no
// configuration exists; a failed attempt resolves with `error` set.
export async function runPromptSync(options?: ApiRequestOptions) {
  return fetchJson<PromptSyncRunResponse>("/api/v1/prompt-sync/sync", {
    ...options,
    init: { ...(options?.init ?? {}), method: "POST" },
  });
}
