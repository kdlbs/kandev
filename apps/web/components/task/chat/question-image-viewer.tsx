"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { IconChevronLeft, IconChevronRight } from "@tabler/icons-react";
import { Dialog, DialogClose, DialogContent, DialogTitle, DialogTrigger } from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useClarificationEscapeGuard } from "@/hooks/use-clarification-escape-guard";
import { swipeDirection } from "@/lib/question-review/viewport";
import { cn } from "@/lib/utils";
import {
  useGalleryRegistration,
  useQuestionImageGallery,
  type GalleryItem,
} from "./question-image-gallery";

const imageOutline = "rounded-md outline -outline-offset-1 outline-black/10 dark:outline-white/10";
const controlSize =
  "h-7 [@media(pointer:coarse)]:h-12 [@media(pointer:coarse)]:min-w-12 max-md:h-12 max-md:min-w-12";

type Props = {
  src: string;
  alt: string;
  /** Joins the question's gallery, so the inspector can step to sibling images. */
  imageId?: string;
  onInspectionChange?: (open: boolean) => void;
};

/** The inspector's current image within the gallery, falling back to this thumbnail. */
function useGalleryPosition(imageId: string | undefined, src: string, alt: string) {
  const gallery = useQuestionImageGallery();
  const items = gallery?.items ?? [];
  const [currentId, setCurrentId] = useState(imageId);
  const index = items.findIndex((item) => item.id === currentId);
  const current: Pick<GalleryItem, "url" | "alt"> = index >= 0 ? items[index] : { url: src, alt };
  return { items, index, current, currentId, setCurrentId };
}

export function QuestionImageViewer({ src, alt, imageId, onInspectionChange }: Props) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const triggerRef = useRef<HTMLButtonElement>(null);
  useGalleryRegistration(imageId, src, alt, triggerRef);
  const position = useGalleryPosition(imageId, src, alt);
  const [open, setOpen] = useState(false);
  const [actualSize, setActualSize] = useState(false);
  const guard = useCallback(() => open, [open]);
  useClarificationEscapeGuard(open ? guard : null);
  useEffect(() => {
    if (!open) return;
    onInspectionChange?.(true);
    return () => onInspectionChange?.(false);
  }, [open, onInspectionChange]);
  const gallery = position.items.length > 1 && position.index >= 0;
  const go = (direction: 1 | -1) => {
    const next = position.items[position.index + direction];
    if (!next) return;
    position.setCurrentId(next.id);
    setActualSize(false);
  };
  return (
    <Dialog
      open={open}
      onOpenChange={(value) => {
        setOpen(value);
        if (value) {
          setActualSize(false);
          position.setCurrentId(imageId);
        }
      }}
    >
      <DialogTrigger asChild>
        <button
          ref={triggerRef}
          type="button"
          aria-label={t("task:openImage", { alt })}
          className="flex max-w-full cursor-pointer flex-col items-start gap-1 rounded-md focus-visible:outline-2 focus-visible:outline-ring max-md:min-h-12 max-md:min-w-12 [@media(pointer:coarse)]:min-h-12 [@media(pointer:coarse)]:min-w-12"
        >
          <img
            src={src}
            alt={alt}
            className={cn("max-h-32 max-w-full object-contain max-md:max-h-24", imageOutline)}
          />
          <span className="text-xs text-muted-foreground">{t("task:questionImageInspect")}</span>
        </button>
      </DialogTrigger>
      <DialogContent
        showCloseButton={false}
        enterConfirms={false}
        aria-describedby={undefined}
        onEscapeKeyDown={(event) => event.stopPropagation()}
        onKeyDown={(event) => {
          event.stopPropagation();
          const step = { ArrowLeft: -1, ArrowRight: 1 }[event.key] as 1 | -1 | undefined;
          // At Actual size, arrows on the focused image scroll it instead.
          const scrolling =
            actualSize && event.target instanceof HTMLElement && event.target.dataset.canvas;
          if (step && !scrolling) {
            event.preventDefault();
            go(step);
          }
        }}
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          (position.items[position.index]?.trigger.current ?? triggerRef.current)?.focus();
        }}
        overlayClassName="data-open:animate-none data-closed:animate-none"
        className={cn(
          "flex flex-col gap-3 overflow-hidden data-open:animate-none data-closed:animate-none",
          isMobile
            ? "h-dvh w-screen max-w-none rounded-none border-0 sm:max-w-none pt-[max(1rem,env(safe-area-inset-top))] pb-[max(1rem,env(safe-area-inset-bottom))]"
            : "h-[min(85dvh,900px)] w-[min(92vw,1200px)] max-w-none sm:max-w-none",
        )}
      >
        <div className="flex shrink-0 items-start justify-between gap-3">
          <DialogTitle className="min-w-0 break-words text-sm">{position.current.alt}</DialogTitle>
          <DialogClose asChild>
            <Button type="button" variant="outline" className={controlSize}>
              {t("common:close")}
            </Button>
          </DialogClose>
        </div>
        <div className="relative flex min-h-0 flex-1 flex-col">
          <ImageCanvas
            src={position.current.url}
            alt={position.current.alt}
            actualSize={actualSize}
            onSwipe={actualSize ? undefined : go}
          />
          {gallery && <GalleryHandles items={position.items} index={position.index} onStep={go} />}
        </div>
        <div className="flex shrink-0 flex-wrap items-center gap-2">
          <SizeButtons actualSize={actualSize} onChange={setActualSize} />
          {gallery && <GalleryCounter items={position.items} index={position.index} />}
        </div>
      </DialogContent>
    </Dialog>
  );
}

function SizeButtons({
  actualSize,
  onChange,
}: {
  actualSize: boolean;
  onChange: (actual: boolean) => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <Button
        type="button"
        variant={actualSize ? "outline" : "secondary"}
        className={controlSize}
        aria-pressed={!actualSize}
        onClick={() => onChange(false)}
      >
        {t("task:questionImageFit")}
      </Button>
      <Button
        type="button"
        variant={actualSize ? "secondary" : "outline"}
        className={controlSize}
        aria-pressed={actualSize}
        onClick={() => onChange(true)}
      >
        {t("task:questionImageActualSize")}
      </Button>
    </>
  );
}

const handleClass =
  "absolute top-1/2 z-10 flex h-16 w-11 -translate-y-1/2 cursor-pointer items-center justify-center rounded-full border border-border/60 bg-background/70 text-foreground shadow-sm backdrop-blur-sm transition-opacity opacity-70 hover:opacity-100 focus-visible:opacity-100 focus-visible:outline-2 focus-visible:outline-ring disabled:pointer-events-none disabled:opacity-0 max-md:h-12 max-md:w-12";

/** Large Previous and Next handles over the image edges. */
function GalleryHandles({
  items,
  index,
  onStep,
}: {
  items: readonly GalleryItem[];
  index: number;
  onStep: (direction: 1 | -1) => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <button
        type="button"
        className={cn(handleClass, "left-2")}
        disabled={index === 0}
        aria-label={t("task:questionImagePrevious")}
        data-testid="question-image-previous"
        onClick={() => onStep(-1)}
      >
        <IconChevronLeft className="h-6 w-6" aria-hidden="true" />
      </button>
      <button
        type="button"
        className={cn(handleClass, "right-2")}
        disabled={index === items.length - 1}
        aria-label={t("task:questionImageNext")}
        data-testid="question-image-next"
        onClick={() => onStep(1)}
      >
        <IconChevronRight className="h-6 w-6" aria-hidden="true" />
      </button>
    </>
  );
}

function GalleryCounter({ items, index }: { items: readonly GalleryItem[]; index: number }) {
  const { t } = useTranslation();
  const item = items[index];
  const counter = item.hasLabel
    ? t("task:questionImageCounterNamed", {
        name: item.name,
        index: index + 1,
        total: items.length,
      })
    : t("task:questionImageCounter", { index: index + 1, total: items.length });
  return (
    <span
      className="ml-auto text-xs tabular-nums"
      aria-live="polite"
      data-testid="question-image-counter"
    >
      {counter}
    </span>
  );
}

function ImageCanvas({
  src,
  alt,
  actualSize,
  onSwipe,
}: {
  src: string;
  alt: string;
  actualSize: boolean;
  onSwipe?: (direction: 1 | -1) => void;
}) {
  const { t } = useTranslation();
  const start = useRef<{ id: number; x: number; y: number } | null>(null);
  return (
    <div
      className="min-h-0 flex-1 overflow-auto overscroll-contain rounded-md bg-muted/40"
      tabIndex={0}
      role="region"
      data-canvas="true"
      aria-label={t("task:questionImageCanvas")}
      data-testid="question-image-canvas"
      onPointerDown={(event) => {
        start.current = onSwipe
          ? { id: event.pointerId, x: event.clientX, y: event.clientY }
          : null;
      }}
      onPointerUp={(event) => {
        const origin = start.current;
        start.current = null;
        if (!origin || origin.id !== event.pointerId || !onSwipe) return;
        const direction = swipeDirection(event.clientX - origin.x, event.clientY - origin.y);
        if (direction !== 0) onSwipe(direction);
      }}
    >
      <div
        className={cn(
          "flex min-h-full min-w-full",
          !actualSize && "h-full items-center justify-center",
        )}
      >
        <img
          src={src}
          alt={alt}
          draggable={false}
          className={cn(
            imageOutline,
            actualSize
              ? "h-auto max-w-none self-start shrink-0"
              : "max-h-full max-w-full object-contain",
          )}
        />
      </div>
    </div>
  );
}
