import { fetchJson, type ApiRequestOptions } from "../client";
import type { SSHTestRequest, SSHTestResult, SSHSession } from "@/lib/types/http-ssh";

export async function testSSHConnection(
  request: SSHTestRequest,
  options?: ApiRequestOptions,
): Promise<SSHTestResult> {
  return fetchJson<SSHTestResult>("/api/v1/ssh/test", {
    ...options,
    init: {
      method: "POST",
      headers: { "Content-Type": "application/json", ...(options?.init?.headers ?? {}) },
      body: JSON.stringify(request),
      ...(options?.init ?? {}),
    },
  });
}

export async function listSSHSessions(
  executorId: string,
  options?: ApiRequestOptions,
): Promise<SSHSession[]> {
  return fetchJson<SSHSession[]>(
    `/api/v1/ssh/executors/${encodeURIComponent(executorId)}/sessions`,
    options,
  );
}
