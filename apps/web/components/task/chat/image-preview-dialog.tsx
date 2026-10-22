"use client";

import { Dialog, DialogClose, DialogContent, DialogTitle, DialogTrigger } from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { IconX } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";

type ImagePreviewDialogProps = {
  src: string;
  alt: string;
  thumbnailClassName?: string;
  interactive?: boolean;
};

export const IMAGE_PREVIEW_DIALOG_CONTENT_CLASSNAME =
  "flex min-h-16 w-[min(96vw,1126px)] max-w-[calc(100vw-1rem)] items-center justify-center overflow-hidden p-2 sm:max-w-[calc(100vw-2rem)] sm:p-3";

const IMAGE_PREVIEW_IMAGE_CLASSNAME =
  "block h-auto max-h-[calc(100dvh-5rem)] w-[min(92vw,1100px)] max-w-full rounded object-contain";

type ImagePreviewContentProps = {
  src: string;
  alt: string;
};

export function ImagePreviewContent({ src, alt }: ImagePreviewContentProps) {
  const { t } = useTranslation();
  return (
    <>
      <DialogTitle className="sr-only">{t("task:imagePreview")}</DialogTitle>
      <img src={src} alt={alt} className={IMAGE_PREVIEW_IMAGE_CLASSNAME} />
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
        <DialogClose asChild>
          <Button
            variant="ghost"
            size="icon-sm"
            className="absolute top-2 right-2 bg-popover/90 max-md:size-12 [@media(pointer:coarse)]:size-12"
            aria-label={t("common:close")}
          >
            <IconX aria-hidden />
          </Button>
        </DialogClose>
      </DialogContent>
    </Dialog>
  );
}
