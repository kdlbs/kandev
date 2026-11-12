"use client";

import { IconCheck } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import type { ReviewSegment } from "@/lib/question-review/segments";
import { cn } from "@/lib/utils";

type Props = {
  segments: readonly ReviewSegment[];
  activeIndex: number;
  isAnswered: (index: number) => boolean;
  onSelect: (index: number) => void;
  disabled: boolean;
};

export function QuestionReviewSegments({
  segments,
  activeIndex,
  isAnswered,
  onSelect,
  disabled,
}: Props) {
  const { t } = useTranslation();
  if (segments.length < 2) return null;
  return (
    <nav
      aria-label={t("task:questionReviewSegments")}
      className="min-w-0 overflow-x-auto overscroll-x-contain"
    >
      <ol className="flex w-max gap-1.5">
        {segments.map((segment) => {
          const active = segment.index === activeIndex;
          const answered = isAnswered(segment.index);
          return (
            <li key={segment.questionId}>
              <button
                type="button"
                disabled={disabled}
                aria-current={active ? "step" : undefined}
                data-testid="question-review-segment"
                data-active={active ? "true" : undefined}
                onClick={() => onSelect(segment.index)}
                className={cn(
                  "flex h-8 max-w-56 cursor-pointer items-center gap-1.5 rounded-md border px-2 text-xs disabled:cursor-not-allowed disabled:opacity-50 max-md:h-12 [@media(pointer:coarse)]:h-12",
                  active ? "border-primary bg-primary/10" : "border-border hover:bg-muted",
                )}
              >
                <span className="font-medium tabular-nums">{segment.index + 1}</span>
                <span className="min-w-0 truncate max-md:sr-only">{segment.label}</span>
                <span className="shrink-0 text-muted-foreground max-md:sr-only">
                  {t("task:questionReviewSegmentImages", { count: segment.images.length })}
                </span>
                {answered && (
                  <IconCheck
                    className="h-3.5 w-3.5 shrink-0 text-emerald-600"
                    role="img"
                    aria-label={t("task:questionReviewAnswered")}
                  />
                )}
              </button>
            </li>
          );
        })}
      </ol>
    </nav>
  );
}
