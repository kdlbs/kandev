"use client";

import { useTranslation } from "react-i18next";

type ReviewDiffStatePlaceholderProps = {
  state: "pending" | "unavailable";
  retainedContent?: boolean;
};

function placeholderTestId(state: "pending" | "unavailable", retainedContent: boolean) {
  if (retainedContent) return "review-diff-refresh-status";
  return state === "pending" ? "review-diff-pending" : "review-diff-unavailable";
}

export function ReviewDiffStatePlaceholder({
  state,
  retainedContent = false,
}: ReviewDiffStatePlaceholderProps) {
  const { t } = useTranslation();
  const pending = state === "pending";
  return (
    <div
      className={
        retainedContent
          ? "flex items-center justify-end px-2 py-1 text-muted-foreground text-xs"
          : "flex items-center justify-center py-12 text-muted-foreground text-sm"
      }
      data-testid={placeholderTestId(state, retainedContent)}
      role="status"
    >
      {pending ? t("task:gitDiffLoading") : t("task:gitDiffUnavailable")}
    </div>
  );
}
