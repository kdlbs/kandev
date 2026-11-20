"use client";

import { IconCheck, IconPencil } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import { MAX_REGIONS } from "@/lib/question-review/regions";
import { reviewControlSize } from "./review-zoom-controls";

/** Mark area toggle: a pen icon, and solid primary with a check while marking. */
export function AnnotateButton({
  pressed,
  disabled,
  count,
  onToggle,
}: {
  pressed: boolean;
  disabled: boolean;
  count: number;
  onToggle: () => void;
}) {
  const { t } = useTranslation();
  const Icon = pressed ? IconCheck : IconPencil;
  return (
    <Button
      type="button"
      variant={pressed ? "default" : "outline"}
      className={reviewControlSize}
      aria-pressed={pressed}
      disabled={disabled}
      title={
        count >= MAX_REGIONS ? t("task:questionReviewRegionLimit", { max: MAX_REGIONS }) : undefined
      }
      data-testid="question-review-annotate"
      onClick={onToggle}
    >
      <Icon className="h-3.5 w-3.5" aria-hidden="true" />
      {t(pressed ? "task:questionReviewMarking" : "task:questionReviewMarkArea")}
    </Button>
  );
}
