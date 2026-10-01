"use client";

import { useTranslation } from "react-i18next";

type ReviewDiffStatePlaceholderProps = { state: "pending" | "unavailable" };

export function ReviewDiffStatePlaceholder({ state }: ReviewDiffStatePlaceholderProps) {
  const { t } = useTranslation();
  const pending = state === "pending";
  return (
    <div
      className="flex items-center justify-center py-12 text-muted-foreground text-sm"
      data-testid={pending ? "review-diff-pending" : "review-diff-unavailable"}
      role="status"
    >
      {pending ? t("task:gitDiffLoading") : t("task:gitDiffUnavailable")}
    </div>
  );
}
