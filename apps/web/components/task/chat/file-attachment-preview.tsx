"use client";

import { memo } from "react";
import { IconFile, IconX } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { formatBytes, type FileAttachment } from "./file-attachment";

type FileAttachmentPreviewProps = {
  attachments: FileAttachment[];
  onRemove: (id: string) => void;
  disabled?: boolean;
};

export const FileAttachmentPreview = memo(function FileAttachmentPreview({
  attachments,
  onRemove,
  disabled = false,
}: FileAttachmentPreviewProps) {
  if (attachments.length === 0) return null;

  return (
    <div className="flex gap-1.5 px-3 pt-2 flex-wrap">
      {attachments.map((attachment) => (
        <div
          key={attachment.id}
          className={cn(
            "relative group rounded-md overflow-hidden border border-border bg-muted/30",
            disabled && "opacity-50",
          )}
        >
          {attachment.isImage && attachment.preview ? (
            /* eslint-disable-next-line @next/next/no-img-element -- base64/blob preview URLs are not compatible with next/image */
            <img
              src={attachment.preview}
              alt="Attachment preview"
              className="h-10 w-10 object-cover"
            />
          ) : (
            <div className="h-10 flex items-center gap-1 px-2">
              <IconFile className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
              <div className="flex flex-col min-w-0">
                <span className="text-[10px] leading-tight text-foreground truncate max-w-24">
                  {attachment.fileName}
                </span>
                <span className="text-[9px] leading-tight text-muted-foreground">
                  {formatBytes(attachment.size)}
                </span>
              </div>
            </div>
          )}

          {!disabled && (
            <button
              type="button"
              onClick={() => onRemove(attachment.id)}
              className={cn(
                "absolute top-0.5 right-0.5 p-0.5 rounded-full",
                "bg-black/70 text-white",
                "opacity-0 group-hover:opacity-100 transition-opacity",
                "hover:bg-black/90 cursor-pointer",
                "focus:outline-none focus:ring-1 focus:ring-white/50",
              )}
              aria-label={`Remove ${attachment.isImage ? "image" : "file"}`}
            >
              <IconX className="h-2.5 w-2.5" />
            </button>
          )}
        </div>
      ))}
    </div>
  );
});
