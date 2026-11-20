import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  PromptConflictResolutions,
  PromptSyncCheckResponse,
  PromptSyncConfig,
  PromptSyncConflict,
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

// setPromptSyncConfig saves the configuration and runs one sync with the
// given conflict choices; a failed sync still saves and resolves with `error`.
export async function setPromptSyncConfig(
  payload: PromptSyncSetConfigRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<PromptSyncRunResponse>(CONFIG_PATH, {
    ...options,
    init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify(payload) },
  });
}

// deletePromptSyncConfig removes Prompt Sync; synced prompts named in
// restoreLocal get their kept local version back.
export async function deletePromptSyncConfig(restoreLocal?: string[], options?: ApiRequestOptions) {
  return fetchJson<{ deleted: boolean }>(CONFIG_PATH, {
    ...options,
    init: {
      ...(options?.init ?? {}),
      method: "DELETE",
      ...(restoreLocal ? { body: JSON.stringify({ restore_local: restoreLocal }) } : {}),
    },
  });
}

// getPromptSyncKeptVersions lists synced prompts that keep a local version
// beside the repository version.
export async function getPromptSyncKeptVersions(options?: ApiRequestOptions) {
  return fetchJson<{ prompts: PromptSyncConflict[] }>("/api/v1/prompt-sync/kept-versions", options);
}

// checkPromptSync lists the conflicts a source would cause without saving or
// changing anything; without a payload it checks the saved configuration. It
// rejects when the source cannot be read.
export async function checkPromptSync(
  payload?: PromptSyncSetConfigRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<PromptSyncCheckResponse>("/api/v1/prompt-sync/check", {
    ...options,
    init: {
      ...(options?.init ?? {}),
      method: "POST",
      ...(payload ? { body: JSON.stringify(payload) } : {}),
    },
  });
}

// runPromptSync triggers an immediate sync, applying any conflict choices. It
// rejects (404) only when no configuration exists; a failed attempt resolves
// with `error` set.
export async function runPromptSync(
  resolutions?: PromptConflictResolutions,
  options?: ApiRequestOptions,
) {
  return fetchJson<PromptSyncRunResponse>("/api/v1/prompt-sync/sync", {
    ...options,
    init: {
      ...(options?.init ?? {}),
      method: "POST",
      ...(resolutions ? { body: JSON.stringify({ resolutions }) } : {}),
    },
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
