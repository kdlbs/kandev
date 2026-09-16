import { fetchBlob, fetchJson, type ApiRequestOptions } from "../client";
import type { Canvas, CanvasPermissionReview } from "./canvas-api";

export type DistributionMetadata = {
  package_id?: string;
  version?: string;
  display_name?: string;
  description?: string;
  author?: string;
  license?: string;
  source_mode?: "static" | "project";
  min_kandev_version?: string;
  repo_url?: string;
};

export type DistributionFile = { path: string; bytes: number };

export type ExportReview = {
  preparation_id: string;
  canvas_id: string;
  workspace_id: string;
  release_id: string;
  metadata: DistributionMetadata;
  sha256: string;
  files: DistributionFile[];
  bundle_bytes: number;
  source_bytes: number;
  expires_at: string;
  bundle_download: string;
  source_download: string;
};

export type InstallReview = {
  preparation_id: string;
  workspace_id: string;
  metadata: Required<
    Pick<
      DistributionMetadata,
      | "package_id"
      | "version"
      | "display_name"
      | "description"
      | "author"
      | "license"
      | "source_mode"
      | "min_kandev_version"
    >
  > & { repo_url?: string };
  sha256: string;
  archive_sha256?: string;
  permissions: CanvasPermissionReview;
  origin_kind: "upload" | "url" | "registry";
  source_id?: string;
  repository_url?: string;
  expires_at: string;
};

export type InstallRequest = {
  workspace_id: string;
  origin_kind?: "upload" | "url" | "registry";
  source_id?: string;
  repository_url?: string;
  package_id?: string;
  expected_version?: string;
  expected_sha256?: string;
  bundle_url?: string;
};

export type InstallResult = { canvas: Canvas; receipt: Record<string, unknown> };

const EXPORTS = "/api/v1/canvases";
const INSTALLS = "/api/v1/canvases/install-preparations";

export function prepareCanvasExport(
  canvasId: string,
  request: { workspace_id: string; expected_release_id: string; metadata?: DistributionMetadata },
  options?: ApiRequestOptions,
): Promise<ExportReview> {
  return fetchJson<ExportReview>(`${EXPORTS}/${encodeURIComponent(canvasId)}/exports`, {
    ...options,
    init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify(request) },
  });
}

export function getCanvasExport(
  preparationId: string,
  options?: ApiRequestOptions,
): Promise<ExportReview> {
  return fetchJson<ExportReview>(
    `${EXPORTS}/exports/${encodeURIComponent(preparationId)}`,
    options,
  );
}

export function downloadCanvasExport(
  preparationId: string,
  kind: "bundle" | "source",
  options?: ApiRequestOptions,
): Promise<Blob> {
  return fetchBlob(`${EXPORTS}/exports/${encodeURIComponent(preparationId)}/${kind}`, options);
}

export function cancelCanvasExport(
  preparationId: string,
  options?: ApiRequestOptions,
): Promise<void> {
  return fetchJson<void>(`${EXPORTS}/exports/${encodeURIComponent(preparationId)}`, {
    ...options,
    init: { ...(options?.init ?? {}), method: "DELETE" },
  });
}

export function prepareCanvasInstall(
  request: InstallRequest,
  options?: ApiRequestOptions,
): Promise<InstallReview> {
  return fetchJson<InstallReview>(INSTALLS, {
    ...options,
    init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify(request) },
  });
}

export function uploadCanvasInstall(
  file: File,
  fields: Omit<InstallRequest, "bundle_url">,
  options?: ApiRequestOptions,
): Promise<InstallReview> {
  const body = new FormData();
  body.set("package", file);
  Object.entries(fields).forEach(([key, value]) => {
    if (value) body.set(key, value);
  });
  return fetchJson<InstallReview>(INSTALLS, {
    ...options,
    init: { ...(options?.init ?? {}), method: "POST", body },
  });
}

export function getCanvasInstall(
  preparationId: string,
  options?: ApiRequestOptions,
): Promise<InstallReview> {
  return fetchJson<InstallReview>(`${INSTALLS}/${encodeURIComponent(preparationId)}`, options);
}

export function confirmCanvasInstall(
  preparationId: string,
  expectedDigest: string,
  options?: ApiRequestOptions,
): Promise<InstallResult> {
  return fetchJson<InstallResult>(`${INSTALLS}/${encodeURIComponent(preparationId)}/confirm`, {
    ...options,
    init: {
      ...(options?.init ?? {}),
      method: "POST",
      body: JSON.stringify({ approved: true, expected_digest: expectedDigest }),
    },
  });
}

export function cancelCanvasInstall(
  preparationId: string,
  options?: ApiRequestOptions,
): Promise<void> {
  return fetchJson<void>(`${INSTALLS}/${encodeURIComponent(preparationId)}`, {
    ...options,
    init: { ...(options?.init ?? {}), method: "DELETE" },
  });
}
