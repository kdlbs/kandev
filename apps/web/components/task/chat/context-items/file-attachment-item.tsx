"use client";

import { memo, type ReactNode } from "react";
import { IconLoader2, IconRefresh } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type { FileAttachmentContextItem } from "@/lib/types/context";
import { formatBytes } from "@/lib/utils/format-bytes";
import { ContextChip } from "./context-chip";

export const FileAttachmentItem = memo(function FileAttachmentItem({
  item,
}: {
  item: FileAttachmentContextItem;
}) {
  const { t } = useTranslation("chat");
  const status = item.attachment.uploadStatus;
  let statusIndicator: ReactNode = null;
  if (status === "pending" || status === "uploading") {
    statusIndicator = (
      <span className="inline-flex items-center gap-1" data-testid="attachment-uploading">
        <IconLoader2 className="h-3 w-3 animate-spin" aria-hidden="true" />
        {t("chat:attachmentUploadPending")}
      </span>
    );
  } else if (status === "failed") {
    statusIndicator = (
      <span
        className="inline-flex items-center gap-1 text-destructive"
        data-testid="attachment-upload-failed"
      >
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

  const preview = (
    <div className="flex items-center gap-2 text-xs text-muted-foreground">
      <span>
        {item.attachment.fileName} ({formatBytes(item.attachment.size)})
      </span>
      {statusIndicator}
    </div>
  );

  return <ContextChip kind="file" label={item.label} preview={preview} onRemove={item.onRemove} />;
});
