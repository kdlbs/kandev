"use client";

import { memo, useState, useCallback } from "react";
import { IconFile, IconPhoto } from "@tabler/icons-react";
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
  const deliveryMode = item.attachment.deliveryMode;
  const deliveryDescription =
    deliveryMode === "path"
      ? "Upload into the workspace so the agent can read or edit the file."
      : "Send as prompt context for visual understanding. The agent will not get a file path.";
  let leadingIcon;

  const handleClick = useCallback(() => {
    setDialogOpen(true);
  }, []);

  const preview = previewSrc ? (
    <div className="space-y-1.5">
      <img src={previewSrc} alt="Preview" className="max-w-full max-h-48 rounded object-contain" />
      {item.onDeliveryModeChange && (
        <div className="space-y-1.5">
          <div
            className="flex items-center gap-1"
            role="group"
            aria-label="Attachment delivery mode"
          >
            <Button
              type="button"
              size="sm"
              variant={deliveryMode === "prompt" ? "default" : "outline"}
              className="h-6 px-2 text-xs"
              data-testid="attachment-delivery-prompt"
              data-selected={deliveryMode === "prompt" ? "true" : "false"}
              aria-pressed={deliveryMode === "prompt"}
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
              variant={deliveryMode === "path" ? "default" : "outline"}
              className="h-6 px-2 text-xs"
              data-testid="attachment-delivery-path"
              data-selected={deliveryMode === "path" ? "true" : "false"}
              aria-pressed={deliveryMode === "path"}
              onClick={(event) => {
                event.stopPropagation();
                item.onDeliveryModeChange?.("path");
              }}
            >
              File
            </Button>
          </div>
          <p className="text-[11px] leading-snug text-muted-foreground">{deliveryDescription}</p>
        </div>
      )}
    </div>
  ) : undefined;
  if (deliveryMode === "path") {
    leadingIcon = (
      <span className="relative h-3 w-3 shrink-0" aria-hidden="true">
        <IconFile className="h-3 w-3 text-muted-foreground" />
        {previewSrc && (
          <img
            src={previewSrc}
            alt=""
            className="absolute -right-1 -bottom-1 h-2 w-2 rounded-[2px] border border-background object-cover"
          />
        )}
      </span>
    );
  } else if (!previewSrc) {
    leadingIcon = <IconPhoto className="h-3 w-3 shrink-0" />;
  }

  return (
    <>
      <ContextChip
        kind="image"
        label={item.label}
        thumbnail={deliveryMode === "prompt" ? previewSrc : undefined}
        leadingIcon={leadingIcon}
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
