import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  SSHTestRequest,
  SSHTestResult,
  SSHSession,
  SSHAgentReadinessResponse,
  SSHProbeShellsResponse,
  SSHIdentitiesResponse,
  SSHReachabilityRecord,
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
      // Content-Type and break server-side parsing. Same ordering inside the
      // inner `headers` literal: spread first, JSON content-type last.
      ...(options?.init ?? {}),
      method: "POST",
      headers: { ...(options?.init?.headers ?? {}), "Content-Type": "application/json" },
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
  body?: { shell?: string },
  options?: ApiRequestOptions,
): Promise<SSHAgentReadinessResponse> {
  return fetchJson<SSHAgentReadinessResponse>(
    `/api/v1/ssh/executors/${encodeURIComponent(executorId)}/probe-agents`,
    {
      ...options,
      init: {
        ...(options?.init ?? {}),
        method: "POST",
        headers: { ...(options?.init?.headers ?? {}), "Content-Type": "application/json" },
        body: JSON.stringify(body ?? {}),
      },
    },
  );
}

export async function probeSSHShells(
  executorId: string,
  options?: ApiRequestOptions,
): Promise<SSHProbeShellsResponse> {
  return fetchJson<SSHProbeShellsResponse>(
    `/api/v1/ssh/executors/${encodeURIComponent(executorId)}/probe-shells`,
    {
      ...options,
      init: {
        ...(options?.init ?? {}),
        method: "POST",
        headers: { ...(options?.init?.headers ?? {}), "Content-Type": "application/json" },
      },
    },
  );
}

/**
 * Lists the private key files the backend host could use as a `file` identity
 * source. Admin-only: callers must treat a rejection as "no picker" rather
 * than as an error worth surfacing, because the free-text path still works.
 */
export async function listSSHIdentities(
  options?: ApiRequestOptions,
): Promise<SSHIdentitiesResponse> {
  return fetchJson<SSHIdentitiesResponse>("/api/v1/ssh/identities", options);
}

export async function getSSHReachability(
  options?: ApiRequestOptions,
): Promise<SSHReachabilityRecord[]> {
  return fetchJson<SSHReachabilityRecord[]>("/api/v1/ssh/reachability", options);
}

export async function getSSHExecutorReachability(
  executorId: string,
  options?: ApiRequestOptions,
): Promise<SSHReachabilityRecord> {
  return fetchJson<SSHReachabilityRecord>(
    `/api/v1/ssh/executors/${encodeURIComponent(executorId)}/reachability`,
    options,
  );
}

export async function probeSSHExecutorReachability(
  executorId: string,
  options?: ApiRequestOptions,
): Promise<SSHReachabilityRecord> {
  return fetchJson<SSHReachabilityRecord>(
    `/api/v1/ssh/executors/${encodeURIComponent(executorId)}/reachability/probe`,
    {
      ...options,
      init: {
        ...(options?.init ?? {}),
        method: "POST",
        headers: { ...(options?.init?.headers ?? {}), "Content-Type": "application/json" },
      },
    },
  );
}
