import { fetchJson, type ApiRequestOptions } from "../client";
import { t } from "@/lib/i18n";
import { PROMPT_SYNC_RATE_LIMIT_ERROR_CODE } from "@/lib/types/prompt-sync";
import type {
  PromptConflictResolutions,
  PromptSyncCheckResponse,
  PromptSyncConfig,
  PromptSyncRemoval,
  PromptSyncRemovalReview,
  PromptSyncRunResponse,
  PromptSyncSetConfigRequest,
  PromptVersionsSave,
} from "@/lib/types/prompt-sync";
import type { CustomPrompt } from "@/lib/types/http";

const CONFIG_PATH = "/api/v1/prompt-sync/config";

function localizeRunResponse(response: PromptSyncRunResponse): PromptSyncRunResponse {
  if (response.error_code !== PROMPT_SYNC_RATE_LIMIT_ERROR_CODE) return response;
  return { ...response, error: t("settings:promptSyncQuotaUnavailable") };
}

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
  const response = await fetchJson<PromptSyncRunResponse>(CONFIG_PATH, {
    ...options,
    init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify(payload) },
  });
  return localizeRunResponse(response);
}

// deletePromptSyncConfig removes Prompt Sync with the user's removal choices:
// which version each prompt with both versions keeps (others keep the one in
// use), and whether prompts that only came from the repository are deleted.
export async function deletePromptSyncConfig(
  removal?: PromptSyncRemoval,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ deleted: boolean }>(CONFIG_PATH, {
    ...options,
    init: {
      ...(options?.init ?? {}),
      method: "DELETE",
      ...(removal ? { body: JSON.stringify(removal) } : {}),
    },
  });
}

// getPromptSyncKeptVersions lists what removing Prompt Sync asks about:
// prompts that have both a local and a repository version, each with the
// version it uses now, and the prompts that only came from the repository.
export async function getPromptSyncKeptVersions(options?: ApiRequestOptions) {
  return fetchJson<PromptSyncRemovalReview>("/api/v1/prompt-sync/kept-versions", options);
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
  const response = await fetchJson<PromptSyncRunResponse>("/api/v1/prompt-sync/sync", {
    ...options,
    init: {
      ...(options?.init ?? {}),
      method: "POST",
      ...(resolutions ? { body: JSON.stringify({ resolutions }) } : {}),
    },
  });
  return localizeRunResponse(response);
}

// savePromptVersions saves a synced prompt's or override's Local override
// switch together with any edit to its local version, in one write.
export async function savePromptVersions(
  promptId: string,
  payload: PromptVersionsSave,
  options?: ApiRequestOptions,
) {
  return fetchJson<CustomPrompt>(
    `/api/v1/prompt-sync/prompts/${encodeURIComponent(promptId)}/versions`,
    {
      ...options,
      init: { ...(options?.init ?? {}), method: "PUT", body: JSON.stringify(payload) },
    },
  );
}
