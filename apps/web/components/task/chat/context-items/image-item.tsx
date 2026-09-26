"use client";

import { memo, useState, useCallback, type ReactNode } from "react";
import { IconFile, IconLoader2, IconPhoto, IconRefresh } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Dialog, DialogContent } from "@kandev/ui/dialog";
import type { ImageContextItem } from "@/lib/types/context";
import {
  IMAGE_PREVIEW_DIALOG_CONTENT_CLASSNAME,
  ImagePreviewContent,
} from "@/components/task/chat/image-preview-dialog";
import { ContextChip } from "./context-chip";

// The image chip intentionally owns the preview, delivery toggle, and retry
// affordance so they stay synchronized on narrow/mobile composers.
// eslint-disable-next-line max-lines-per-function, complexity
export const ImageItem = memo(function ImageItem({ item }: { item: ImageContextItem }) {
  const { t } = useTranslation("chat");
  const [dialogOpen, setDialogOpen] = useState(false);
  const previewSrc = item.attachment.preview;
  const deliveryMode = item.attachment.deliveryMode;
  const uploadStatus = item.attachment.uploadStatus;
  const deliveryDescription =
    deliveryMode === "path"
      ? t("chat:attachmentDeliveryPathDescription")
      : t("chat:attachmentDeliveryPromptDescription");
  let leadingIcon;

  const handleClick = useCallback(() => {
    setDialogOpen(true);
  }, []);

  let uploadStatusRow: ReactNode = null;
  if (uploadStatus === "pending" || uploadStatus === "uploading") {
    uploadStatusRow = (
      <span className="inline-flex items-center gap-1 text-[11px] text-muted-foreground">
        <IconLoader2 className="h-3 w-3 animate-spin" aria-hidden="true" />
        {t("chat:attachmentUploadPending")}
      </span>
    );
  } else if (uploadStatus === "failed") {
    uploadStatusRow = (
      <span className="inline-flex items-center gap-1 text-[11px] text-destructive">
        {t("chat:attachmentUploadFailed")}
        {item.onRetry && (
          <Button
            type="button"
            size="icon"
            variant="ghost"
            className="h-5 w-5"
            aria-label={t("chat:retryAttachmentUpload")}
            onClick={(event) => {
              event.stopPropagation();
              item.onRetry?.();
            }}
          >
            <IconRefresh className="h-3 w-3" aria-hidden="true" />
          </Button>
        )}
      </span>
    );
  }
  const preview =
    previewSrc || uploadStatusRow ? (
      <div className="space-y-1.5">
        {previewSrc && (
          <img
            src={previewSrc}
            alt={t("chat:attachmentPreviewAlt")}
            className="max-w-full max-h-48 rounded object-contain"
          />
        )}
        {uploadStatusRow}
        {item.onDeliveryModeChange && (
          <div className="space-y-1.5">
            <div
              className="flex items-center gap-1"
              role="group"
              aria-label={t("chat:attachmentDeliveryMode")}
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
                {t("chat:attachmentDeliveryPrompt")}
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
                {t("chat:attachmentDeliveryFile")}
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
            <ImagePreviewContent src={previewSrc} alt={t("chat:attachmentPreviewFullAlt")} />
          </DialogContent>
        </Dialog>
      )}
    </>
  );
});
