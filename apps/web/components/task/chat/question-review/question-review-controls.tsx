"use client";

import { useContext, type RefObject } from "react";
import { IconPhoto } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import type { ClarificationOption, ClarificationQuestion } from "@/lib/types/http";
import {
  buildSegmentImages,
  imageShortName,
  segmentImageIndex,
} from "@/lib/question-review/segments";
import { ShowImageContext } from "./review-image-request";

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

/** Shows a choice's image on the review stage, which replaces the compact thumbnail. */
export function ChoiceStageImageRef({
  question,
  option,
}: {
  question: ClarificationQuestion;
  option: ClarificationOption;
}) {
  const { t } = useTranslation();
  const show = useContext(ShowImageContext);
  const imageId = option.images?.find((image) => typeof image?.id === "string")?.id;
  const images = buildSegmentImages(question);
  const index = imageId ? segmentImageIndex(images, imageId) : 0;
  if (!imageId || index === 0) return null;
  return (
    <Button
      type="button"
      variant="link"
      size="sm"
      className="h-auto cursor-pointer gap-1.5 p-0 text-xs max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
      data-testid="question-review-choice-image"
      disabled={!show}
      onClick={() => show?.(imageId)}
    >
      <IconPhoto className="h-3.5 w-3.5" aria-hidden="true" />
      {t("task:questionReviewChoiceImageRef", {
        index: imageShortName(images[index - 1], index - 1),
      })}
    </Button>
  );
}
