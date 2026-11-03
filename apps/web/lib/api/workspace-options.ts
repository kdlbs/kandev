import type { ApiRequestOptions } from "./client";

export type WorkspaceApiOptions = ApiRequestOptions & { workspaceId?: string };

export function withWorkspace(path: string, options?: WorkspaceApiOptions): string {
  if (!options?.workspaceId) return path;
  const separator = path.includes("?") ? "&" : "?";
  return `${path}${separator}workspace_id=${encodeURIComponent(options.workspaceId)}`;
}

export function requestOptions(options?: WorkspaceApiOptions): ApiRequestOptions | undefined {
  if (!options) return undefined;
  const { workspaceId: _workspaceId, ...rest } = options;
  return rest;
}
