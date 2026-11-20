"use client";

import { useState } from "react";
import {
  IconArrowsExchange,
  IconChevronDown,
  IconFocusCentered,
  IconHelp,
  IconMinus,
  IconPlus,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { useTranslation } from "react-i18next";
import { imageShortName, type ReviewImage } from "@/lib/question-review/segments";
import { formatZoom } from "@/lib/question-review/viewport";
import { cn } from "@/lib/utils";
import { CandidateStepper } from "./review-compare-controls";
import { ReviewImageOptions } from "./review-image-option";
import { useReviewImageLabel } from "./review-image-picker";
import type { ChangesStatus } from "./use-changes-zoom";

const ZOOM_LEVELS = [1, 2, 4, 8] as const;
// i18n-exempt: comparison slot identifiers, written the same in every locale
const SLOT_LETTER = { a: "A", b: "B" } as const;
const clusterButton =
  "h-8 w-8 cursor-pointer rounded-full max-md:h-11 max-md:w-11 [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11";

type PairProps = {
  images: readonly ReviewImage[];
  aId: string;
  bId: string;
  onPick: (slot: "a" | "b", id: string) => void;
  onSwap: () => void;
  urls: Record<string, string | undefined>;
  stepper?: { position: { index: number; total: number }; onStep: (direction: 1 | -1) => void };
};

/** A and B as one control: pick either, swap them, and step B through candidates. */
export function ImagePairControl(props: PairProps) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-1" data-testid="question-review-pair">
      <ImageSlotPicker {...props} slot="a" />
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="h-7 w-7 cursor-pointer"
        aria-label={t("task:questionReviewSwap")}
        title={t("task:questionReviewSwap")}
        data-testid="question-review-swap"
        onClick={props.onSwap}
      >
        <IconArrowsExchange className="h-4 w-4" aria-hidden="true" />
      </Button>
      <ImageSlotPicker {...props} slot="b" />
      {props.stepper && props.stepper.position.total > 1 && (
        <CandidateStepper
          images={props.images}
          bId={props.bId}
          position={props.stepper.position}
          onStep={props.stepper.onStep}
          compact
          countOnly
        />
      )}
    </div>
  );
}

function ImageSlotPicker(props: PairProps & { slot: "a" | "b" }) {
  const { t } = useTranslation();
  const imageLabel = useReviewImageLabel();
  const [open, setOpen] = useState(false);
  const selectedId = props.slot === "a" ? props.aId : props.bId;
  const index = props.images.findIndex((image) => image.id === selectedId);
  const slotName = t(props.slot === "a" ? "task:questionReviewPickA" : "task:questionReviewPickB");
  const selected = index >= 0 ? props.images[index] : undefined;
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          className="h-7 max-w-40 cursor-pointer gap-1 px-2 text-xs"
          aria-label={
            selected
              ? t("task:questionReviewSlotImage", {
                  slot: slotName,
                  image: imageLabel(selected, index),
                })
              : slotName
          }
          data-testid={`question-review-pick-${props.slot}`}
        >
          <span className="text-muted-foreground">{SLOT_LETTER[props.slot]}</span>
          <span className="min-w-0 truncate">
            {selected ? imageShortName(selected, index) : ""}
          </span>
          <IconChevronDown className="h-3 w-3 shrink-0" aria-hidden="true" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-[22.5rem] p-1 data-closed:animate-none!" align="start">
        <ReviewImageOptions
          slot={props.slot}
          images={props.images}
          aId={props.aId}
          bId={props.bId}
          urls={props.urls}
          onPick={(slot, id) => {
            props.onPick(slot, id);
            setOpen(false);
          }}
        />
      </PopoverContent>
    </Popover>
  );
}

type ClusterProps = {
  scale: number;
  onZoom: (direction: 1 | -1) => void;
  onFit: () => void;
  onScale: (scale: number) => void;
  changes?: { status: ChangesStatus; onClick: () => void };
};

/** Zoom controls on the image, like a map viewer. */
export function ZoomCluster({ scale, onZoom, onFit, onScale, changes }: ClusterProps) {
  const { t } = useTranslation();
  return (
    <div
      className="pointer-events-auto absolute right-2 bottom-2 z-10 flex items-center gap-0.5 rounded-full border border-border bg-background/90 p-0.5 shadow-sm"
      data-testid="question-review-zoom"
      onPointerDown={(event) => event.stopPropagation()}
      onDoubleClick={(event) => event.stopPropagation()}
    >
      {changes && <ChangesButton {...changes} />}
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className={clusterButton}
        aria-label={t("task:questionReviewZoomOut")}
        data-testid="question-review-zoom-out"
        onClick={() => onZoom(-1)}
      >
        <IconMinus className="h-4 w-4" aria-hidden="true" />
      </Button>
      <ZoomLevelMenu scale={scale} onFit={onFit} onScale={onScale} />
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className={clusterButton}
        aria-label={t("task:questionReviewZoomIn")}
        data-testid="question-review-zoom-in"
        onClick={() => onZoom(1)}
      >
        <IconPlus className="h-4 w-4" aria-hidden="true" />
      </Button>
      <ShortcutHelp />
    </div>
  );
}

function ZoomLevelMenu({
  scale,
  onFit,
  onScale,
}: {
  scale: number;
  onFit: () => void;
  onScale: (scale: number) => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const choose = (action: () => void) => {
    action();
    setOpen(false);
  };
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          className="h-8 min-w-14 cursor-pointer gap-0.5 rounded-full px-2 text-xs tabular-nums"
          aria-label={t("task:questionReviewZoomStatus", { level: formatZoom(scale) })}
          data-testid="question-review-zoom-level"
        >
          {formatZoom(scale)}
          <IconChevronDown className="h-3 w-3" aria-hidden="true" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-32 p-1 data-closed:animate-none!" side="top" align="center">
        <MenuItem testId="question-review-fit" onClick={() => choose(onFit)}>
          {t("task:questionImageFit")}
        </MenuItem>
        {ZOOM_LEVELS.map((level) => (
          <MenuItem
            key={level}
            testId={level === 1 ? "question-review-actual" : `question-review-zoom-${level * 100}`}
            onClick={() => choose(() => onScale(level))}
          >
            {formatZoom(level)}
          </MenuItem>
        ))}
      </PopoverContent>
    </Popover>
  );
}

function MenuItem({
  testId,
  onClick,
  children,
}: {
  testId: string;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      className="flex w-full cursor-pointer rounded-sm px-2 py-1.5 text-left text-xs tabular-nums hover:bg-muted"
      data-testid={testId}
      onClick={onClick}
    >
      {children}
    </button>
  );
}

const CHANGES_LABEL_KEYS: Record<ChangesStatus, string> = {
  idle: "task:questionReviewZoomToChanges",
  computing: "task:questionReviewFindingChanges",
  none: "task:questionReviewNoChanges",
  unavailable: "task:questionReviewDiffUnavailable",
};

export function ChangesButton({ status, onClick }: { status: ChangesStatus; onClick: () => void }) {
  const { t } = useTranslation();
  const label = t(CHANGES_LABEL_KEYS[status]);
  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      className={cn(clusterButton, status === "computing" && "animate-pulse")}
      aria-label={label}
      title={label}
      disabled={status !== "idle"}
      data-testid="question-review-zoom-changes"
      data-status={status}
      onClick={onClick}
    >
      <IconFocusCentered className="h-4 w-4" aria-hidden="true" />
    </Button>
  );
}

function ShortcutHelp() {
  const { t } = useTranslation();
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className={clusterButton}
          aria-label={t("task:questionReviewShortcuts")}
          data-testid="question-review-help"
        >
          <IconHelp className="h-4 w-4" aria-hidden="true" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        className="w-80 text-xs leading-relaxed data-closed:animate-none!"
        side="top"
        align="end"
      >
        {t("task:questionReviewStageHelp")}
      </PopoverContent>
    </Popover>
  );
}

/** Keeps clicks on a control over the image from panning, drawing, or zooming it. */
export function StageOverlayControl({ children }: { children: React.ReactNode }) {
  return (
    <div
      className="pointer-events-auto absolute top-2 right-2 z-10"
      onPointerDown={(event) => event.stopPropagation()}
      onDoubleClick={(event) => event.stopPropagation()}
    >
      {children}
    </div>
  );
}
