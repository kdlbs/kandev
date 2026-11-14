"use client";
import { useTranslation } from "react-i18next";
import { formatDateTime } from "@/lib/i18n/formats";
import { parseTurnTimestamp } from "@/lib/state/slices/session/turn-actions";

// Shared presentation for the install-wide retention cards. The skip vocabulary
// is one reducer's vocabulary, so both policies report the same reasons and
// render them through one component instead of two copies that can drift.

export function retentionDate(value?: string) {
  return value && parseTurnTimestamp(value) !== null ? formatDateTime(value) : "";
}

const skipReasons = new Set([
  "eligibility_budget",
  "protected_tasks",
  "oversize",
  "already_removed",
  "no_payload",
  "unsupported",
  "malformed",
  "invalid_message",
]);

export function RetentionSkippedItems({
  skipped,
  labelKey,
}: {
  skipped: Record<string, number>;
  labelKey: string;
}) {
  const { t } = useTranslation();
  const entries = Object.entries(skipped).filter(([, count]) => count > 0);
  if (entries.length === 0) return null;
  return (
    <dl className="space-y-1 text-xs text-muted-foreground">
      {entries.map(([reason, count]) => (
        <div key={reason} className="flex flex-wrap gap-2">
          <dt>{t(`${labelKey}.${skipReasons.has(reason) ? reason : "other"}`)}</dt>
          <dd>{count}</dd>
        </div>
      ))}
    </dl>
  );
}

export function skippedTotal(skipped: Record<string, number> | undefined) {
  return Object.values(skipped ?? {}).reduce((sum, value) => sum + value, 0);
}
