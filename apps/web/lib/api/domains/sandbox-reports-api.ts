import { fetchJson, type ApiRequestOptions } from "../client";
import type { SandboxTypedReport } from "./sandbox-report-types";

export type SandboxReportSource = {
  id: string;
  schema: string;
  digest: string;
  cursor: string;
  next_cursor: string;
  integrity: "complete" | "partial" | "unverified";
  complete: boolean;
};
export type SandboxDenial = {
  id: string;
  source_id: string;
  provider_record_id: string;
  kind: "filesystem" | "network" | "command";
  operation: "read" | "write" | "connect" | "execute";
  target: string;
  occurred_at: string;
};
export type SandboxReport = {
  schema_version: number;
  attempt_id: string;
  session_id: string;
  revision_id: string;
  policy_digest: string;
  ceiling_digest: string;
  provider: {
    protocol_major?: number;
    plugin_id: string;
    installation_id: string;
    key: string;
    version: string;
    package_digest: string;
    runtime_version: string;
    runtime_digest: string;
  };
  version: number;
  digest: string;
  captured_at: string;
  data: {
    final_audit?: { binding_digest: string; artifact_set_digest: string; captured_at: string };
    v2?: SandboxTypedReport;
    last_successful_read_at?: string;
    refresh_error?: "unavailable" | "unsupported" | "invalid_evidence" | "limit_exceeded";
    status: "available" | "unavailable" | "unsupported";
    integrity: "complete" | "partial" | "unverified";
    complete: boolean;
    finalized: boolean;
    supported_classes: SandboxDenial["kind"][] | null;
    sources: SandboxReportSource[] | null;
    records: SandboxDenial[] | null;
  };
};

export function fetchSandboxReport(
  sessionId: string,
  attemptId: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<SandboxReport>(
    `/api/v1/sandbox/sessions/${encodeURIComponent(sessionId)}/attempts/${encodeURIComponent(attemptId)}/report`,
    { ...options, cache: "no-store" },
  );
}
