import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  SetSlackConfigRequest,
  SlackConfig,
  TestSlackConnectionResult,
} from "@/lib/types/slack";

type WorkspaceApiOptions = ApiRequestOptions & { workspaceId?: string };

function withWorkspace(path: string, options?: WorkspaceApiOptions): string {
  if (!options?.workspaceId) return path;
  const separator = path.includes("?") ? "&" : "?";
  return `${path}${separator}workspace_id=${encodeURIComponent(options.workspaceId)}`;
}

function requestOptions(options?: WorkspaceApiOptions): ApiRequestOptions | undefined {
  if (!options) return undefined;
  const { workspaceId: _workspaceId, ...rest } = options;
  return rest;
}

// getSlackConfig returns null when the backend responds 204 (no config yet).
export async function getSlackConfig(options?: WorkspaceApiOptions): Promise<SlackConfig | null> {
  const res = await fetchJson<SlackConfig | undefined>(
    withWorkspace(`/api/v1/slack/config`, options),
    requestOptions(options),
  );
  return res ?? null;
}

export async function setSlackConfig(
  payload: SetSlackConfigRequest,
  options?: WorkspaceApiOptions,
) {
  return fetchJson<SlackConfig>(withWorkspace(`/api/v1/slack/config`, options), {
    ...requestOptions(options),
    init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify(payload) },
  });
}

export async function deleteSlackConfig(options?: WorkspaceApiOptions) {
  return fetchJson<{ deleted: boolean }>(withWorkspace(`/api/v1/slack/config`, options), {
    ...requestOptions(options),
    init: { ...(options?.init ?? {}), method: "DELETE" },
  });
}

export async function testSlackConnection(
  payload: SetSlackConfigRequest,
  options?: WorkspaceApiOptions,
) {
  return fetchJson<TestSlackConnectionResult>(withWorkspace(`/api/v1/slack/config/test`, options), {
    ...requestOptions(options),
    init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify(payload) },
  });
}

// copySlackConfig copies the Slack config + credentials from the workspace in
// options (source) to targetWorkspaceId.
export async function copySlackConfig(targetWorkspaceId: string, options?: WorkspaceApiOptions) {
  return fetchJson<SlackConfig>(withWorkspace(`/api/v1/slack/config/copy`, options), {
    ...requestOptions(options),
    init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify({ targetWorkspaceId }) },
  });
}
