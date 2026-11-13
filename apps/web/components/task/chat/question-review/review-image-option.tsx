"use client";

import { IconCheck, IconPhoto } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { imageShortName, type ReviewImage } from "@/lib/question-review/segments";
import { cn } from "@/lib/utils";
import { useReviewImageLabel } from "./review-image-picker";

export type ImageSlot = "a" | "b";

type Props = {
  slot: ImageSlot;
  images: readonly ReviewImage[];
  aId: string;
  bId: string;
  /** Loaded blob URLs; rows without one show a placeholder instead of reading the file. */
  urls: Record<string, string | undefined>;
  onPick: (slot: ImageSlot, id: string) => void;
};

/**
 * Image choices for one slot, shared by the desktop popovers and the phone
 * sheet: thumbnail, nickname, description, choice, and where each image is now.
 */
export function ReviewImageOptions({ slot, images, aId, bId, urls, onPick }: Props) {
  const selected = slot === "a" ? aId : bId;
  const other = slot === "a" ? bId : aId;
  return (
    <div className="space-y-1">
      {images.map((image, index) => (
        <ImageOptionRow
          key={image.id}
          slot={slot}
          image={image}
          index={index}
          url={urls[image.id]}
          selected={image.id === selected}
          inOther={image.id === other}
          onPick={() => onPick(slot, image.id)}
        />
      ))}
    </div>
  );
}

function ImageOptionRow(props: {
  slot: ImageSlot;
  image: ReviewImage;
  index: number;
  url?: string;
  selected: boolean;
  inOther: boolean;
  onPick: () => void;
}) {
  const { t } = useTranslation();
  const imageLabel = useReviewImageLabel();
  const { image, index } = props;
  return (
    <button
      type="button"
      aria-pressed={props.selected}
      aria-label={imageLabel(image, index)}
      data-testid={`question-review-pick-${props.slot}-option`}
      className={cn(
        "flex min-h-14 w-full cursor-pointer items-start gap-2 rounded-md border p-1.5 text-left text-xs",
        props.selected ? "border-primary bg-primary/10" : "border-transparent hover:bg-muted",
      )}
      onClick={props.onPick}
    >
      <span className="flex h-12 w-16 shrink-0 items-center justify-center overflow-hidden rounded-sm bg-muted/60">
        {props.url ? (
          <img
            src={props.url}
            alt=""
            draggable={false}
            className="max-h-full max-w-full object-contain"
          />
        ) : (
          <IconPhoto className="h-4 w-4 text-muted-foreground" aria-hidden="true" />
        )}
      </span>
      <span className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="flex items-center gap-1.5">
          <span className="truncate font-medium">{imageShortName(image, index)}</span>
          {props.inOther && (
            <span className="shrink-0 rounded-sm bg-muted px-1 text-[10px] text-muted-foreground">
              {t(props.slot === "a" ? "task:questionReviewInB" : "task:questionReviewInA")}
            </span>
          )}
          {props.selected && (
            <IconCheck className="ml-auto h-3.5 w-3.5 shrink-0 text-primary" aria-hidden="true" />
          )}
        </span>
        <span className="line-clamp-2 text-muted-foreground">{image.alt}</span>
        {image.choiceLabel && (
          <span className="truncate text-muted-foreground">
            {t("task:questionReviewChoiceName", { choice: image.choiceLabel })}
          </span>
        )}
      </span>
    </button>
  );
}
