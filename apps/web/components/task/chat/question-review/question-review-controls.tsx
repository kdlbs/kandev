"use client";

import type { RefObject } from "react";
import { IconPhoto } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import type { ClarificationOption, ClarificationQuestion } from "@/lib/types/http";
import {
  buildSegmentImages,
  imageShortName,
  segmentImageIndex,
} from "@/lib/question-review/segments";

/** Opens the large review view from the compact question panel. */
export function QuestionReviewButton({
  triggerRef,
  onOpen,
  disabled,
}: {
  triggerRef: RefObject<HTMLButtonElement | null>;
  onOpen: () => void;
  disabled: boolean;
}) {
  const { t } = useTranslation();
  return (
    <Button
      ref={triggerRef}
      type="button"
      variant="outline"
      size="sm"
      disabled={disabled}
      aria-haspopup="dialog"
      data-testid="question-review-open"
      onClick={onOpen}
      className="h-7 cursor-pointer gap-1.5 px-2 text-xs max-md:h-11 [@media(pointer:coarse)]:h-11"
    >
      <IconPhoto className="h-3.5 w-3.5" aria-hidden="true" />
      {t("task:questionReviewOpen")}
    </Button>
  );
}

/** Replaces the compact form while the same form is shown in the large view. */
export function QuestionReviewPlaceholder() {
  const { t } = useTranslation();
  return (
    <p
      data-testid="question-review-placeholder"
      className="flex min-h-11 items-center px-4 py-2 text-sm text-muted-foreground"
    >
      {t("task:questionReviewPlaceholder")}
    </p>
  );
}

/** The image a choice shows on the review stage when it is selected, if any. */
export function choiceStageImageId(option: ClarificationOption | undefined): string | undefined {
  return option?.images?.find((image) => typeof image?.id === "string")?.id;
}

/** Marks a choice whose image the review stage shows when the choice is selected. */
export function ChoiceStageImageHint({
  question,
  option,
}: {
  question: ClarificationQuestion;
  option: ClarificationOption;
}) {
  const { t } = useTranslation();
  const imageId = choiceStageImageId(option);
  const images = buildSegmentImages(question);
  const index = imageId ? segmentImageIndex(images, imageId) : 0;
  if (index === 0) return null;
  const name = imageShortName(images[index - 1], index - 1);
  return (
    <span
      data-testid="question-review-choice-image"
      className="mt-0.5 inline-flex flex-shrink-0 items-center gap-1 text-xs text-muted-foreground"
    >
      <IconPhoto className="h-3.5 w-3.5" aria-hidden="true" />
      <span className="sr-only">{t("task:questionReviewChoiceImageHint", { index: name })}</span>
      <span aria-hidden="true" className="max-w-24 truncate">
        {name}
      </span>
    </span>
  );
}
