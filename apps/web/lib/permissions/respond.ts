import { t } from "@/lib/i18n";
import type { PermissionOptionKind } from "@/lib/types/permission";

export type PermissionOption = {
  option_id: string;
  name: string;
  kind: PermissionOptionKind;
  metadata?: Record<string, unknown>;
};

export type PermissionActionChoice = {
  option_id: string;
  label: string;
  kind: PermissionOptionKind;
};

export type PermissionDecision = {
  optionId: string;
  cancelled: boolean;
  rejected: boolean;
};

export type PermissionRequestIdentity = {
  task_id: string;
  session_id: string;
  request_id: string;
  pending_id: string;
};

const CANCEL_DECISION: PermissionDecision = { optionId: "", cancelled: true, rejected: false };

export function isStalePermissionResponse(error: unknown): boolean {
  if (!(error instanceof Error)) return false;
  return ["permission_not_found", "permission_stale", "permission_already_resolved"].some((code) =>
    error.message.includes(code),
  );
}

function isRejectKind(option: PermissionOption): boolean {
  return option.kind === "reject_once" || option.kind === "reject_always";
}

// rejected=true tells the backend to persist "rejected" status without
// treating this as a dialog cancellation (cancelled=true would race with the
// permission-cancelled path that marks the request expired).
export function denyDecision(options: PermissionOption[]): PermissionDecision {
  const reject = options.find(isRejectKind);
  if (!reject) return CANCEL_DECISION;
  return { optionId: reject.option_id, cancelled: false, rejected: true };
}

// Approve is the one-shot allow: allow_once first, allow_always only when the
// agent offers nothing else, so "Always allow" stays a distinct button.
export function approveDecision(options: PermissionOption[]): PermissionDecision | null {
  const allow =
    options.find((opt) => opt.kind === "allow_once") ??
    options.find((opt) => opt.kind === "allow_always");
  if (!allow) return null;
  return { optionId: allow.option_id, cancelled: false, rejected: false };
}

export function allowAlwaysDecision(options: PermissionOption[]): PermissionDecision | null {
  const always = options.find((opt) => opt.kind === "allow_always");
  if (!always) return null;
  return { optionId: always.option_id, cancelled: false, rejected: false };
}

export function codexDecisionLabel(option: PermissionOption): string {
  switch (option.metadata?.codex_decision) {
    case "accept":
      return t("task:approve");
    case "accept_for_session":
      return t("task:alwaysAllow");
    case "decline":
      return t("task:deny");
    case "cancel":
      return t("common:cancel");
    case "accept_with_execpolicy_amendment":
      return t("task:approveWithCommandPolicy");
    case "apply_network_policy_allow":
      return t("task:allowNetworkAccess");
    case "apply_network_policy_deny":
      return t("task:blockNetworkAccess");
    default:
      return option.name;
  }
}

export function offeredChoices(options: PermissionOption[]): PermissionActionChoice[] {
  return options
    .filter((option) => option.metadata?.codex_app_server === true)
    .map((option) => ({
      option_id: option.option_id,
      kind: option.kind,
      label: codexDecisionLabel(option),
    }));
}

export function offeredChoiceDecision(
  options: PermissionOption[],
  optionId: string,
): PermissionDecision | null {
  const option = options.find(
    (candidate) =>
      candidate.option_id === optionId && candidate.metadata?.codex_app_server === true,
  );
  if (!option) return null;
  if (option.metadata?.codex_decision === "cancel") return CANCEL_DECISION;
  return { optionId: option.option_id, cancelled: false, rejected: isRejectKind(option) };
}

export function buildPermissionRespondRequest(
  identity: PermissionRequestIdentity,
  decision: PermissionDecision,
) {
  return {
    task_id: identity.task_id,
    session_id: identity.session_id,
    request_id: identity.request_id,
    pending_id: identity.pending_id,
    option_id: decision.cancelled ? undefined : decision.optionId,
    cancelled: decision.cancelled,
    rejected: decision.rejected,
  };
}
