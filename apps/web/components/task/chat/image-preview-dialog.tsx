"use client";

import { Dialog, DialogContent, DialogTitle, DialogTrigger } from "@kandev/ui/dialog";
import { cn } from "@/lib/utils";

type ImagePreviewDialogProps = {
  src: string;
  alt: string;
  thumbnailClassName?: string;
  interactive?: boolean;
};

export const IMAGE_PREVIEW_DIALOG_CONTENT_CLASSNAME =
  "flex w-fit max-w-[calc(100vw-1rem)] items-center justify-center overflow-hidden p-2 sm:max-w-[calc(100vw-2rem)] sm:p-3";

const IMAGE_PREVIEW_IMAGE_CLASSNAME =
  "block h-auto max-h-[calc(100dvh-5rem)] w-[min(92vw,1100px)] max-w-full rounded object-contain";

type ImagePreviewContentProps = {
  src: string;
  alt: string;
};

export function ImagePreviewContent({ src, alt }: ImagePreviewContentProps) {
  return (
    <>
      <DialogTitle className="sr-only">Image preview</DialogTitle>
      {/* eslint-disable-next-line @next/next/no-img-element -- base64 preview URL */}
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
  if (!interactive) {
    return (
      // eslint-disable-next-line @next/next/no-img-element -- base64 preview URL
      <img src={src} alt={alt} className={thumbnailClassName} />
    );
  }

  return (
    <Dialog>
      <DialogTrigger asChild>
        <button
          type="button"
          aria-label={`Open ${alt}`}
          className="inline-flex max-w-full cursor-pointer items-center justify-center rounded-md focus:outline-none focus:ring-2 focus:ring-primary"
        >
          {/* eslint-disable-next-line @next/next/no-img-element -- base64 preview URL */}
          <img src={src} alt="" className={cn("pointer-events-none", thumbnailClassName)} />
        </button>
      </DialogTrigger>
      <DialogContent
        aria-describedby={undefined}
        className={IMAGE_PREVIEW_DIALOG_CONTENT_CLASSNAME}
      >
        <ImagePreviewContent src={src} alt={`Full size ${alt}`} />
      </DialogContent>
    </Dialog>
  );
}
