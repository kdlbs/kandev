import { useTranslation } from "react-i18next";

import { sanitizeSessionErrorDetails } from "@/lib/session-error-details";

type PartialFeedbackChange = {
  label: string;
  requested: string;
  effective?: string;
  status: string;
  error?: unknown;
};

export function partialConfigurationFeedbackText({
  changes,
  failed,
  reason,
  t,
}: {
  changes: PartialFeedbackChange[];
  failed?: { label: string; value: string };
  reason: string;
  t: ReturnType<typeof useTranslation>["t"];
}) {
  const parts: string[] = [];
  const applied = changes
    .filter((change) => change.status === "applied")
    .map((change) => `${change.label}: ${change.effective ?? change.requested}`)
    .join(", ");
  const clamped = changes.filter((change) => change.status === "clamped");
  const saved = changes
    .filter((change) => change.status === "saved_for_launch")
    .map((change) => `${change.label}: ${change.requested}`)
    .join(", ");
  const unconfirmed = changes
    .filter((change) => change.status === "unconfirmed")
    .map((change) => `${change.label}: ${change.requested}`)
    .join(", ");

  if (applied) parts.push(t("task:sessionConfigurationApplied", { value: applied }));
  clamped.forEach((change) =>
    parts.push(
      t("task:sessionConfigurationClamped", {
        requested: `${change.label}: ${change.requested}`,
        effective: `${change.label}: ${change.effective ?? change.requested}`,
      }),
    ),
  );
  if (saved) parts.push(t("task:sessionConfigurationSaved", { value: saved }));
  if (unconfirmed) parts.push(t("task:sessionConfigurationUnconfirmed", { value: unconfirmed }));
  changes
    .filter((change) => change.status === "persistence_failed")
    .forEach((change) => {
      const safeReason = sanitizeSessionErrorDetails(change.error) || reason;
      parts.push(
        change.effective
          ? t("task:sessionConfigurationPersistenceFailed", {
              value: `${change.label}: ${change.effective}`,
              reason: safeReason,
            })
          : t("task:sessionConfigurationSaveFailed", {
              value: `${change.label}: ${change.requested}`,
              reason: safeReason,
            }),
      );
    });
  if (failed) {
    parts.push(
      t("task:sessionConfigurationRejected", {
        value: `${failed.label}: ${failed.value}`,
        reason,
      }),
    );
  }
  return parts.join(" ");
}
