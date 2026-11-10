"use client";

import { useCallback, useEffect, useState } from "react";
import { Dialog, DialogClose, DialogContent, DialogTitle, DialogTrigger } from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useClarificationEscapeGuard } from "@/hooks/use-clarification-escape-guard";
import { cn } from "@/lib/utils";

const imageOutline = "rounded-md outline -outline-offset-1 outline-black/10 dark:outline-white/10";
const controlSize = "h-7 [@media(pointer:coarse)]:h-12 max-md:h-12";

type Props = { src: string; alt: string; onInspectionChange?: (open: boolean) => void };

export function QuestionImageViewer({ src, alt, onInspectionChange }: Props) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const [open, setOpen] = useState(false);
  const [actualSize, setActualSize] = useState(false);
  const guard = useCallback(() => open, [open]);
  useClarificationEscapeGuard(open ? guard : null);
  useEffect(() => {
    if (!open) return;
    onInspectionChange?.(true);
    return () => onInspectionChange?.(false);
  }, [open, onInspectionChange]);
  return (
    <Dialog
      open={open}
      onOpenChange={(value) => {
        setOpen(value);
        if (value) setActualSize(false);
      }}
    >
      <DialogTrigger asChild>
        <button
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
        onKeyDown={(event) => event.stopPropagation()}
        overlayClassName="data-open:animate-none data-closed:animate-none"
        className={cn(
          "flex flex-col gap-3 overflow-hidden data-open:animate-none data-closed:animate-none",
          isMobile
            ? "h-dvh w-screen max-w-none rounded-none border-0 sm:max-w-none pt-[max(1rem,env(safe-area-inset-top))] pb-[max(1rem,env(safe-area-inset-bottom))]"
            : "h-[min(85dvh,900px)] w-[min(92vw,1200px)] max-w-none sm:max-w-none",
        )}
      >
        <div className="flex shrink-0 items-start justify-between gap-3">
          <DialogTitle className="min-w-0 break-words text-sm">{alt}</DialogTitle>
          <DialogClose asChild>
            <Button type="button" variant="outline" className={controlSize}>
              {t("common:close")}
            </Button>
          </DialogClose>
        </div>
        <ImageCanvas src={src} alt={alt} actualSize={actualSize} />
        <div className="flex shrink-0 gap-2">
          <Button
            type="button"
            variant={actualSize ? "outline" : "secondary"}
            className={controlSize}
            aria-pressed={!actualSize}
            onClick={() => setActualSize(false)}
          >
            {t("task:questionImageFit")}
          </Button>
          <Button
            type="button"
            variant={actualSize ? "secondary" : "outline"}
            className={controlSize}
            aria-pressed={actualSize}
            onClick={() => setActualSize(true)}
          >
            {t("task:questionImageActualSize")}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

function ImageCanvas({ src, alt, actualSize }: { src: string; alt: string; actualSize: boolean }) {
  const { t } = useTranslation();
  return (
    <div
      className="min-h-0 flex-1 overflow-auto overscroll-contain rounded-md bg-muted/40"
      tabIndex={0}
      role="region"
      aria-label={t("task:questionImageCanvas")}
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
