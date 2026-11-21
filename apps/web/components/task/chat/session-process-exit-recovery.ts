import type { TFunction } from "i18next";
import type { SessionErrorDetailsField } from "@/lib/session-error-details";
import type { AgentErrorCause } from "@/lib/types/task-status-summary";

export function isProcessExitCause(cause: AgentErrorCause): boolean {
  return cause.code === "process_exited";
}

export function processExitSummary(cause: AgentErrorCause, translate: TFunction): string {
  return cause.reason === "mise_config_untrusted"
    ? translate("task:sessionBootstrapProcessMiseBody")
    : translate("task:sessionBootstrapProcessExitedBody");
}

export function processExitTitleKey(cause: AgentErrorCause): string {
  return cause.reason === "mise_config_untrusted"
    ? "task:sessionBootstrapProcessMiseTitle"
    : "task:sessionBootstrapProcessExitedTitle";
}

export function processExitPrerequisite(
  cause: AgentErrorCause | null,
  translate: TFunction,
): string | null {
  if (cause?.code !== "process_exited" || cause.reason !== "mise_config_untrusted") return null;
  return translate("task:sessionBootstrapProcessMisePrerequisite", { command: "mise trust" });
}

export function appendProcessExitDetail(
  fields: SessionErrorDetailsField[],
  cause: AgentErrorCause,
  translate: TFunction,
) {
  if (
    cause.code !== "process_exited" ||
    typeof cause.exit_code !== "number" ||
    !Number.isInteger(cause.exit_code)
  ) {
    return;
  }
  fields.push({
    label: translate("task:sessionErrorDetailExitCode"),
    value: String(cause.exit_code),
  });
}
