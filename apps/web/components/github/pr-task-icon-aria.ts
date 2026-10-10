import type { TFunction } from "i18next";

export function buildPRTaskIconAriaLabel({
  t,
  number,
  statusCount,
  statusLabels,
  hasConflicts,
  autoFixEnabled,
  autoMergeEnabled,
}: {
  t: TFunction;
  number?: number;
  statusCount?: number;
  statusLabels: string[];
  hasConflicts: boolean;
  autoFixEnabled: boolean;
  autoMergeEnabled: boolean;
}) {
  const statusAriaLabel = statusCount
    ? t("github:pullRequestStatuses", { count: statusCount })
    : t("github:pullRequestStatus", { number });
  const automationLabels = [
    autoFixEnabled ? t("github:autoFixEnabledAria") : null,
    autoMergeEnabled ? t("github:autoMergeEnabledAria") : null,
  ];
  return [
    statusAriaLabel,
    ...statusLabels,
    hasConflicts ? t("github:conflicts") : null,
    ...automationLabels,
  ]
    .filter(Boolean)
    .join(", ");
}
