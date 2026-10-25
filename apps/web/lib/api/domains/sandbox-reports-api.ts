import { fetchJson, type ApiRequestOptions } from "../client";

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
