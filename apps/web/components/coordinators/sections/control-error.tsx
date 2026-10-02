"use client";

import { useTranslation } from "react-i18next";
import type { ControlFieldError } from "@/hooks/domains/coordinator/use-control-draft";

export const CODE_KEYS: Record<string, string> = {
  invalid_setting: "coordinator:controlErrorInvalidSetting",
  stop_denied_only: "coordinator:controlErrorInvalidSetting",
  automatic_not_available: "coordinator:controlErrorAutomatic",
  not_eligible: "coordinator:controlErrorNotEligible",
  watches_foreign_workflow: "coordinator:controlErrorForeignBoard",
  watches_empty: "coordinator:watchesKeepOneBoard",
  watches_too_many: "coordinator:watchesAtMost",
  policy_denied: "coordinator:controlErrorPolicyDenied",
  invalid_body: "coordinator:controlErrorInvalidBody",
  invalid_scope: "coordinator:controlErrorInvalidScope",
  action_missing: "coordinator:controlErrorActionMissing",
  watches_duplicate: "coordinator:controlErrorDuplicateBoard",
  unknown_action: "coordinator:controlErrorUnknownAction",
  invalid_projects: "coordinator:controlErrorInvalidProjects",
  projects_empty: "coordinator:watchesKeepOneProject",
  projects_too_many: "coordinator:watchesProjectsAtMost",
  projects_duplicate: "coordinator:controlErrorDuplicateProject",
  projects_foreign_entry: "coordinator:controlErrorForeignProject",
};

/** One inline message for a rejected save, above the save bar. */
export function ControlError({ error }: { error: ControlFieldError | null }) {
  const { t } = useTranslation();
  if (!error) return null;
  const key = (error.code && CODE_KEYS[error.code]) || "coordinator:controlErrorInvalidBody";
  const message = t(key, { field: error.field ?? "", detail: error.detail });
  return (
    <p
      role="alert"
      className="text-sm text-destructive"
      data-testid="control-error"
      data-field={error.field ?? ""}
    >
      {error.field === "projects" ? `${t("coordinator:watchesProjects")}: ${message}` : message}
    </p>
  );
}
