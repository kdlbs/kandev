"use client";

import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { IconPaperclip, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { attachmentContentUrl } from "@/lib/api/domains/attachment-api";
import { useTranslation } from "react-i18next";
import type { useCommentAttachments } from "./use-comment-attachments";

export type CommentAttachments = ReturnType<typeof useCommentAttachments>;

const controlClass = "h-7 cursor-pointer text-xs max-md:h-11 pointer-coarse:h-11";

function AttachmentFileLabel({ file }: { file: CommentAttachments["files"][number] }) {
  const { t } = useTranslation();
  const contents = (
    <>
      <span className="truncate">{file.name}</span>
      <span className="text-muted-foreground">
        {t(
          {
            failed: "task:attachmentFailed",
            ready: "task:attachmentReady",
            uploading: "task:attachmentUploading",
          }[file.status],
        )}
      </span>
    </>
  );
  const id = file.descriptor?.attachment_id;
  return file.status === "ready" && id ? (
    <a
      className="flex min-w-0 flex-1 cursor-pointer flex-col justify-center text-xs hover:underline max-md:min-h-11 pointer-coarse:min-h-11"
      href={attachmentContentUrl(id)}
      target="_blank"
      rel="noreferrer"
      title={file.name}
    >
      {contents}
    </a>
  ) : (
    <div className="flex min-w-0 flex-1 flex-col text-xs" title={file.name}>
      {contents}
    </div>
  );
}

function AttachmentRow({
  file,
  disabled,
  onRetry,
  onRemove,
}: {
  file: CommentAttachments["files"][number];
  disabled?: boolean;
  onRetry: CommentAttachments["retry"];
  onRemove: CommentAttachments["remove"];
}) {
  const { t } = useTranslation();
  return (
    <li
      className="flex items-center gap-1"
      data-testid="comment-attachment-row"
      data-status={file.status}
    >
      <AttachmentFileLabel file={file} />
      {file.status === "failed" && file.local && (
        <Button
          type="button"
          size="sm"
          variant="ghost"
          disabled={disabled}
          className={controlClass}
          onClick={() => onRetry(file.key)}
        >
          {t("task:retryAttachment")}
        </Button>
      )}
      <Button
        type="button"
        size="sm"
        variant="ghost"
        disabled={disabled}
        className="h-7 w-7 cursor-pointer p-0 max-md:size-11 pointer-coarse:size-11"
        aria-label={t("task:removeAttachment", { name: file.name })}
        onClick={() => onRemove(file.key)}
      >
        <IconTrash className="size-4" />
      </Button>
    </li>
  );
}

export function CommentAttachmentControl({
  controller,
  disabled,
  children,
}: {
  controller: CommentAttachments;
  disabled?: boolean;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  const input = useRef<HTMLInputElement>(null);
  const [open, setOpen] = useState(false);
  const listId = useId();
  const trigger = useRef<HTMLButtonElement>(null);
  const picker = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (open) picker.current?.focus();
  }, [open]);

  return (
    <>
      {open && (
        <div
          id={listId}
          className="my-2 space-y-1 rounded-md border p-2"
          data-testid="comment-attachment-list"
          onKeyDown={(event) => {
            if (event.key !== "Escape") return;
            event.preventDefault();
            event.stopPropagation();
            setOpen(false);
            trigger.current?.focus();
          }}
        >
          <Button
            ref={picker}
            type="button"
            size="sm"
            variant="outline"
            disabled={disabled}
            className={controlClass}
            onClick={() => input.current?.click()}
          >
            {t("task:addFiles")}
          </Button>
          <ul
            className="max-h-[min(13rem,25dvh)] space-y-1 overflow-y-auto overscroll-contain"
            aria-live="polite"
          >
            {controller.files.map((file) => (
              <AttachmentRow
                key={file.key}
                file={file}
                disabled={disabled}
                onRetry={controller.retry}
                onRemove={controller.remove}
              />
            ))}
          </ul>
        </div>
      )}
      <div className="flex flex-wrap items-center gap-x-1">
        <input
          ref={input}
          type="file"
          multiple
          hidden
          data-testid="comment-attachment-input"
          onChange={(event) => {
            controller.addFiles(Array.from(event.currentTarget.files ?? []));
            setOpen(true);
            event.currentTarget.value = "";
          }}
        />
        <Button
          ref={trigger}
          type="button"
          size="sm"
          variant="ghost"
          disabled={disabled}
          className={controlClass}
          aria-expanded={open}
          aria-controls={open ? listId : undefined}
          data-testid="comment-attachment-trigger"
          onClick={() => setOpen(!open)}
        >
          <IconPaperclip className="size-4" />
          {t("task:attachFiles")}
          {controller.files.length > 0 && (
            <span className="tabular-nums">({controller.files.length})</span>
          )}
        </Button>
        <div className="min-w-fit flex-1">{children}</div>
      </div>
    </>
  );
}
