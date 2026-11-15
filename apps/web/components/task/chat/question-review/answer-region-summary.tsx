"use client";

import { useTranslation } from "react-i18next";
import type { ClarificationAnswerAnnotation, ClarificationQuestion } from "@/lib/types/http";
import { buildSegmentImages } from "@/lib/question-review/segments";

function isRegion(value: unknown): value is ClarificationAnswerAnnotation {
  if (!value || typeof value !== "object") return false;
  const region = value as Partial<ClarificationAnswerAnnotation>;
  return typeof region.image_id === "string" && typeof region.comment === "string";
}

/** Regions submitted with an answer, as plain text in the answered question's history. */
export function AnswerRegionSummary({
  question,
  annotations,
}: {
  question: ClarificationQuestion;
  annotations: readonly ClarificationAnswerAnnotation[] | undefined;
}) {
  const { t } = useTranslation();
  const regions = Array.isArray(annotations) ? annotations.filter(isRegion) : [];
  if (regions.length === 0) return null;
  const images = buildSegmentImages(question);
  return (
    <ol className="mt-1 space-y-0.5" data-testid="clarification-answer-regions">
      {regions.map((region, index) => (
        <li key={index} className="break-words">
          {t("task:questionReviewRegionHistory", {
            number: index + 1,
            image: images.find((image) => image.id === region.image_id)?.alt ?? region.image_id,
            comment: region.comment,
          })}
        </li>
      ))}
    </ol>
  );
}
