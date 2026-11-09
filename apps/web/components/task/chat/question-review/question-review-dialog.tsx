"use client";

import {
  useCallback,
  useMemo,
  useState,
  type Dispatch,
  type ReactNode,
  type SetStateAction,
} from "react";
import type { CompareSettings } from "@/lib/question-review/compare";
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
import { ReviewLayoutProvider } from "./review-layout";
import { RegionsSheet, RegionsSidePanel } from "./review-region-panel";
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

function isTextEntry(target: EventTarget | null): boolean {
  if (target instanceof HTMLTextAreaElement) return true;
  if (!(target instanceof HTMLInputElement)) return false;
  return !["range", "checkbox", "radio", "button", "submit", "file"].includes(target.type);
}

/** Hides the phone bottom bar while a text field has focus, so it never covers the keyboard. */
function useTextEntryFocus() {
  const [typing, setTyping] = useState(false);
  return {
    typing,
    onFocusCapture: (event: React.FocusEvent) => setTyping(isTextEntry(event.target)),
    onBlurCapture: (event: React.FocusEvent) => {
      if (isTextEntry(event.target) && !isTextEntry(event.relatedTarget)) setTyping(false);
    },
  };
}

export function QuestionReviewDialog(props: Props) {
  const breakpoint = useResponsiveBreakpoint();
  const phone = breakpoint.isMobile;
  const { review } = props;
  const guard = useCallback(() => true, []);
  useClarificationEscapeGuard(guard);
  const annotations = useReviewAnnotations(props.annotations, props.setAnnotations);
  const [bottomBar, setBottomBar] = useState<HTMLDivElement | null>(null);
  const [compareSettings, setCompareSettings] = useState<CompareSettings>();
  const focus = useTextEntryFocus();
  const layout = useMemo(
    () => ({ phone, touch: phone || !breakpoint.isFinePointer, bottomBar }),
    [bottomBar, breakpoint.isFinePointer, phone],
  );
  const segment = props.segments[props.activeIndex] ?? props.segments[0];
  const stage = segment ? (
    <QuestionReviewStage
      key={segment.questionId}
      pendingId={props.pendingId}
      sessionId={props.sessionId}
      segment={segment}
      annotations={annotations}
      popoverContainer={review.scopeRef}
      compareSettings={compareSettings}
      onCompareSettings={setCompareSettings}
    />
  ) : null;
  const form = (
    <div
      className={cn("min-h-0 overflow-y-auto overscroll-contain", phone && "pb-2")}
      data-testid="question-review-form"
    >
      {props.form}
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
          // Escape closes an open comment, then leaves marking, then closes the view.
          if (annotations.commenting) event.preventDefault();
          else if (annotations.annotating) {
            event.preventDefault();
            annotations.setAnnotating(false);
          }
        }}
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          review.triggerRef.current?.focus();
        }}
        onFocusCapture={focus.onFocusCapture}
        onBlurCapture={focus.onBlurCapture}
        className={cn(
          "flex max-w-none flex-col gap-0 overflow-hidden p-0 data-open:animate-none data-closed:animate-none sm:max-w-none",
          phone
            ? "h-dvh w-screen rounded-none border-0 pt-[env(safe-area-inset-top)] pb-[env(safe-area-inset-bottom)]"
            : "h-[min(94dvh,1200px)] w-[min(96vw,1800px)]",
        )}
      >
        <ReviewLayoutProvider value={layout}>
          <div
            ref={review.scopeRef}
            tabIndex={-1}
            className="relative flex min-h-0 flex-1 flex-col outline-none"
          >
            <ReviewHeader {...props} />
            {phone ? (
              <PhoneBody
                stage={stage}
                form={form}
                hideBar={focus.typing}
                barRef={setBottomBar}
                regions={segment && <RegionsSheet segment={segment} controller={annotations} />}
              />
            ) : (
              <DesktopBody
                stage={stage}
                form={form}
                regions={segment && <RegionsSidePanel segment={segment} controller={annotations} />}
              />
            )}
          </div>
        </ReviewLayoutProvider>
      </DialogContent>
    </Dialog>
  );
}

type BodyProps = { stage: ReactNode; form: ReactNode; regions: ReactNode };

function PhoneBody(
  props: BodyProps & { hideBar: boolean; barRef: (element: HTMLDivElement | null) => void },
) {
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex h-[min(55dvh,32rem)] shrink-0 flex-col px-3 pb-2">{props.stage}</div>
      <div className="flex min-h-0 flex-1 flex-col border-t border-border">{props.form}</div>
      <div
        ref={props.barRef}
        className={cn(
          "flex shrink-0 items-center gap-2 border-t border-border bg-background px-3 py-2",
          props.hideBar && "hidden",
        )}
        data-testid="question-review-bottom-bar"
      />
      {props.regions}
    </div>
  );
}

function DesktopBody(props: BodyProps) {
  const { t } = useTranslation();
  return (
    <ResizablePanelGroup orientation="vertical" className="min-h-0 flex-1">
      <ResizablePanel defaultSize="62%" minSize="30%" className="flex flex-row gap-3 px-4 pb-3">
        <div className="flex min-w-0 flex-1 flex-col">{props.stage}</div>
        {props.regions}
      </ResizablePanel>
      <ResizableHandle withHandle aria-label={t("task:questionReviewResize")} />
      <ResizablePanel defaultSize="38%" minSize="25%" className="flex flex-col">
        {props.form}
      </ResizablePanel>
    </ResizablePanelGroup>
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
