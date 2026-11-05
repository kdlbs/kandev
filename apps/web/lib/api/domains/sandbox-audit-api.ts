import { fetchJson, type ApiRequestOptions } from "../client";
import type { SandboxReport } from "./sandbox-reports-api";

export type SandboxAuditTrust = {
  schema_version: number;
  id: string;
  version: number;
  org_id: string;
  workspace_id: string;
  installation_id: string;
  provider_key: string;
  fingerprint: string;
  public_key_pem: string;
  revoked: boolean;
  operator_id: string;
  source: "operator" | "unconfigured";
  created_at: string;
  digest: string;
};

export type SandboxAuditTrustRequest = {
  expected_version: number;
  public_key_pem?: string;
  revoke?: boolean;
};

export type SandboxAuditRequest = {
  request_id: string;
  expected_report_digest: string;
  expected_artifact_digest: string;
  expected_trust_version: number;
};

export type SandboxAuditResult = {
  artifact_set_digest: string;
  verifier_version: string;
  session_id: string;
  event_root: string;
  event_count: number;
  chain_state: "unchecked" | "valid" | "invalid";
  finalization: "final" | "interrupted";
  signature_state: "unchecked" | "unsigned" | "untrusted" | "verified" | "invalid" | "unsupported";
  expected_key_fingerprint: string;
  signer_key_fingerprint: string;
  ledger_state: "unavailable" | "valid" | "invalid";
  trust_revision: string;
};

export type SandboxAuditJob = {
  schema_version: number;
  id: string;
  version: number;
  state: "pending" | "running" | "completed" | "failed" | "interrupted";
  attempt_id: string;
  session_id: string;
  provider_session_id: string;
  report_digest: string;
  artifact_set_digest: string;
  binding_digest: string;
  provider: SandboxReport["provider"];
  trust: {
    revision_id: string;
    version: number;
    fingerprint: string;
    revoked: boolean;
    source: string;
    operator_id: string;
    configured_at: string;
  };
  requested_by: string;
  requested_at: string;
  started_at?: string;
  completed_at?: string;
  failure_code?:
    | "unavailable"
    | "unsupported"
    | "invalid_evidence"
    | "timeout"
    | "limit_exceeded"
    | "cancelled"
    | "restarted";
  result?: SandboxAuditResult;
};

export type SandboxAuditView = { verification: SandboxAuditJob; signer_trusted_now: boolean };
export type SandboxAuditPage = { verifications: SandboxAuditView[]; next_cursor?: string };

function auditPath(sessionId: string, attemptId: string) {
  return `/api/v1/sandbox/sessions/${encodeURIComponent(sessionId)}/attempts/${encodeURIComponent(attemptId)}/audit`;
}

export function fetchSandboxAuditTrust(
  session: string,
  attempt: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<SandboxAuditTrust>(`${auditPath(session, attempt)}/trust`, {
    ...options,
    cache: "no-store",
  });
}

export function saveSandboxAuditTrust(
  session: string,
  attempt: string,
  request: SandboxAuditTrustRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<SandboxAuditTrust>(`${auditPath(session, attempt)}/trust`, {
    ...options,
    cache: "no-store",
    init: { ...options?.init, method: "PUT", body: JSON.stringify(request) },
  });
}

export function fetchSandboxAuditHistory(
  session: string,
  attempt: string,
  cursor = "",
  options?: ApiRequestOptions,
) {
  const query = cursor ? `?${new URLSearchParams({ cursor })}` : "";
  return fetchJson<SandboxAuditPage>(`${auditPath(session, attempt)}/verifications${query}`, {
    ...options,
    cache: "no-store",
  });
}

export function fetchSandboxAuditVerification(
  session: string,
  attempt: string,
  id: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<SandboxAuditView>(
    `${auditPath(session, attempt)}/verifications/${encodeURIComponent(id)}`,
    { ...options, cache: "no-store" },
  );
}

export function verifySandboxAudit(
  session: string,
  attempt: string,
  request: SandboxAuditRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<SandboxAuditView>(`${auditPath(session, attempt)}/verifications`, {
    ...options,
    cache: "no-store",
    init: { ...options?.init, method: "POST", body: JSON.stringify(request) },
  });
}
