import { fetchJson } from "../client";
import type { SandboxGrants, SandboxPolicy, SandboxRevision } from "@/lib/types/sandbox";
import type { SandboxDenial, SandboxReport } from "./sandbox-reports-api";

export type SandboxLearningRequest = {
  report_digest: string;
  record_ids: string[];
  name: string;
  scope: "workspace" | "installation";
};
export type SandboxLearningProposal = {
  id: string;
  schema_version: number;
  attempt_id: string;
  session_id: string;
  report_digest: string;
  report_version: number;
  report_sources: NonNullable<SandboxReport["data"]["sources"]>;
  report_integrity: SandboxReport["data"]["integrity"];
  report_complete: boolean;
  base_revision_id: string;
  base_digest: string;
  ceiling_revision_id: string;
  ceiling_digest: string;
  provider: SandboxReport["provider"];
  executor_id: string;
  binding_version: number;
  selected_records: SandboxDenial[];
  added_grants: SandboxGrants;
  policy: SandboxPolicy;
  operator_id: string;
  created_at: string;
  digest: string;
  published_revision_id?: string;
};

export function proposeSandboxLearning(
  sessionId: string,
  attemptId: string,
  request: SandboxLearningRequest,
) {
  return fetchJson<SandboxLearningProposal>(
    `/api/v1/sandbox/sessions/${encodeURIComponent(sessionId)}/attempts/${encodeURIComponent(attemptId)}/learning`,
    { init: { method: "POST", body: JSON.stringify(request) } },
  );
}

export function publishSandboxLearning(proposal: SandboxLearningProposal) {
  return fetchJson<SandboxRevision>(
    `/api/v1/sandbox/learning/${encodeURIComponent(proposal.id)}/publish`,
    {
      init: { method: "POST", body: JSON.stringify({ expected_digest: proposal.digest }) },
    },
  );
}
