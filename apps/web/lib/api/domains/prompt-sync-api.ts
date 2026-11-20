import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  PromptSyncConfig,
  PromptSyncRunResponse,
  PromptSyncSetConfigRequest,
} from "@/lib/types/prompt-sync";
import type { CustomPrompt } from "@/lib/types/http";

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

function promptActionPath(promptId: string, action: "override" | "revert") {
  return `/api/v1/prompt-sync/prompts/${encodeURIComponent(promptId)}/${action}`;
}

// overridePrompt turns a synced prompt into a local, editable override.
export async function overridePrompt(promptId: string, options?: ApiRequestOptions) {
  return fetchJson<CustomPrompt>(promptActionPath(promptId, "override"), {
    ...options,
    init: { ...(options?.init ?? {}), method: "POST" },
  });
}

// revertPromptOverride returns an override to sync and re-syncs it.
export async function revertPromptOverride(promptId: string, options?: ApiRequestOptions) {
  return fetchJson<CustomPrompt>(promptActionPath(promptId, "revert"), {
    ...options,
    init: { ...(options?.init ?? {}), method: "POST" },
  });
}
