"use client";

import { useTranslation } from "react-i18next";
import { imageShortName, type ReviewImage } from "@/lib/question-review/segments";

export function useReviewImageLabel() {
  const { t } = useTranslation();
  return (image: ReviewImage, index: number) =>
    image.choiceLabel === undefined
      ? t("task:questionReviewImageLabel", { index: imageShortName(image, index), alt: image.alt })
      : t("task:questionReviewChoiceImageLabel", {
          index: imageShortName(image, index),
          alt: image.alt,
          choice: image.choiceLabel,
        });
}
