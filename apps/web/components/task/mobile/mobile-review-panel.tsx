import type { ReviewItemSummary } from "@/lib/plugins/types";
import type { MobileSessionPanel } from "@/lib/state/slices/ui/types";
import { t } from "@/lib/i18n";
import { ReviewDetailPanelComponent } from "../review-detail-panel";
import { ReviewItemSelector } from "../review-item-selector";

type MobileReviewPanelProps = {
  currentMobilePanel: MobileSessionPanel;
  reviews: readonly ReviewItemSummary[];
  selectedReview: ReviewItemSummary | null;
  onSelectReview: (review: ReviewItemSummary) => void;
};

export function MobileReviewPanel({
  currentMobilePanel,
  reviews,
  selectedReview,
  onSelectReview,
}: MobileReviewPanelProps) {
  if (currentMobilePanel !== "review") return null;
  if (reviews.length === 0) return null;
  return (
    <div className="flex min-h-0 flex-1 flex-col" data-testid="mobile-review-panel">
      <div className="shrink-0 px-2 py-2">
        <ReviewItemSelector
          reviews={reviews}
          selectedReview={selectedReview}
          onSelectReview={onSelectReview}
          presentation="mobile"
          className="w-full"
        />
      </div>
      {selectedReview ? (
        <div className="min-h-0 flex-1">
          <ReviewDetailPanelComponent
            key={`${selectedReview.providerId}:${selectedReview.reviewKey}`}
            panelId={`mobile-review-${selectedReview.providerId}`}
            params={{ providerId: selectedReview.providerId, reviewKey: selectedReview.reviewKey }}
            presentation="mobile"
          />
        </div>
      ) : (
        <div className="flex flex-1 items-center justify-center px-6 text-center text-sm text-muted-foreground">
          {t("integrations:chooseReviewToOpen")}
        </div>
      )}
    </div>
  );
}
