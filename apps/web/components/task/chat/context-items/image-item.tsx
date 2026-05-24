"use client";

import { memo, useState, useCallback } from "react";
import { Dialog, DialogContent } from "@kandev/ui/dialog";
import type { ImageContextItem } from "@/lib/types/context";
import {
  IMAGE_PREVIEW_DIALOG_CONTENT_CLASSNAME,
  ImagePreviewContent,
} from "@/components/task/chat/image-preview-dialog";
import { ContextChip } from "./context-chip";

export const ImageItem = memo(function ImageItem({ item }: { item: ImageContextItem }) {
  const [dialogOpen, setDialogOpen] = useState(false);
  const previewSrc = item.attachment.preview;

  const handleClick = useCallback(() => {
    setDialogOpen(true);
  }, []);

  const preview = previewSrc ? (
    <div className="space-y-1.5">
      {/* eslint-disable-next-line @next/next/no-img-element -- base64 preview */}
      <img src={previewSrc} alt="Preview" className="max-w-full max-h-48 rounded object-contain" />
    </div>
  ) : undefined;

  return (
    <>
      <ContextChip
        kind="image"
        label={item.label}
        thumbnail={previewSrc}
        preview={preview}
        onClick={previewSrc ? handleClick : undefined}
        onRemove={item.onRemove}
      />
      {previewSrc && (
        <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
          <DialogContent
            aria-describedby={undefined}
            className={IMAGE_PREVIEW_DIALOG_CONTENT_CLASSNAME}
          >
            <ImagePreviewContent src={previewSrc} alt="Full size preview" />
          </DialogContent>
        </Dialog>
      )}
    </>
  );
});
