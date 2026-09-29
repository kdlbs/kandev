"use client";

import { useTranslation } from "react-i18next";
import type { ControlFieldError } from "@/hooks/domains/coordinator/use-control-draft";

const CODE_KEYS: Record<string, string> = {
  invalid_setting: "coordinator:controlErrorInvalidSetting",
  stop_denied_only: "coordinator:controlErrorInvalidSetting",
  automatic_not_available: "coordinator:controlErrorAutomatic",
  watches_foreign_workflow: "coordinator:controlErrorForeignBoard",
  watches_empty: "coordinator:watchesKeepOneBoard",
  watches_too_many: "coordinator:watchesAtMost",
  policy_denied: "coordinator:controlErrorPolicyDenied",
  invalid_body: "coordinator:controlErrorInvalidBody",
};

/** One inline message for a rejected save, above the save bar. */
export function ControlError({ error }: { error: ControlFieldError | null }) {
  const { t } = useTranslation();
  if (!error) return null;
  const key = (error.code && CODE_KEYS[error.code]) || "coordinator:controlErrorInvalidBody";
  return (
    <p
      role="alert"
      className="text-sm text-destructive"
      data-testid="control-error"
      data-field={error.field ?? ""}
    >
      {t(key, { field: error.field ?? "", detail: error.detail })}
    </p>
  );
}
