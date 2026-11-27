"use client";

import {
  IconArrowLeft,
  IconChevronLeft,
  IconChevronRight,
  IconLayoutColumns,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import type { ReviewItem } from "@/lib/question-review/segments";
import { cn } from "@/lib/utils";
import type { ReviewItems } from "./use-review-items";
import { ReviewBottomBar } from "./review-layout";
import { reviewControlSize } from "./review-zoom-controls";

const KIND_KEYS: Record<ReviewItem["kind"], string> = {
  pair: "task:questionReviewItemPair",
  single: "task:questionReviewItemSingle",
  gallery: "task:questionReviewItemImage",
};

const navButton =
  "h-7 w-7 shrink-0 cursor-pointer [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11 max-md:h-11 max-md:w-11";

/** "Item N of M" with the item's kind, and Previous and Next item controls. */
export function ItemNav({ items, compact = false }: { items: ReviewItems; compact?: boolean }) {
  const { t } = useTranslation();
  if (items.count < 2 || items.compare || !items.item) return null;
  const kind = t(KIND_KEYS[items.item.kind]);
  const position = { index: items.index + 1, total: items.count, kind };
  return (
    <div className="flex shrink-0 items-center gap-1" data-testid="question-review-item-nav">
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className={navButton}
        aria-label={t("task:questionReviewPreviousItem")}
        title={t("task:questionReviewPreviousItem")}
        disabled={!items.canPrev}
        data-testid="question-review-item-prev"
        onClick={() => items.step(-1)}
      >
        <IconChevronLeft className="h-4 w-4" aria-hidden="true" />
      </Button>
      <span
        className="text-xs whitespace-nowrap text-muted-foreground tabular-nums"
        aria-live="polite"
        data-testid="question-review-item-position"
      >
        {t(
          compact ? "task:questionReviewItemPositionShort" : "task:questionReviewItemPosition",
          position,
        )}
      </span>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className={navButton}
        aria-label={t("task:questionReviewNextItem")}
        title={t("task:questionReviewNextItem")}
        disabled={!items.canNext}
        data-testid="question-review-item-next"
        onClick={() => items.step(1)}
      >
        <IconChevronRight className="h-4 w-4" aria-hidden="true" />
      </Button>
    </div>
  );
}

/**
 * Phones: item navigation gets its own row of the bottom bar, so it never
 * squeezes the comparison controls below touch size.
 */
export function ItemNavRow({ items }: { items: ReviewItems }) {
  if (items.count < 2 || items.compare) return null;
  return (
    <ReviewBottomBar>
      <div className="flex w-full justify-center">
        <ItemNav items={items} compact />
      </div>
    </ReviewBottomBar>
  );
}

type ModeButtonProps = {
  label: string;
  shortLabel: string;
  short: boolean;
  icon: ReactNode;
  testId: string;
  onClick: () => void;
};

/** Phones show the short label; the accessible name keeps the full one. */
function ItemModeButton({ label, shortLabel, short, icon, testId, onClick }: ModeButtonProps) {
  return (
    <Button
      type="button"
      variant="outline"
      className={cn(reviewControlSize, "shrink-0 gap-1.5")}
      aria-label={short ? label : undefined}
      data-testid={testId}
      onClick={onClick}
    >
      {icon}
      {short ? shortLabel : label}
    </Button>
  );
}

/** Starts an operator comparison from the current gallery image. */
export function CompareImagesButton({
  items,
  short = false,
}: {
  items: ReviewItems;
  short?: boolean;
}) {
  const { t } = useTranslation();
  if (!items.canCompare || items.compare) return null;
  return (
    <ItemModeButton
      label={t("task:questionReviewCompareImages")}
      shortLabel={t("task:questionReviewCompareShort")}
      short={short}
      icon={<IconLayoutColumns className="h-4 w-4" aria-hidden="true" />}
      testId="question-review-compare-images"
      onClick={items.startCompare}
    />
  );
}

/** Ends an operator comparison and returns to the gallery item shown as A. */
export function BackToGalleryButton({
  items,
  aId,
  short = false,
}: {
  items: ReviewItems;
  aId: string;
  short?: boolean;
}) {
  const { t } = useTranslation();
  if (!items.compare) return null;
  return (
    <ItemModeButton
      label={t("task:questionReviewBackToGallery")}
      shortLabel={t("task:questionReviewGalleryShort")}
      short={short}
      icon={<IconArrowLeft className="h-4 w-4" aria-hidden="true" />}
      testId="question-review-back-to-gallery"
      onClick={() => items.backToGallery(aId)}
    />
  );
}

/**
 * Phones: Back to gallery takes the bottom-bar row item navigation uses, with
 * room at the end for candidate stepping.
 */
export function BackToGalleryRow({
  items,
  aId,
  children,
}: {
  items: ReviewItems;
  aId: string;
  children?: ReactNode;
}) {
  if (!items.compare) return null;
  return (
    <ReviewBottomBar>
      <BackToGalleryButton items={items} aId={aId} short />
      <div className="ml-auto">{children}</div>
    </ReviewBottomBar>
  );
}
