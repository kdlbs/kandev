export type SandboxReportKind = "filesystem" | "network" | "command" | "http_endpoint";
export type SandboxReportDecision =
  | "allowed"
  | "denied"
  | "approval_requested"
  | "approval_granted"
  | "approval_denied"
  | "error";
export type SandboxReportEvent = {
  id: string;
  source_id: string;
  provider_record_id: string;
  sequence: number;
  kind: SandboxReportKind;
  decision: SandboxReportDecision;
  reason_code: string;
  occurred_at: string;
  exact_target: boolean;
  learning_blocker: "" | "incomplete_identity" | "not_denied" | "unsupported_policy";
  legacy_record_id?: string;
  operation: {
    filesystem?: { path: string; access: string };
    tcp?: { host: string; port: number; transport: string };
    command?: { id: string; caller: string };
    http_endpoint?: { route_id: string; origin: string; method: string; path: string };
  };
};
export type SandboxTypedReport = {
  schema_version: 2;
  artifact_set_digest: string;
  integrity: "unchecked" | "valid" | "invalid";
  finalization: "unknown" | "live" | "final" | "interrupted";
  coverage: {
    kind: SandboxReportKind;
    coverage: "supported" | "partial" | "unsupported" | "unavailable";
  }[];
  sources: { id: string; schema: string; digest: string; identity: string; bytes: number }[] | null;
  records: SandboxReportEvent[] | null;
};
