"use client";

import { useTranslation } from "react-i18next";
import { formatDateTime } from "@/lib/i18n/formats";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";

const DATE_OPTIONS: Intl.DateTimeFormatOptions = {
  year: "numeric",
  month: "short",
  day: "numeric",
  hour: "numeric",
  minute: "2-digit",
};

export function WalkthroughGenerationTime({ updatedAt }: { updatedAt?: string }) {
  const { t } = useTranslation();
  if (!updatedAt || parseStrictRfc3339Timestamp(updatedAt) === null) return null;

  return (
    <time
      dateTime={updatedAt}
      title={t("diff:walkthroughGeneratedAt", {
        date: formatDateTime(updatedAt, { ...DATE_OPTIONS, timeZoneName: "short" }),
      })}
      data-testid="walkthrough-generated-at"
      className="ml-[calc(var(--spacing)*2+1px)] block text-left text-xs text-muted-foreground [overflow-wrap:anywhere]"
    >
      {t("diff:walkthroughGeneratedAt", {
        date: formatDateTime(updatedAt, DATE_OPTIONS),
      })}
    </time>
  );
}
