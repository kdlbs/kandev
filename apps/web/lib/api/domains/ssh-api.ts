import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  SSHTestRequest,
  SSHTestResult,
  SSHSession,
  SSHAgentReadinessResponse,
} from "@/lib/types/http-ssh";

export async function testSSHConnection(
  request: SSHTestRequest,
  options?: ApiRequestOptions,
): Promise<SSHTestResult> {
  return fetchJson<SSHTestResult>("/api/v1/ssh/test", {
    ...options,
    init: {
      // Caller overrides come first so an extension can add e.g. an extra
      // header or signal, then the required POST + JSON body + Content-Type
      // win — otherwise a caller passing `init.headers` would clobber the
      // Content-Type and break server-side parsing.
      ...(options?.init ?? {}),
      method: "POST",
      headers: { "Content-Type": "application/json", ...(options?.init?.headers ?? {}) },
      body: JSON.stringify(request),
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

export async function probeSSHAgents(
  executorId: string,
  options?: ApiRequestOptions,
): Promise<SSHAgentReadinessResponse> {
  return fetchJson<SSHAgentReadinessResponse>(
    `/api/v1/ssh/executors/${encodeURIComponent(executorId)}/probe-agents`,
    {
      ...options,
      init: {
        ...(options?.init ?? {}),
        method: "POST",
        headers: { "Content-Type": "application/json", ...(options?.init?.headers ?? {}) },
      },
    },
  );
}
