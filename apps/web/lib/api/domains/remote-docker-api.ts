import { fetchJson, type ApiRequestOptions } from "../client";
import type { SSHTestRequest, SSHTestResult } from "@/lib/types/http-ssh";

/**
 * Runs the remote Docker executor's connection test.
 *
 * The request is the same SSH target the SSH executor tests, because the
 * daemon is reached over that connection. The response adds the daemon and
 * API-version steps, so the shared connection card renders both without
 * knowing which executor it is serving.
 *
 * `fetchJson` sets the JSON content type and merges caller headers, so this
 * only supplies the method and body.
 */
export async function testRemoteDockerConnection(
  request: SSHTestRequest,
  options?: ApiRequestOptions,
): Promise<SSHTestResult> {
  return fetchJson<SSHTestResult>("/api/v1/remote-docker/test", {
    ...options,
    init: {
      ...(options?.init ?? {}),
      method: "POST",
      body: JSON.stringify(request),
    },
  });
}

/**
 * Builds an image on a remote Docker executor's own daemon.
 *
 * The executor ID is required because the daemon is a property of that
 * executor's connection. Building on the install-wide daemon would place the
 * image where the task container will never run.
 */
export function buildRemoteDockerImage(
  executorId: string,
  payload: { dockerfile: string; tag: string; build_args?: Record<string, string | null> },
): Promise<Response> {
  return fetch(`/api/v1/remote-docker/executors/${encodeURIComponent(executorId)}/build`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "include",
    body: JSON.stringify(payload),
  });
}
