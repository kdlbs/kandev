"use client";

import { ToggleGroup, ToggleGroupItem } from "@kandev/ui/toggle-group";
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

type Props = {
  label: string;
  images: readonly ReviewImage[];
  value: string;
  onChange: (id: string) => void;
  testId: string;
};

/** Picks one segment image. Buttons show the stage number; names carry the description. */
export function ReviewImagePicker({ label, images, value, onChange, testId }: Props) {
  const imageLabel = useReviewImageLabel();
  return (
    <div className="flex items-center gap-1.5">
      <span className="text-xs text-muted-foreground" aria-hidden="true">
        {label}
      </span>
      <ToggleGroup
        type="single"
        variant="outline"
        size="sm"
        aria-label={label}
        value={value}
        onValueChange={(next) => {
          if (next) onChange(next);
        }}
        data-testid={testId}
      >
        {images.map((image, index) => (
          <ToggleGroupItem
            key={image.id}
            value={image.id}
            aria-label={imageLabel(image, index)}
            title={imageLabel(image, index)}
            className="h-7 min-w-7 cursor-pointer [@media(pointer:coarse)]:h-12 [@media(pointer:coarse)]:min-w-12 max-md:h-12 max-md:min-w-12"
          >
            <span className="max-w-24 truncate">{imageShortName(image, index)}</span>
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
    </div>
  );
}
