import { fetchJson, type ApiRequestOptions } from "../client";

export type CanvasScopeKind = "task" | "workspace";
export type CanvasStatus = "pending" | "active" | "disabled" | "archived" | "error" | "removed";
export type CanvasReleaseStatus = "valid" | "pending_permission" | "invalid" | "unavailable";

export type CanvasRelease = {
  id: string;
  package_digest?: string;
  validation_status: CanvasReleaseStatus | string;
  validation_error?: string;
  permissions?: CanvasPermissionReview;
  missing_permissions?: string[];
  permission_digest?: string;
  source_actor_kind?: string;
  source_user_id?: string;
  source_task_id?: string;
  source_session_id?: string;
  protocol_version?: number;
  created_at?: string;
};

export type Canvas = {
  id: string;
  plugin_instance_id: string;
  plugin_id: string;
  workspace_id: string;
  task_id?: string;
  origin_task_id?: string;
  created_by_session_id?: string;
  scope_kind: CanvasScopeKind | string;
  title: string;
  status: CanvasStatus | string;
  active_release_id?: string;
  active_release_status?: CanvasReleaseStatus | string;
  active_release_error?: string;
  grant_generation?: number;
  effective_grants?: CanvasGrantProjection[];
  active_release?: CanvasRelease;
  pending_release?: CanvasRelease;
  created_at?: string;
  updated_at?: string;
};

export type CanvasListResponse = {
  canvases: Canvas[];
  total?: number;
};

export type CanvasRuntimeResponse = {
  runtime_url?: string | null;
  release_id?: string;
  active_release_id?: string;
  web_app_key?: string;
  placement?: string;
  expires_in_seconds?: number;
  canvas?: Canvas;
  binding?: {
    scope_kind?: CanvasScopeKind | string;
    grant_generation?: number;
  };
};

export type CanvasPermissionReview = {
  reads?: string[];
  writes?: string[];
  events?: string[];
  shared_state?: boolean;
  external_origins?: string[];
};

export type CanvasGrantProjection = {
  permission_kind: string;
  resource?: string;
  network_origin?: string;
  scope_ceiling: string;
};

export type CanvasPromotionPreview = {
  canvas_id: string;
  title?: string;
  origin_task_id?: string;
  source_actor_kind?: string;
  source_user_id?: string;
  source_task_id?: string;
  source_session_id?: string;
  active_release?: CanvasRelease;
  permissions?: CanvasPermissionReview;
  active_release_id?: string;
  permission_digest?: string;
  grant_generation?: number;
  current_scope?: CanvasScopeKind | string;
  target_scope?: CanvasScopeKind | string;
  placement?: string;
};

export type CanvasReleaseListResponse = {
  releases: CanvasRelease[];
};

export type CanvasEditResponse = {
  task_id?: string;
  session_id?: string;
  canvas_id: string;
};

export type CanvasRequestOptions = ApiRequestOptions & {
  includeArchived?: boolean;
};

export function canvasHref(canvasId: string): string {
  return `/canvases/${encodeURIComponent(canvasId)}`;
}

export function workspaceCanvasSettingsHref(workspaceId: string): string {
  return `/settings/workspaces/${encodeURIComponent(workspaceId)}/canvases`;
}

function canvasPath(canvasId: string, suffix = ""): string {
  return `/api/v1/canvases/${encodeURIComponent(canvasId)}${suffix}`;
}

function withArchived(path: string, includeArchived?: boolean): string {
  return includeArchived ? `${path}${path.includes("?") ? "&" : "?"}include_archived=true` : path;
}

export function listWorkspaceCanvases(
  workspaceId: string,
  options?: CanvasRequestOptions,
): Promise<CanvasListResponse> {
  const { includeArchived, ...requestOptions } = options ?? {};
  return fetchJson<CanvasListResponse>(
    withArchived(`/api/v1/workspaces/${encodeURIComponent(workspaceId)}/canvases`, includeArchived),
    requestOptions,
  );
}

export function listTaskCanvases(
  taskId: string,
  options?: CanvasRequestOptions & { workspaceId?: string },
): Promise<CanvasListResponse> {
  const { includeArchived, workspaceId, ...requestOptions } = options ?? {};
  const query = workspaceId ? `?workspace_id=${encodeURIComponent(workspaceId)}` : "";
  return fetchJson<CanvasListResponse>(
    withArchived(`/api/v1/tasks/${encodeURIComponent(taskId)}/canvases${query}`, includeArchived),
    requestOptions,
  );
}

export function getCanvas(canvasId: string, options?: ApiRequestOptions): Promise<Canvas> {
  return fetchJson<Canvas>(canvasPath(canvasId), options);
}

export function getCanvasRuntime(
  canvasId: string,
  options?: ApiRequestOptions,
): Promise<CanvasRuntimeResponse> {
  return fetchJson<CanvasRuntimeResponse>(canvasPath(canvasId, "/runtime"), options);
}

export function requestCanvasPromotion(
  canvasId: string,
  options?: ApiRequestOptions,
): Promise<CanvasPromotionPreview> {
  return fetchJson<CanvasPromotionPreview>(canvasPath(canvasId, "/promotion-preview"), options);
}

export function confirmCanvasPromotion(
  canvasId: string,
  review: {
    expected_release_id: string;
    expected_permission_digest: string;
    expected_grant_generation: number;
  },
  options?: ApiRequestOptions,
): Promise<Canvas> {
  return mutate<Canvas>(canvasPath(canvasId, "/promotion"), "POST", review, options);
}

export function listCanvasReleases(
  canvasId: string,
  options?: ApiRequestOptions,
): Promise<CanvasReleaseListResponse> {
  return fetchJson<CanvasReleaseListResponse>(canvasPath(canvasId, "/releases"), options);
}

export function approveCanvasRelease(
  canvasId: string,
  releaseId: string,
  options?: ApiRequestOptions,
): Promise<Canvas> {
  return mutate<Canvas>(
    canvasPath(canvasId, `/releases/${encodeURIComponent(releaseId)}/approve`),
    "POST",
    undefined,
    options,
  );
}

export function rejectCanvasRelease(
  canvasId: string,
  releaseId: string,
  options?: ApiRequestOptions,
): Promise<Canvas> {
  return mutate<Canvas>(
    canvasPath(canvasId, `/releases/${encodeURIComponent(releaseId)}/reject`),
    "POST",
    undefined,
    options,
  );
}

export function rollbackCanvas(
  canvasId: string,
  releaseId?: string,
  options?: ApiRequestOptions,
): Promise<Canvas> {
  return mutate<Canvas>(
    canvasPath(canvasId, "/rollback"),
    "POST",
    {
      ...(releaseId ? { release_id: releaseId } : {}),
    },
    options,
  );
}

export function startCanvasEdit(
  canvasId: string,
  options?: ApiRequestOptions,
): Promise<CanvasEditResponse> {
  return mutate<CanvasEditResponse>(canvasPath(canvasId, "/edit"), "POST", undefined, options);
}

export function archiveCanvas(canvasId: string, options?: ApiRequestOptions): Promise<Canvas> {
  return mutate<Canvas>(canvasPath(canvasId, "/archive"), "POST", undefined, options);
}

export function restoreCanvas(canvasId: string, options?: ApiRequestOptions): Promise<Canvas> {
  return mutate<Canvas>(canvasPath(canvasId, "/restore"), "POST", undefined, options);
}

export function removeCanvas(canvasId: string, options?: ApiRequestOptions): Promise<void> {
  return mutate<void>(canvasPath(canvasId), "DELETE", undefined, options);
}

function mutate<T>(
  path: string,
  method: "POST" | "DELETE",
  body: unknown,
  options?: ApiRequestOptions,
): Promise<T> {
  return fetchJson<T>(path, {
    ...options,
    init: {
      ...(options?.init ?? {}),
      method,
      ...(body === undefined ? {} : { body: JSON.stringify(body) }),
    },
  });
}
