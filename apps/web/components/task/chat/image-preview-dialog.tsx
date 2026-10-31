"use client";

import { Dialog, DialogClose, DialogContent, DialogTitle, DialogTrigger } from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { IconX } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import { useRef, useState } from "react";

type ImagePreviewDialogProps = {
  src: string;
  alt: string;
  thumbnailClassName?: string;
  interactive?: boolean;
};

export const IMAGE_PREVIEW_DIALOG_CONTENT_CLASSNAME =
  "flex h-[min(90dvh,900px)] w-[min(96vw,1126px)] max-w-[calc(100vw-1rem)] flex-col gap-2 overflow-hidden p-2 sm:max-w-[calc(100vw-2rem)] sm:p-3 max-md:h-dvh max-md:w-screen max-md:max-w-none max-md:rounded-none max-md:pt-[max(0.5rem,env(safe-area-inset-top))] max-md:pb-[max(0.5rem,env(safe-area-inset-bottom))]";

type ImagePreviewContentProps = {
  src: string;
  alt: string;
};

export function ImagePreviewContent({ src, alt }: ImagePreviewContentProps) {
  return <ImageInspection key={src} src={src} alt={alt} />;
}

function ImageInspection({ src, alt }: ImagePreviewContentProps) {
  const { t } = useTranslation();
  const [actualSize, setActualSize] = useState(false);
  const canvasRef = useRef<HTMLDivElement>(null);
  const changeMode = (actual: boolean) => {
    if (canvasRef.current) {
      canvasRef.current.scrollLeft = 0;
      canvasRef.current.scrollTop = 0;
    }
    setActualSize(actual);
  };
  return (
    <>
      <div className="flex shrink-0 items-center justify-between">
        <DialogTitle className="sr-only">{t("task:imagePreview")}</DialogTitle>
        <span aria-hidden />
        <DialogClose asChild>
          <Button
            variant="ghost"
            size="icon-sm"
            className="max-md:size-12 [@media(pointer:coarse)]:size-12"
            aria-label={t("common:close")}
          >
            <IconX aria-hidden />
          </Button>
        </DialogClose>
      </div>
      <div
        ref={canvasRef}
        role="region"
        aria-label={t("task:questionImageCanvas")}
        tabIndex={0}
        className={cn(
          "min-h-0 flex-1 overscroll-contain rounded-md bg-muted/40",
          actualSize ? "overflow-auto" : "overflow-hidden",
        )}
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
              "rounded",
              actualSize
                ? "h-auto max-w-none shrink-0 self-start"
                : "max-h-full max-w-full object-contain",
            )}
          />
        </div>
      </div>
      <div className="flex shrink-0 gap-2">
        <Button
          type="button"
          variant={actualSize ? "outline" : "secondary"}
          className="h-7 max-md:h-12 max-md:min-w-12 [@media(pointer:coarse)]:h-12 [@media(pointer:coarse)]:min-w-12"
          aria-pressed={!actualSize}
          onClick={() => changeMode(false)}
        >
          {t("task:questionImageFit")}
        </Button>
        <Button
          type="button"
          variant={actualSize ? "secondary" : "outline"}
          className="h-7 max-md:h-12 max-md:min-w-12 [@media(pointer:coarse)]:h-12 [@media(pointer:coarse)]:min-w-12"
          aria-pressed={actualSize}
          onClick={() => changeMode(true)}
        >
          {t("task:questionImageActualSize")}
        </Button>
      </div>
    </>
  );
}

export function ImagePreviewDialog({
  src,
  alt,
  thumbnailClassName,
  interactive = true,
}: ImagePreviewDialogProps) {
  const { t } = useTranslation();
  if (!interactive) {
    return <img src={src} alt={alt} draggable={false} className={thumbnailClassName} />;
  }

  return (
    <Dialog>
      <DialogTrigger asChild>
        <button
          type="button"
          aria-label={t("task:openImage", { alt })}
          className="inline-flex max-w-full cursor-pointer items-center justify-center rounded-md focus:outline-none focus:ring-2 focus:ring-primary max-md:min-h-12 max-md:min-w-12 [@media(pointer:coarse)]:min-h-12 [@media(pointer:coarse)]:min-w-12"
        >
          <img
            src={src}
            alt=""
            draggable={false}
            className={cn("pointer-events-none", thumbnailClassName)}
          />
        </button>
      </DialogTrigger>
      <DialogContent
        aria-describedby={undefined}
        showCloseButton={false}
        className={cn(
          IMAGE_PREVIEW_DIALOG_CONTENT_CLASSNAME,
          "motion-reduce:data-open:zoom-in-100 motion-reduce:data-closed:zoom-out-100",
        )}
      >
        <ImagePreviewContent src={src} alt={t("task:fullSize", { alt })} />
      </DialogContent>
    </Dialog>
  );
}
