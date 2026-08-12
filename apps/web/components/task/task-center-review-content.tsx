"use client";

import { TabsContent } from "@kandev/ui/tabs";
import type { ReviewItemSummary } from "@/lib/plugins/types";
import { t } from "@/lib/i18n";
import { ReviewDetailPanelComponent } from "./review-detail-panel";
import { ReviewItemSelector } from "./review-item-selector";
import { reviewItemId } from "./review-selection";

export function TaskCenterReviewContent({
  reviews,
  selectedReview,
  onSelectReview,
}: {
  reviews: readonly ReviewItemSummary[];
  selectedReview: ReviewItemSummary | null;
  onSelectReview: (review: ReviewItemSummary) => void;
}) {
  return (
    <TabsContent value="pr" className="flex-1 min-h-0" data-testid="review-detail-panel">
      <div className="flex h-full min-h-0 flex-col gap-2 p-2">
        <ReviewItemSelector
          reviews={reviews}
          selectedReview={selectedReview}
          onSelectReview={onSelectReview}
          presentation="desktop"
        />
        {selectedReview ? (
          <div className="min-h-0 flex-1">
            <ReviewDetailPanelComponent
              key={reviewItemId(selectedReview)}
              panelId="task-center-review"
              params={{
                providerId: selectedReview.providerId,
                reviewKey: selectedReview.reviewKey,
                connectionScope: selectedReview.connectionScope,
                repositoryId: selectedReview.repositoryId,
                changeRequestNumber: selectedReview.changeRequestNumber,
              }}
            />
          </div>
        ) : (
          <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
            {t("integrations:chooseReviewToOpen")}
          </div>
        )}
      </div>
    </TabsContent>
  );
}
