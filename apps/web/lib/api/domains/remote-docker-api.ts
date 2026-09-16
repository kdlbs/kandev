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
