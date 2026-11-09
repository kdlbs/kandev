"use client";

import { useState, type ReactNode } from "react";
import { IconArrowsExchange, IconChevronDown, IconDots } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle } from "@kandev/ui/drawer";
import { useTranslation } from "react-i18next";
import { COMPARE_MODES, type CompareMode } from "@/lib/question-review/compare";
import { imageShortName, type ReviewImage } from "@/lib/question-review/segments";
import { formatZoom } from "@/lib/question-review/viewport";
import { cn } from "@/lib/utils";
import { useCompareModeLabel } from "./review-compare-toolbar";
import { useReviewImageLabel } from "./review-image-picker";

const touchButton = "min-h-11 cursor-pointer";

function Sheet({
  title,
  open,
  onOpenChange,
  testId,
  children,
}: {
  title: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  testId: string;
  children: ReactNode;
}) {
  return (
    <Drawer open={open} onOpenChange={onOpenChange}>
      <DrawerContent
        className="z-[60] max-h-[82dvh] pb-[calc(1rem+env(safe-area-inset-bottom))]"
        data-testid={testId}
      >
        <DrawerHeader className="text-left">
          <DrawerTitle>{title}</DrawerTitle>
        </DrawerHeader>
        <div className="min-h-0 space-y-2 overflow-y-auto px-4">{children}</div>
      </DrawerContent>
    </Drawer>
  );
}

const MODE_HINT_KEYS: Record<CompareMode, string> = {
  toggle: "task:questionReviewModeToggleHint",
  wipe: "task:questionReviewModeWipeHint",
  side: "task:questionReviewModeSideHint",
  onion: "task:questionReviewModeOnionHint",
  diff: "task:questionReviewModeDiffHint",
};

export function ModeSheetButton({
  mode,
  onChange,
}: {
  mode: CompareMode;
  onChange: (mode: CompareMode) => void;
}) {
  const { t } = useTranslation();
  const label = useCompareModeLabel();
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button
        type="button"
        variant="outline"
        className={touchButton}
        data-testid="question-review-mode-button"
        aria-haspopup="dialog"
        onClick={() => setOpen(true)}
      >
        {t("task:questionReviewModeButton", { mode: label(mode) })}
        <IconChevronDown className="h-4 w-4" aria-hidden="true" />
      </Button>
      <Sheet
        title={t("task:questionReviewCompareModes")}
        open={open}
        onOpenChange={setOpen}
        testId="question-review-mode-sheet"
      >
        {COMPARE_MODES.map((option) => (
          <button
            key={option}
            type="button"
            aria-pressed={option === mode}
            className={cn(
              "flex min-h-12 w-full cursor-pointer flex-col items-start rounded-md border px-3 py-2 text-left",
              option === mode ? "border-primary bg-primary/10" : "border-border",
            )}
            onClick={() => {
              onChange(option);
              setOpen(false);
            }}
          >
            <span className="text-sm font-medium">{label(option)}</span>
            <span className="text-xs text-muted-foreground">{t(MODE_HINT_KEYS[option])}</span>
          </button>
        ))}
      </Sheet>
    </>
  );
}

/** Rarely used stage actions on phones: zoom steps, Fit, Actual size, Mark area. */
export function MoreSheetButton({ children }: { children: (close: () => void) => ReactNode }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button
        type="button"
        variant="outline"
        size="icon"
        className="h-11 w-11 cursor-pointer"
        aria-label={t("task:questionReviewMore")}
        aria-haspopup="dialog"
        data-testid="question-review-more"
        onClick={() => setOpen(true)}
      >
        <IconDots className="h-4 w-4" aria-hidden="true" />
      </Button>
      <Sheet
        title={t("task:questionReviewMore")}
        open={open}
        onOpenChange={setOpen}
        testId="question-review-more-sheet"
      >
        <div className="flex flex-wrap items-center gap-2 pb-2">
          {children(() => setOpen(false))}
        </div>
      </Sheet>
    </>
  );
}

type PickerProps = {
  images: readonly ReviewImage[];
  urls: Record<string, string | undefined>;
  aId: string;
  bId: string;
  onPick: (slot: "a" | "b", id: string) => void;
  onSwap: () => void;
};

/** Pair button for the bottom bar; opens large tiles for choosing A and B. */
export function ImagePickerSheetButton(props: PickerProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const name = (id: string) => {
    const index = props.images.findIndex((image) => image.id === id);
    return index >= 0 ? imageShortName(props.images[index], index) : "";
  };
  return (
    <>
      <Button
        type="button"
        variant="outline"
        className={cn(touchButton, "max-w-40 min-w-0")}
        aria-haspopup="dialog"
        data-testid="question-review-picker-button"
        onClick={() => setOpen(true)}
      >
        <span className="min-w-0 truncate">
          {t("task:questionReviewPairLabel", { a: name(props.aId), b: name(props.bId) })}
        </span>
        <IconChevronDown className="h-4 w-4 shrink-0" aria-hidden="true" />
      </Button>
      <Sheet
        title={t("task:questionReviewChooseImages")}
        open={open}
        onOpenChange={setOpen}
        testId="question-review-picker-sheet"
      >
        <PickerTiles {...props} slot="a" />
        <PickerTiles {...props} slot="b" />
        <Button
          type="button"
          variant="outline"
          className={cn(touchButton, "w-full")}
          onClick={props.onSwap}
        >
          <IconArrowsExchange className="h-4 w-4" aria-hidden="true" />
          {t("task:questionReviewSwap")}
        </Button>
      </Sheet>
    </>
  );
}

function PickerTiles(props: PickerProps & { slot: "a" | "b" }) {
  const { t } = useTranslation();
  const imageLabel = useReviewImageLabel();
  const selected = props.slot === "a" ? props.aId : props.bId;
  return (
    <fieldset className="space-y-1.5">
      <legend className="text-xs font-medium">
        {t(props.slot === "a" ? "task:questionReviewPickA" : "task:questionReviewPickB")}
      </legend>
      <div className="grid grid-cols-2 gap-2">
        {props.images.map((image, index) => (
          <button
            key={image.id}
            type="button"
            aria-pressed={image.id === selected}
            aria-label={imageLabel(image, index)}
            data-testid={`question-review-tile-${props.slot}`}
            className={cn(
              "flex min-h-12 cursor-pointer flex-col items-start gap-1 rounded-md border p-1.5 text-left",
              image.id === selected ? "border-primary bg-primary/10" : "border-border",
            )}
            onClick={() => props.onPick(props.slot, image.id)}
          >
            {props.urls[image.id] && (
              <img
                src={props.urls[image.id]}
                alt=""
                className="h-16 w-full rounded-sm object-contain"
                draggable={false}
              />
            )}
            <span className="w-full truncate text-xs font-medium">
              {imageShortName(image, index)}
            </span>
          </button>
        ))}
      </div>
    </fieldset>
  );
}

/** Zoom level on the image; a tap switches between Fit and 100%. */
export function ZoomChip({ scale, onToggle }: { scale: number; onToggle: () => void }) {
  const { t } = useTranslation();
  return (
    <button
      type="button"
      className="pointer-events-auto absolute right-2 bottom-2 z-10 flex min-h-11 min-w-11 cursor-pointer items-center justify-center rounded-full border border-border bg-background/90 px-3 text-xs tabular-nums shadow-sm"
      aria-label={t("task:questionReviewZoomChip", { level: formatZoom(scale) })}
      data-testid="question-review-zoom-chip"
      onPointerDown={(event) => event.stopPropagation()}
      onDoubleClick={(event) => event.stopPropagation()}
      onClick={onToggle}
    >
      {formatZoom(scale)}
    </button>
  );
}

/** Status text over the image on phones, instead of a toolbar row. */
export function StatusChips({ children }: { children: ReactNode }) {
  return (
    <div
      className="pointer-events-none absolute bottom-2 left-2 z-10 flex max-w-[calc(100%-4.5rem)] flex-wrap gap-1 [&>*]:rounded-full [&>*]:bg-background/90 [&>*]:px-2 [&>*]:py-1 [&>*]:shadow-sm"
      data-testid="question-review-status-chips"
    >
      {children}
    </div>
  );
}
