"use client";

import { memo, useState, useCallback } from "react";
import { Button } from "@kandev/ui/button";
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
      {item.onDeliveryModeChange && (
        <div className="flex items-center gap-1" role="group" aria-label="Attachment delivery mode">
          <Button
            type="button"
            size="sm"
            variant={item.attachment.deliveryMode === "prompt" ? "default" : "outline"}
            className="h-6 px-2 text-xs"
            data-testid="attachment-delivery-prompt"
            data-selected={item.attachment.deliveryMode === "prompt" ? "true" : "false"}
            aria-pressed={item.attachment.deliveryMode === "prompt"}
            onClick={(event) => {
              event.stopPropagation();
              item.onDeliveryModeChange?.("prompt");
            }}
          >
            Prompt
          </Button>
          <Button
            type="button"
            size="sm"
            variant={item.attachment.deliveryMode === "path" ? "default" : "outline"}
            className="h-6 px-2 text-xs"
            data-testid="attachment-delivery-path"
            data-selected={item.attachment.deliveryMode === "path" ? "true" : "false"}
            aria-pressed={item.attachment.deliveryMode === "path"}
            onClick={(event) => {
              event.stopPropagation();
              item.onDeliveryModeChange?.("path");
            }}
          >
            File
          </Button>
        </div>
      )}
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
