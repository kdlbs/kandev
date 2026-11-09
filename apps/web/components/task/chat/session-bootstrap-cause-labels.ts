import type { TFunction } from "i18next";

export function causeLabel(code: string | undefined, translate: TFunction): string {
  if (code === "process_exited") {
    return translate("task:sessionBootstrapProcessExitedBody");
  }
  return causeLabelSwitch(code, translate);
}

function causeLabelSwitch(code: string | undefined, translate: TFunction): string {
  switch (code) {
    case "authentication_required":
      return translate("task:sessionBootstrapCauseAuthenticationRequired");
    case "permission_denied":
      return translate("task:sessionBootstrapCausePermissionDenied");
    case "destination_invalid":
      return translate("task:sessionBootstrapCauseDestinationInvalid");
    case "source_branch_missing":
      return translate("task:sessionBootstrapCauseSourceBranchMissing");
    case "runtime_cleanup_failed":
      return translate("task:sessionRuntimeCleanupFailedBody");
    case "transport_unavailable":
      return translate("task:sessionBootstrapCauseTransportUnavailable");
    case "timeout":
      return translate("task:sessionBootstrapCauseTimeout");
    case "model_unavailable":
      return translate("task:sessionBootstrapCauseModelUnavailable");
    case "model_selection_failed":
      return translate("task:sessionBootstrapCauseModelSelectionFailed");
    case "permission_mode_failed":
      return translate("task:sessionBootstrapCausePermissionModeFailed");
    case "permission_mode_unconfirmed":
      return translate("task:sessionBootstrapCausePermissionModeUnconfirmed");
    case "permission_mode_mismatch":
      return translate("task:sessionBootstrapCausePermissionModeMismatch");
    case "delivery_storage":
      return translate("task:sessionBootstrapCauseDeliveryStorage");
    case "plugin_preparation_failed":
      return translate("task:sessionBootstrapPluginFailureTitle");
    default:
      return translate("task:sessionBootstrapCauseUnknown");
  }
}

export function operationLabel(operation: string | undefined, translate: TFunction): string {
  if (operation === "start") return translate("task:sessionRecoveryStartAttempt");
  if (operation === "restore_workspace") {
    return translate("task:sessionRecoveryRestoreAttempt");
  }
  if (operation === "delivery") return translate("task:sessionRecoveryDeliveryAttempt");
  return translate("task:sessionRecoveryResumeAttempt");
}
