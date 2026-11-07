"use client";

import { useId, type KeyboardEvent, type ReactNode, type RefObject } from "react";
import { useTranslation } from "react-i18next";
import type { ReviewView } from "@/lib/question-review/viewport";
import { cn } from "@/lib/utils";

type Props = {
  stageRef?: RefObject<HTMLDivElement | null>;
  view: ReviewView;
  onStageKey: (event: { key: string; shiftKey: boolean; altKey: boolean }) => boolean;
  pointerHandlers: React.HTMLAttributes<HTMLDivElement>;
  children: ReactNode;
  className?: string;
  overlay?: ReactNode;
  testId?: string;
  label?: string;
};

/**
 * Focusable stage canvas. Every key pressed here stays here: the answer form's
 * window-level shortcuts never see it, so stage keys cannot pick an option,
 * change the question, or submit. Tab and Escape keep their normal meaning.
 */
export function ReviewCanvas(props: Props) {
  const { t } = useTranslation();
  const helpId = useId();
  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "Tab" || event.key === "Escape") return;
    event.stopPropagation();
    if (event.metaKey || event.ctrlKey) return;
    if (props.onStageKey(event)) event.preventDefault();
  };
  return (
    <div
      ref={props.stageRef}
      tabIndex={0}
      role="region"
      aria-label={props.label ?? t("task:questionReviewStage")}
      aria-describedby={helpId}
      data-testid={props.testId ?? "question-review-stage"}
      data-review-pane=""
      onKeyDown={onKeyDown}
      {...props.pointerHandlers}
      className={cn(
        "relative min-h-0 flex-1 cursor-grab touch-none select-none overflow-hidden overscroll-contain rounded-md bg-muted/40 focus-visible:outline-2 focus-visible:outline-ring active:cursor-grabbing",
        props.className,
      )}
    >
      <p id={helpId} className="sr-only">
        {t("task:questionReviewStageHelp")}
      </p>
      <ReviewLayer view={props.view}>{props.children}</ReviewLayer>
      {props.overlay}
    </div>
  );
}

/** One transformed layer in image coordinates. Layers that share a view stay aligned. */
export function ReviewLayer({ view, children }: { view: ReviewView; children: ReactNode }) {
  return (
    <div
      className="absolute top-0 left-0 origin-top-left"
      style={{ transform: `scale(${view.scale}) translate(${-view.x}px, ${-view.y}px)` }}
    >
      {children}
    </div>
  );
}

type ImageProps = {
  src: string;
  alt: string;
  scale: number;
  onSize: (size: { width: number; height: number }) => void;
  className?: string;
  style?: React.CSSProperties;
};

/** A stage image at its natural pixel size. Above 100% pixels stay sharp. */
export function ReviewImage({ src, alt, scale, onSize, className, style }: ImageProps) {
  return (
    <img
      src={src}
      alt={alt}
      draggable={false}
      data-testid="question-review-image"
      onLoad={(event) =>
        onSize({
          width: event.currentTarget.naturalWidth,
          height: event.currentTarget.naturalHeight,
        })
      }
      className={cn(
        "block max-w-none outline -outline-offset-1 outline-black/10 dark:outline-white/10",
        className,
      )}
      style={{ imageRendering: scale > 1 ? "pixelated" : "auto", ...style }}
    />
  );
}
