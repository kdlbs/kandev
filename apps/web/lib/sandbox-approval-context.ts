import type { AppState } from "./state/store";
import type { SandboxApproval } from "./api/domains/sandbox-approvals-api";

export function sandboxApprovalContext(state: AppState, sessionId: string, backend: string) {
  const { auth, connection } = state;
  const session = state.taskSessions.items[sessionId];
  const snapshot = session?.execution_snapshot;
  const bound = isSnapshotBound(session, sessionId);
  const canRead =
    state.features.sandboxPolicies &&
    auth.mode === "enabled" &&
    auth.authenticated &&
    Boolean(auth.user?.id) &&
    bound;
  return {
    key: JSON.stringify([
      backend,
      auth.mode,
      auth.authenticated,
      auth.user?.id,
      auth.user?.role,
      sessionId,
      session?.queue_incarnation_id,
      snapshot?.attempt_id,
      connection.epoch ?? 0,
    ]),
    enabled: Boolean(canRead && connection.status === "connected"),
    operator: auth.user?.role === "admin",
    snapshot: bound ? snapshot : null,
  };
}

export function isApprovalBound(
  approval: SandboxApproval,
  context: ReturnType<typeof sandboxApprovalContext>,
): boolean {
  const snapshot = context.snapshot;
  return Boolean(
    snapshot &&
    approval.session_id === snapshot.session_id &&
    approval.attempt_id === snapshot.attempt_id &&
    approval.policy_digest === snapshot.sandbox?.policy_digest &&
    Number.isSafeInteger(approval.version) &&
    approval.version > 0,
  );
}

export function isApprovalPending(approval: SandboxApproval, now: number): boolean {
  return (
    approval.state === "pending" && approval.can_decide && Date.parse(approval.expires_at) > now
  );
}

function isSnapshotBound(
  session: AppState["taskSessions"]["items"][string] | undefined,
  sessionId: string,
) {
  const snapshot = session?.execution_snapshot;
  return (
    snapshot?.schema_version === 1 &&
    snapshot.session_id === sessionId &&
    snapshot.task_id === session?.task_id &&
    snapshot.sandbox?.attempt_id === snapshot.attempt_id
  );
}
