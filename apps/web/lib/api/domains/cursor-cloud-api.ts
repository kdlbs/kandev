import { fetchJson, type ApiRequestOptions } from "../client";

export type CursorCloudModel = {
  id: string;
  displayName: string;
  description?: string;
  aliases?: string[];
};

export type CursorCloudRepository = {
  url: string;
  startingRef?: string;
};

export type CursorCloudConfigResponse = {
  connected: boolean;
  callback_route?: string;
  cursor_reachability?: string;
  models?: CursorCloudModel[];
  repositories?: CursorCloudRepository[];
};

export type CursorCloudSubmissionResolution = {
  operationId: string;
  state: string;
  candidates: Array<{ runId: string; status: string; createdAt: string }>;
};

export async function testCursorCloudConnection(
  secretId: string,
  callbackUrl: string,
  options?: ApiRequestOptions,
): Promise<CursorCloudConfigResponse> {
  return postConfig("test", secretId, callbackUrl, options);
}

export async function loadCursorCloudCatalog(
  secretId: string,
  callbackUrl: string,
  options?: ApiRequestOptions,
): Promise<CursorCloudConfigResponse> {
  return postConfig("catalog", secretId, callbackUrl, options);
}

async function postConfig(
  action: "test" | "catalog",
  secretId: string,
  callbackUrl: string,
  options?: ApiRequestOptions,
): Promise<CursorCloudConfigResponse> {
  return fetchJson<CursorCloudConfigResponse>(`/api/v1/cursor-cloud/${action}`, {
    ...options,
    init: {
      method: "POST",
      body: JSON.stringify({ secret_id: secretId, callback_url: callbackUrl }),
      ...(options?.init ?? {}),
    },
  });
}

function submissionPath(taskId: string, sessionId: string): string {
  return `/api/v1/tasks/${encodeURIComponent(taskId)}/sessions/${encodeURIComponent(sessionId)}/cursor-cloud/submission`;
}

export async function getCursorCloudSubmissionResolution(
  taskId: string,
  sessionId: string,
  options?: ApiRequestOptions,
): Promise<CursorCloudSubmissionResolution> {
  return fetchJson<CursorCloudSubmissionResolution>(submissionPath(taskId, sessionId), options);
}

export async function bindCursorCloudSubmissionCandidate(
  taskId: string,
  sessionId: string,
  runId: string,
  options?: ApiRequestOptions,
): Promise<{ operation_id: string; state: string }> {
  return fetchJson(`${submissionPath(taskId, sessionId)}/bind`, {
    ...options,
    init: { method: "POST", body: JSON.stringify({ run_id: runId }), ...(options?.init ?? {}) },
  });
}

export async function retryCursorCloudSubmission(
  taskId: string,
  sessionId: string,
  resolutionId: string,
  acknowledgeDuplicateWork: boolean,
  options?: ApiRequestOptions,
): Promise<{ operation_id: string; state: string }> {
  return fetchJson(`${submissionPath(taskId, sessionId)}/retry`, {
    ...options,
    init: {
      method: "POST",
      body: JSON.stringify({
        resolution_id: resolutionId,
        acknowledge_duplicate_work: acknowledgeDuplicateWork,
      }),
      ...(options?.init ?? {}),
    },
  });
}
