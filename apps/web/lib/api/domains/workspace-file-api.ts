import { getBackendConfig } from "@/lib/config";
import { readInterimSettingsInterlockToken } from "@/src/boot-payload";
import { ApiError } from "../client";

/** What to do when an upload's destination already exists. */
export type UploadResolution = "replace" | "keep_both";

export type UploadConflict = {
  path: string;
  is_dir: boolean;
};

export type UploadedWorkspaceFile = {
  path: string;
  size_bytes: number;
  resolution_applied?: string;
};

export type UploadWorkspaceFileParams = {
  sessionId: string;
  dir: string;
  relativePath: string;
  file: File;
  repo?: string;
  resolution?: UploadResolution;
};

function interlockHeaders(): Record<string, string> | undefined {
  const token = readInterimSettingsInterlockToken();
  return token ? { ["X-Kandev-Interim-Settings-Interlock"]: token } : undefined;
}

async function readErrorMessage(response: Response, fallback: string): Promise<[string, unknown]> {
  let body: unknown = null;
  try {
    body = await response.json();
  } catch {
    // Keep the status as the useful error when the server returned no JSON.
  }
  const message =
    body && typeof body === "object" && "error" in body && typeof body.error === "string"
      ? body.error
      : fallback;
  return [message, body];
}

/**
 * Report which of the destination-relative paths already exist, before any
 * content is uploaded. This is what lets the caller resolve every conflict up
 * front rather than discovering them one failed upload at a time.
 */
export async function preflightWorkspaceUpload(params: {
  sessionId: string;
  dir: string;
  paths: string[];
  repo?: string;
}): Promise<UploadConflict[]> {
  const response = await fetch(
    `${getBackendConfig().apiBaseUrl}/api/v1/task-sessions/${params.sessionId}/workspace/files/preflight`,
    {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json", ...(interlockHeaders() ?? {}) },
      body: JSON.stringify({ dir: params.dir, repo: params.repo ?? "", paths: params.paths }),
    },
  );

  if (!response.ok) {
    // i18n-exempt: server-provided error text, or an HTTP status diagnostic when
    // the server sent none. Callers render translated copy for the generic case.
    const [message, body] = await readErrorMessage(
      response,
      `Upload preflight failed (${response.status})`,
    );
    throw new ApiError(message, response.status, body);
  }

  const payload = (await response.json()) as { conflicts?: UploadConflict[] };
  return payload.conflicts ?? [];
}

/**
 * Upload one file. One request carries one file so a rejection of one file in a
 * selection does not fail the rest, and so per-file status stays observable.
 */
export async function uploadWorkspaceFile(
  params: UploadWorkspaceFileParams,
): Promise<UploadedWorkspaceFile> {
  const form = new FormData();
  form.append("dir", params.dir);
  form.append("repo", params.repo ?? "");
  form.append("relative_path", params.relativePath);
  form.append("size_bytes", String(params.file.size));
  if (params.resolution) form.append("resolution", params.resolution);
  form.append("file", params.file, params.file.name);

  const response = await fetch(
    `${getBackendConfig().apiBaseUrl}/api/v1/task-sessions/${params.sessionId}/workspace/files`,
    { method: "POST", credentials: "include", body: form, headers: interlockHeaders() },
  );

  if (!response.ok) {
    // i18n-exempt: server-provided error text, or an HTTP status diagnostic when
    // the server sent none. Callers render translated copy for the generic case.
    const [message, body] = await readErrorMessage(response, `Upload failed (${response.status})`);
    throw new ApiError(message, response.status, body);
  }

  return (await response.json()) as UploadedWorkspaceFile;
}
