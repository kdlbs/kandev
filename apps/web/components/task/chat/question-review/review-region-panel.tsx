"use client";

import { useState } from "react";
import { IconLayoutSidebarRightCollapse, IconListDetails } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle } from "@kandev/ui/drawer";
import { useTranslation } from "react-i18next";
import type { ReviewSegment } from "@/lib/question-review/segments";
import { ReviewRegionList } from "./review-region-list";
import type { ReviewAnnotationController } from "./use-review-annotations";

/** Phone: opens the regions sheet; shows how many regions the question has. */
export function RegionsButton({
  controller,
  questionId,
}: {
  controller: ReviewAnnotationController;
  questionId: string;
}) {
  const { t } = useTranslation();
  const count = controller.list(questionId).length;
  return (
    <Button
      type="button"
      variant="outline"
      className="min-h-11 shrink-0 cursor-pointer"
      aria-haspopup="dialog"
      data-testid="question-review-regions-button"
      onClick={() => controller.setListOpen(true)}
    >
      <IconListDetails className="h-4 w-4" aria-hidden="true" />
      {t("task:questionReviewRegionsButton", { count })}
    </Button>
  );
}

export function RegionsSheet({
  segment,
  controller,
}: {
  segment: ReviewSegment;
  controller: ReviewAnnotationController;
}) {
  const { t } = useTranslation();
  return (
    <Drawer open={controller.listOpen} onOpenChange={controller.setListOpen}>
      <DrawerContent
        className="z-[60] max-h-[82dvh] pb-[calc(1rem+env(safe-area-inset-bottom))]"
        data-testid="question-review-regions-sheet"
      >
        <DrawerHeader className="text-left">
          <DrawerTitle>{t("task:questionReviewRegionsTitle")}</DrawerTitle>
        </DrawerHeader>
        <div className="min-h-0 overflow-y-auto px-4">
          <ReviewRegionList
            segment={segment}
            controller={controller}
            onBeforeEdit={() => controller.setListOpen(false)}
          />
        </div>
      </DrawerContent>
    </Drawer>
  );
}

/** Desktop: regions beside the image while marking or once the question has regions. */
export function RegionsSidePanel({
  segment,
  controller,
}: {
  segment: ReviewSegment;
  controller: ReviewAnnotationController;
}) {
  const { t } = useTranslation();
  const [collapsed, setCollapsed] = useState(false);
  const count = controller.list(segment.questionId).length;
  if (segment.images.length === 0 || (!controller.annotating && count === 0)) return null;
  if (collapsed) {
    return (
      <Button
        type="button"
        variant="outline"
        className="h-7 shrink-0 cursor-pointer self-start"
        data-testid="question-review-regions-expand"
        onClick={() => setCollapsed(false)}
      >
        <IconListDetails className="h-3.5 w-3.5" aria-hidden="true" />
        {t("task:questionReviewRegionsButton", { count })}
      </Button>
    );
  }
  return (
    <aside
      className="flex w-72 shrink-0 flex-col gap-2 overflow-y-auto border-l border-border pl-3"
      aria-label={t("task:questionReviewRegionsTitle")}
      data-testid="question-review-regions-panel"
    >
      <div className="flex justify-end">
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="h-7 w-7 cursor-pointer"
          aria-label={t("task:questionReviewRegionsHide")}
          onClick={() => setCollapsed(true)}
        >
          <IconLayoutSidebarRightCollapse className="h-4 w-4" aria-hidden="true" />
        </Button>
      </div>
      <ReviewRegionList segment={segment} controller={controller} />
    </aside>
  );
}
