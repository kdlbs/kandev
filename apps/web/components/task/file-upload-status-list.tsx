"use client";

import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import type { UploadItem, UploadStatus } from "@/hooks/use-file-upload";

function uploadStatusLabel(status: UploadStatus, t: TFunction): string {
  switch (status) {
    case "pending":
      return t("common:pending");
    case "blocked":
      return t("task:blocked");
    case "uploading":
      return t("task:loading");
    case "ready":
      return t("task:done");
    case "failed":
      return t("task:failed");
  }
}

export function FileUploadStatusList({ uploads }: { uploads: UploadItem[] }) {
  const { t } = useTranslation();
  if (uploads.length === 0) return null;
  return (
    <div
      className="border-b border-foreground/10 px-3 py-2 text-xs"
      data-testid="files-upload-status"
      aria-live="polite"
    >
      <div className="space-y-1">
        {uploads.map((upload) => (
          <div key={upload.id} className="flex min-w-0 items-start gap-2">
            <span className="min-w-0 flex-1 truncate" title={upload.destinationPath}>
              {upload.destinationPath}
            </span>
            <span className="shrink-0 text-muted-foreground">
              {uploadStatusLabel(upload.status, t)}
            </span>
            {upload.error && (
              <span className="max-w-[40%] truncate text-destructive" title={upload.error}>
                {upload.error}
              </span>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
