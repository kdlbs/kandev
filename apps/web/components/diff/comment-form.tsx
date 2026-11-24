"use client";

import { useState, useRef, useEffect } from "react";
import { Button } from "@kandev/ui/button";
import { Textarea } from "@kandev/ui/textarea";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import { IconPlayerPlay, IconSend, IconX } from "@tabler/icons-react";
import { cn } from "@kandev/ui/lib/utils";
import { useTranslation } from "react-i18next";

import type { CommentAttachmentOptions } from "@/components/shared/selection-comment/use-comment-attachments";
import { useCommentAttachments } from "@/components/shared/selection-comment/use-comment-attachments";
import { CommentAttachmentControl } from "@/components/shared/selection-comment/comment-attachment-control";
import type { MessageAttachment } from "@/lib/services/session-launch-service";

export type CommentFormSubmit = (content: string, attachments?: MessageAttachment[]) => unknown;

interface CommentFormProps {
  initialContent?: string;
  value?: string;
  onChange?: (content: string) => void;
  className?: string;
  onSubmit: CommentFormSubmit;
  attachmentOptions?: CommentAttachmentOptions;
  onCancel?: () => void;
  onSubmitAndRun?: CommentFormSubmit;
  isEditing?: boolean;
  autoFocus?: boolean;
  submitDisabled?: boolean;
}

function ActionButtons({
  disabled,
  modKey,
  isEditing,
  onCancel,
  onSubmit,
  onSubmitAndRun,
}: {
  disabled: boolean;
  modKey: string;
  isEditing: boolean;
  onCancel?: () => void;
  onSubmit: () => void;
  onSubmitAndRun?: () => void;
}) {
  const { t } = useTranslation();
  const showRunButton = !!onSubmitAndRun && !isEditing;
  return (
    <TooltipProvider delayDuration={400}>
      <div className="flex gap-1">
        {onCancel && (
          <Button
            size="sm"
            variant="ghost"
            onClick={onCancel}
            className="h-6 cursor-pointer px-2 text-xs max-md:h-11 pointer-coarse:h-11"
          >
            <IconX className="mr-1 h-3 w-3" />
            {t("common:cancel")}
          </Button>
        )}
        <div className="inline-flex">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                size="sm"
                variant={showRunButton ? "outline" : "default"}
                onClick={onSubmit}
                disabled={disabled}
                className={`h-6 cursor-pointer px-2 text-xs max-md:h-11 pointer-coarse:h-11 ${showRunButton ? "rounded-r-none border-r-0" : ""}`}
              >
                <IconSend className="mr-1 h-3 w-3" />
                {isEditing ? t("diff:update") : t("diff:add")}
              </Button>
            </TooltipTrigger>
            <TooltipContent side="bottom">
              <p>{t("diff:saveCommentForReview", { modKey })}</p>
            </TooltipContent>
          </Tooltip>
          {showRunButton && onSubmitAndRun && (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  size="sm"
                  onClick={onSubmitAndRun}
                  disabled={disabled}
                  className="h-6 cursor-pointer gap-1 rounded-l-none px-2 text-xs max-md:h-11 pointer-coarse:h-11"
                >
                  <IconPlayerPlay className="h-3 w-3" />
                  {t("diff:run")}
                </Button>
              </TooltipTrigger>
              <TooltipContent side="bottom">
                <p>{t("diff:saveAndSendToAgent", { modKey })}</p>
              </TooltipContent>
            </Tooltip>
          )}
        </div>
      </div>
    </TooltipProvider>
  );
}

export function CommentForm({
  initialContent = "",
  value,
  onChange,
  className,
  onSubmit,
  onCancel,
  onSubmitAndRun,
  isEditing = false,
  autoFocus = true,
  submitDisabled = false,
  attachmentOptions,
}: CommentFormProps) {
  const { t } = useTranslation();
  const attachments = useCommentAttachments(attachmentOptions);
  const [localContent, setLocalContent] = useState(initialContent);
  const content = value ?? localContent;
  const setContent = (next: string) => {
    if (value === undefined) setLocalContent(next);
    onChange?.(next);
  };
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    if (autoFocus && textareaRef.current) textareaRef.current.focus();
  }, [autoFocus]);

  const submit = (callback?: CommentFormSubmit) => {
    const trimmed = content.trim();
    if (!callback || !trimmed || submitDisabled || !attachments.canSubmit()) return;
    const result = attachmentOptions
      ? callback(trimmed, attachments.descriptors)
      : callback(trimmed);
    if (result !== false) setContent("");
  };
  const handleSubmit = () => submit(onSubmit);
  const handleSubmitAndRun = () => submit(isEditing ? undefined : onSubmitAndRun);

  const isMac = typeof navigator !== "undefined" && navigator.platform?.includes("Mac");
  const modKey = isMac ? "⌘" : "Ctrl";

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      if (e.shiftKey && onSubmitAndRun) handleSubmitAndRun();
      else handleSubmit();
    } else if (e.key === "Escape" && onCancel) {
      e.preventDefault();
      e.stopPropagation();
      onCancel();
    }
  };

  const disabled = !content.trim() || submitDisabled || !attachments.ready;
  const actions = (
    <div data-comment-actions className="flex flex-wrap items-center justify-end gap-2">
      <span
        data-comment-shortcuts
        className={cn("mr-auto text-[10px] text-muted-foreground", attachmentOptions && "hidden")}
      >
        {onSubmitAndRun
          ? t("diff:enterToAddShiftEnterToRun", { modKey })
          : t("diff:enterToAdd", { modKey })}
      </span>
      <ActionButtons
        disabled={disabled}
        modKey={modKey}
        isEditing={isEditing}
        onCancel={onCancel}
        onSubmit={handleSubmit}
        onSubmitAndRun={onSubmitAndRun ? handleSubmitAndRun : undefined}
      />
    </div>
  );
  return (
    <div
      className={cn(
        "flex flex-col gap-2 rounded-md border border-border bg-card p-2 shadow-md",
        className,
      )}
    >
      <Textarea
        ref={textareaRef}
        value={content}
        onChange={(e) => setContent(e.target.value)}
        onPaste={attachments.handlePaste}
        onKeyDown={handleKeyDown}
        placeholder={t("diff:addAComment")}
        className="min-h-[60px] resize-none text-xs"
        rows={2}
      />
      {attachmentOptions ? (
        <CommentAttachmentControl controller={attachments}>{actions}</CommentAttachmentControl>
      ) : (
        actions
      )}
    </div>
  );
}
