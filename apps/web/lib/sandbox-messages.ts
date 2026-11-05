import { ApiError } from "./api/client";

export class SandboxUpdateError extends Error {
  readonly messageKey: ReturnType<typeof sandboxErrorMessageKey>;
  constructor(cause: unknown) {
    // i18n-exempt: internal error identity. The UI translates messageKey.
    super("sandbox_update_failed");
    this.messageKey = sandboxErrorMessageKey(cause);
  }
}

export function sandboxErrorMessageKey(reason: unknown) {
  const body = reason instanceof ApiError ? reason.body : null;
  return sandboxMessageKey(
    body && typeof body === "object" && "error" in body ? body.error : undefined,
  );
}

export function sandboxMessageKey(code: unknown) {
  switch (code) {
    case "sandbox_policy_ineligible":
      return "agents:sandboxSelectionBlocked";
    case "sandbox_revision_conflict":
      return "agents:sandboxRevisionConflict";
    case "sandbox_runtime_unavailable":
      return "agents:sandboxRuntimeUnavailable";
    case "sandbox_policy_not_selected":
      return "agents:sandboxPolicyNotSelected";
    case "sandbox_operator_required":
      return "agents:sandboxOperatorRequired";
    default:
      return "agents:sandboxLoadFailed";
  }
}
