"use client";

import { useCallback, type Dispatch, type ReactNode, type SetStateAction } from "react";
import { IconCheck } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@kandev/ui/dialog";
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from "@kandev/ui/resizable";
import { useTranslation } from "react-i18next";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useClarificationEscapeGuard } from "@/hooks/use-clarification-escape-guard";
import type { ReviewSegment } from "@/lib/question-review/segments";
import { cn } from "@/lib/utils";
import { QuestionReviewSegments } from "./question-review-segments";
import { QuestionReviewStage } from "./question-review-stage";
import { ReviewRegionList } from "./review-region-list";
import type { QuestionReviewController } from "./use-question-review";
import { useReviewAnnotations, type ReviewAnnotations } from "./use-review-annotations";

export type QuestionReviewSubmit = {
  show: boolean;
  enabled: boolean;
  submitting: boolean;
  onSubmit: () => void;
};

type Props = {
  review: QuestionReviewController;
  segments: readonly ReviewSegment[];
  activeIndex: number;
  onSelectSegment: (index: number) => void;
  isAnswered: (index: number) => boolean;
  pendingId: string;
  sessionId: string;
  form: ReactNode;
  submit: QuestionReviewSubmit;
  headerActions?: ReactNode;
  annotations: ReviewAnnotations;
  setAnnotations: Dispatch<SetStateAction<ReviewAnnotations>>;
};

const touchSize = "max-md:h-12 [@media(pointer:coarse)]:h-12";

export function QuestionReviewDialog(props: Props) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const { review } = props;
  const guard = useCallback(() => true, []);
  useClarificationEscapeGuard(guard);
  const annotations = useReviewAnnotations(props.annotations, props.setAnnotations);
  const segment = props.segments[props.activeIndex] ?? props.segments[0];
  const stage = segment ? (
    <QuestionReviewStage
      key={segment.questionId}
      pendingId={props.pendingId}
      sessionId={props.sessionId}
      segment={segment}
      annotations={annotations}
      popoverContainer={review.scopeRef}
    />
  ) : null;
  const form = (
    <div className="min-h-0 overflow-y-auto overscroll-contain" data-testid="question-review-form">
      {props.form}
      {segment && <ReviewRegionList segment={segment} controller={annotations} />}
    </div>
  );
  return (
    <Dialog open onOpenChange={(open) => !open && review.closeReview()}>
      <DialogContent
        showCloseButton={false}
        enterConfirms={false}
        data-testid="question-review-dialog"
        overlayClassName="data-open:animate-none data-closed:animate-none"
        onEscapeKeyDown={(event) => {
          // Escape closes an open region comment first, not the whole view.
          if (annotations.commenting) event.preventDefault();
        }}
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          review.triggerRef.current?.focus();
        }}
        className={cn(
          "flex max-w-none flex-col gap-0 overflow-hidden p-0 data-open:animate-none data-closed:animate-none sm:max-w-none",
          isMobile
            ? "h-dvh w-screen rounded-none border-0 pt-[env(safe-area-inset-top)] pb-[env(safe-area-inset-bottom)]"
            : "h-[min(94dvh,1200px)] w-[min(96vw,1800px)]",
        )}
      >
        <div
          ref={review.scopeRef}
          tabIndex={-1}
          className="relative flex min-h-0 flex-1 flex-col outline-none"
        >
          <ReviewHeader {...props} />
          {isMobile ? (
            <div className="flex min-h-0 flex-1 flex-col">
              <div className="flex h-[min(55dvh,32rem)] shrink-0 flex-col px-3 pb-2">{stage}</div>
              <div className="flex min-h-0 flex-1 flex-col border-t border-border">{form}</div>
            </div>
          ) : (
            <ResizablePanelGroup orientation="vertical" className="min-h-0 flex-1">
              <ResizablePanel defaultSize="62%" minSize="30%" className="flex flex-col px-4 pb-3">
                {stage}
              </ResizablePanel>
              <ResizableHandle withHandle aria-label={t("task:questionReviewResize")} />
              <ResizablePanel defaultSize="38%" minSize="25%" className="flex flex-col">
                {form}
              </ResizablePanel>
            </ResizablePanelGroup>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}

function ReviewHeader(props: Props) {
  const { t } = useTranslation();
  const total = props.segments.length;
  return (
    <div className="flex shrink-0 flex-wrap items-center gap-x-3 gap-y-2 px-4 py-3 max-md:px-3">
      <div className="flex min-w-0 flex-col">
        <DialogTitle className="text-sm">{t("task:questionReviewTitle")}</DialogTitle>
        <DialogDescription className="text-xs text-muted-foreground">
          {t("task:questionReviewPosition", { index: props.activeIndex + 1, total })}
        </DialogDescription>
      </div>
      <div className="order-last w-full min-w-0 md:order-none md:w-auto md:flex-1">
        <QuestionReviewSegments
          segments={props.segments}
          activeIndex={props.activeIndex}
          isAnswered={props.isAnswered}
          onSelect={props.onSelectSegment}
          disabled={props.submit.submitting}
        />
      </div>
      <div className="ml-auto flex items-center gap-1.5">
        {props.headerActions}
        {props.submit.show && (
          <Button
            type="button"
            className={cn("h-7 cursor-pointer gap-1", touchSize)}
            disabled={!props.submit.enabled || props.submit.submitting}
            data-testid="question-review-submit"
            onClick={props.submit.onSubmit}
          >
            {props.submit.submitting ? t("task:submitting") : t("task:submit")}
            {!props.submit.submitting && <IconCheck className="h-3 w-3" aria-hidden="true" />}
          </Button>
        )}
        <Button
          type="button"
          variant="outline"
          className={cn("h-7 cursor-pointer", touchSize)}
          data-testid="question-review-close"
          onClick={props.review.closeReview}
        >
          {t("common:close")}
        </Button>
      </div>
    </div>
  );
}
