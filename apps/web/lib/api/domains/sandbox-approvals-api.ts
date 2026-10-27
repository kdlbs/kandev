import { fetchJson, type ApiRequestOptions } from "../client";

export type SandboxApproval = {
  request_id: string;
  attempt_id: string;
  session_id: string;
  version: number;
  state: "pending" | "approved" | "denied" | "expired" | "responding" | "sent" | "undelivered";
  access: "read" | "write";
  target: string;
  policy_digest: string;
  expires_at: string;
  can_decide: boolean;
};

export type SandboxDecision = "allow_once" | "deny";

export function fetchSandboxApprovals(sessionId: string, options?: ApiRequestOptions) {
  return fetchJson<SandboxApproval[]>(
    `/api/v1/sandbox/sessions/${encodeURIComponent(sessionId)}/approvals`,
    { ...options, cache: "no-store" },
  );
}

export function decideSandboxApproval(approval: SandboxApproval, decision: SandboxDecision) {
  return fetchJson<SandboxApproval>(
    `/api/v1/sandbox/attempts/${encodeURIComponent(approval.attempt_id)}/approvals/${encodeURIComponent(approval.request_id)}/decision`,
    {
      init: {
        method: "POST",
        body: JSON.stringify({ expected_version: approval.version, decision }),
      },
    },
  );
}
