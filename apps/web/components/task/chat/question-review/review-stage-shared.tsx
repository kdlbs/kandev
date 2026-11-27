"use client";

import { useCallback, useMemo, useState, type KeyboardEvent, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  useQuestionImageOwner,
  useQuestionImages,
  type QuestionImageState,
} from "@/hooks/domains/session/use-question-images";
import type { ReviewSegment } from "@/lib/question-review/segments";
import type { Size } from "@/lib/question-review/viewport";
import { imageErrorKey } from "../question-images";
import { reviewControlSize } from "./review-zoom-controls";

/**
 * Keeps every key pressed in the stage area (toolbar and canvas) out of the
 * answer form's window-level shortcuts. Tab and Escape keep their meaning.
 */
export function stopStageKeys(event: KeyboardEvent<HTMLElement>) {
  if (event.key !== "Tab" && event.key !== "Escape") event.stopPropagation();
}

export function useStageImages(pendingId: string, sessionId: string, segment: ReviewSegment) {
  const { owner, authenticated } = useQuestionImageOwner(sessionId);
  const refs = useMemo(() => segment.images.map((image) => image.ref), [segment.images]);
  const active = authenticated && Boolean(sessionId) && refs.length > 0;
  const { states, retry } = useQuestionImages(pendingId, segment.questionId, refs, active, owner);
  const [sizes, setSizes] = useState<Record<string, Size>>({});
  const setSize = useCallback(
    (id: string, size: Size) => setSizes((current) => ({ ...current, [id]: size })),
    [],
  );
  return { states, retry, sizes, setSize };
}

export type StageImages = ReturnType<typeof useStageImages>;

export function ImageState({
  state,
  onRetry,
}: {
  state: QuestionImageState | undefined;
  onRetry: () => void;
}) {
  const { t } = useTranslation();
  const error = state?.kind === "error";
  return (
    <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 p-4 text-center">
      <p role="status" className="text-xs text-muted-foreground">
        {t(error ? imageErrorKey(state.code) : "task:questionImageLoading")}
      </p>
      {error && (
        <Button
          type="button"
          variant="outline"
          className={reviewControlSize}
          onPointerDown={(event) => event.stopPropagation()}
          onClick={onRetry}
        >
          {t("task:questionImageRetry")}
        </Button>
      )}
    </div>
  );
}

/**
 * The stage's bottom edge: which image is shown and status on the left, zoom on
 * the right. One row, so the left side wraps upward instead of covering zoom.
 */
export function StageBottomRow({ start, end }: { start?: ReactNode; end?: ReactNode }) {
  return (
    <div className="pointer-events-none absolute inset-x-2 bottom-2 z-10 flex items-end gap-2">
      <div className="flex min-w-0 flex-1 flex-wrap-reverse items-end gap-1 text-xs">{start}</div>
      {end && (
        <div
          className="pointer-events-auto shrink-0"
          onPointerDown={(event) => event.stopPropagation()}
          onDoubleClick={(event) => event.stopPropagation()}
        >
          {end}
        </div>
      )}
    </div>
  );
}
