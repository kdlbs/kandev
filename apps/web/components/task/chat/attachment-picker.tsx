"use client";

import { useContext, useRef, useState } from "react";
import {
  IconAlertTriangle,
  IconFile,
  IconLoader2,
  IconPaperclip,
  IconPhoto,
  IconTrash,
} from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import {
  Drawer,
  DrawerContent,
  DrawerHeader,
  DrawerNested,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { ModalDrawerContentContext } from "@kandev/ui/modal-drawer-context";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { cn } from "@/lib/utils";
import { ImagePreviewDialog } from "./image-preview-dialog";
import { MAX_FILES, MAX_TOTAL_SIZE } from "./file-attachment";
import { formatBytes } from "@/lib/utils/format-bytes";
import { attachmentContentUrl } from "@/lib/api/domains/attachment-api";
import type { FileAttachment } from "./file-attachment";

type AttachmentPickerProps = {
  files: readonly FileAttachment[];
  onAddFiles: (files: File[]) => void;
  onRetry: (id: string) => void;
  onRemove: (id: string) => void;
  title: string;
  hint: string;
  testId: string;
  disabled?: boolean;
  variant?: "input" | "header";
  linkFiles?: boolean;
};

function attachmentStatusLabelKey(file: FileAttachment): string {
  switch (file.uploadStatus) {
    case "failed":
      return "task:attachmentFailed";
    case "pending":
    case "uploading":
      return "task:attachmentUploading";
    default:
      return "task:attachmentReady";
  }
}

function fileSource(file: FileAttachment): string | null {
  if (file.preview) return file.preview;
  if (file.attachmentId) return attachmentContentUrl(file.attachmentId);
  return null;
}

function AttachmentBadge({
  testId,
  totalCount,
  failedCount,
  uploading,
}: {
  testId: string;
  totalCount: number;
  failedCount: number;
  uploading: boolean;
}) {
  if (failedCount > 0) {
    return (
      <span
        data-testid={`${testId}-badge`}
        data-state="failed"
        className="absolute -right-1 -top-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-destructive px-1 text-[9px] font-semibold leading-none text-white"
      >
        {failedCount}
      </span>
    );
  }
  if (uploading) {
    return (
      <span
        data-testid={`${testId}-badge`}
        data-state="uploading"
        className="absolute -right-1 -top-1 flex h-4 w-4 items-center justify-center rounded-full bg-blue-500 text-white"
      >
        <IconLoader2 className="h-2.5 w-2.5 animate-spin" />
      </span>
    );
  }
  if (totalCount > 0) {
    return (
      <span
        data-testid={`${testId}-badge`}
        data-state="ready"
        className="absolute -right-1 -top-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-blue-500 px-1 text-[9px] font-semibold leading-none text-white"
      >
        {totalCount}
      </span>
    );
  }
  return null;
}

function AttachmentFileName({
  file,
  linkFiles,
  touch,
}: {
  file: FileAttachment;
  linkFiles?: boolean;
  touch: boolean;
}) {
  const className = "block truncate text-xs font-medium";
  if (!linkFiles || file.uploadStatus !== "ready" || !file.attachmentId) {
    return <span className={className}>{file.fileName}</span>;
  }
  return (
    <a
      href={attachmentContentUrl(file.attachmentId)}
      target="_blank"
      rel="noreferrer"
      className={cn(
        className,
        "hover:underline focus-visible:underline",
        touch && "flex min-h-11 items-center",
      )}
    >
      {file.fileName}
    </a>
  );
}

function AttachmentFileRow({
  file,
  picker,
  touch,
}: {
  file: FileAttachment;
  picker: AttachmentPickerProps;
  touch: boolean;
}) {
  const { t } = useTranslation();
  const source = file.isImage ? fileSource(file) : null;
  return (
    <div
      data-testid={`${picker.testId}-row`}
      data-status={file.uploadStatus ?? "ready"}
      className="flex items-center gap-2 rounded-md border border-border/70 px-2 py-1.5"
    >
      <span className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded bg-muted">
        {file.isImage ? (
          <IconPhoto className="h-4 w-4 text-muted-foreground" />
        ) : (
          <IconFile className="h-4 w-4 text-muted-foreground" />
        )}
      </span>
      <span className="min-w-0 flex-1">
        <AttachmentFileName file={file} linkFiles={picker.linkFiles} touch={touch} />
        <span
          data-testid={`${picker.testId}-status`}
          className={cn(
            "block text-[10px]",
            file.uploadStatus === "failed" ? "text-destructive" : "text-muted-foreground",
          )}
        >
          {file.uploadStatus === "failed" && (
            <IconAlertTriangle className="mr-1 inline h-2.5 w-2.5" />
          )}
          {t(attachmentStatusLabelKey(file))}
        </span>
      </span>
      <span className="flex flex-shrink-0 items-center gap-1">
        {file.isImage && source && (
          <span data-testid={`${picker.testId}-inspect`} className="inline-flex">
            <ImagePreviewDialog
              src={source}
              alt={file.fileName}
              interactive
              stopShortcutPropagation
              aboveAttachmentPicker
              thumbnailClassName={touch ? "h-11 w-11 rounded" : "h-7 w-7 rounded"}
            />
          </span>
        )}
        {file.uploadStatus === "failed" && file.file && (
          <button
            type="button"
            disabled={picker.disabled}
            onClick={() => picker.onRetry(file.id)}
            data-testid={`${picker.testId}-retry`}
            className={cn(
              "cursor-pointer rounded px-1.5 text-[11px] text-foreground/80 hover:bg-muted",
              touch ? "min-h-11 px-3" : "h-7",
            )}
          >
            {t("task:retryAttachment")}
          </button>
        )}
        <button
          type="button"
          disabled={picker.disabled}
          onClick={() => picker.onRemove(file.id)}
          data-testid={`${picker.testId}-remove`}
          aria-label={t("task:removeAttachment", { name: file.fileName })}
          className={cn(
            "flex cursor-pointer items-center justify-center rounded text-muted-foreground hover:bg-muted hover:text-foreground",
            touch ? "h-11 w-11" : "h-7 w-7",
          )}
        >
          <IconTrash className="h-3.5 w-3.5" />
        </button>
      </span>
    </div>
  );
}

function AttachmentManager({
  picker,
  onPickFiles,
  touch,
}: {
  picker: AttachmentPickerProps;
  onPickFiles: () => void;
  touch: boolean;
}) {
  const { t } = useTranslation();
  const { files, title, hint } = picker;
  return (
    <div
      className="w-[min(20rem,calc(100vw-2rem))] space-y-2"
      data-testid={`${picker.testId}-manager`}
    >
      <div className="flex items-center justify-between gap-2">
        <p className="text-xs font-medium">{title}</p>
        <button
          type="button"
          onClick={onPickFiles}
          disabled={picker.disabled}
          data-testid={`${picker.testId}-add`}
          className={cn(
            "cursor-pointer rounded text-[11px] font-medium text-blue-600 hover:bg-blue-500/10 dark:text-blue-300",
            touch ? "min-h-11 px-3" : "px-2 py-1",
          )}
        >
          {t("task:addFiles")}
        </button>
      </div>
      {files.length === 0 ? (
        <p className="text-[11px] text-muted-foreground">{hint}</p>
      ) : (
        <div className="max-h-[min(14rem,40dvh)] overflow-y-auto overscroll-contain">
          <div className="space-y-1.5">
            {files.map((file) => (
              <AttachmentFileRow key={file.id} file={file} picker={picker} touch={touch} />
            ))}
          </div>
        </div>
      )}
      <p className="text-[10px] text-muted-foreground">
        {t("task:answerAttachmentsLimit", {
          maxFiles: MAX_FILES,
          maxSize: formatBytes(MAX_TOTAL_SIZE),
        })}
      </p>
    </div>
  );
}

export function AttachmentPicker(picker: AttachmentPickerProps) {
  const { disabled, files, onAddFiles, testId, title, variant } = picker;
  const { t } = useTranslation();
  const { isFinePointer, isMobile } = useResponsiveBreakpoint();
  const touchPresentation = isMobile || !isFinePointer;
  const nested = useContext(ModalDrawerContentContext);
  const DrawerRoot = nested ? DrawerNested : Drawer;
  const [open, setOpen] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const totalCount = files.length;
  const failedCount = files.filter((file) => file.uploadStatus === "failed").length;
  const uploading = files.some(
    (file) => file.uploadStatus === "pending" || file.uploadStatus === "uploading",
  );
  const trigger = (
    <button
      type="button"
      disabled={disabled}
      aria-label={title}
      aria-expanded={open}
      data-testid={`${testId}-trigger`}
      data-variant={variant}
      className={cn(
        "relative flex flex-shrink-0 items-center justify-center rounded transition-colors",
        touchPresentation ? "h-11 w-11" : "h-7 w-7",
        disabled ? "cursor-not-allowed opacity-50" : "cursor-pointer hover:bg-muted",
      )}
    >
      <IconPaperclip className="h-3.5 w-3.5" />
      <AttachmentBadge
        testId={testId}
        totalCount={totalCount}
        failedCount={failedCount}
        uploading={uploading}
      />
    </button>
  );
  const openPicker = () => inputRef.current?.click();
  const manager = (
    <AttachmentManager picker={picker} onPickFiles={openPicker} touch={touchPresentation} />
  );
  return (
    <span className="relative flex flex-shrink-0 items-center">
      <input
        ref={inputRef}
        type="file"
        multiple
        hidden
        data-testid={`${testId}-input`}
        onChange={(event) => {
          onAddFiles(Array.from(event.target.files ?? []));
          event.target.value = "";
        }}
      />
      {!touchPresentation ? (
        <Popover open={open} onOpenChange={setOpen}>
          <PopoverTrigger asChild>{trigger}</PopoverTrigger>
          <PopoverContent
            align="end"
            className="z-[70] w-auto p-3"
            onMouseDown={(event) => event.stopPropagation()}
            onKeyDown={(event) => event.stopPropagation()}
          >
            {manager}
          </PopoverContent>
        </Popover>
      ) : (
        <DrawerRoot open={open} onOpenChange={setOpen}>
          <DrawerTrigger asChild>{trigger}</DrawerTrigger>
          <DrawerContent className="z-[70]" onKeyDown={(event) => event.stopPropagation()}>
            <DrawerHeader>
              <DrawerTitle>{title}</DrawerTitle>
            </DrawerHeader>
            <div className="px-4 pb-[calc(1rem+env(safe-area-inset-bottom))]">{manager}</div>
          </DrawerContent>
        </DrawerRoot>
      )}
      <span className="sr-only" role="status" aria-live="polite">
        {t("task:answerAttachmentsStatus", {
          total: totalCount,
          failed: failedCount,
        })}
      </span>
    </span>
  );
}
