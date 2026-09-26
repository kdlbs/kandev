"use client";

import { useState, useRef, useEffect } from "react";
import { Button } from "@kandev/ui/button";
import { Textarea } from "@kandev/ui/textarea";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import { IconPlayerPlay, IconSend, IconX } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";

interface CommentFormProps {
  initialContent?: string;
  onSubmit: (content: string) => void;
  onCancel?: () => void;
  onSubmitAndRun?: (content: string) => void;
  isEditing?: boolean;
  autoFocus?: boolean;
}

function ActionButtons({
  disabled,
  modKey,
  isEditing,
  showRunButton,
  onCancel,
  onSubmit,
  onSubmitAndRun,
}: {
  disabled: boolean;
  modKey: string;
  isEditing: boolean;
  showRunButton: boolean;
  onCancel?: () => void;
  onSubmit: () => void;
  onSubmitAndRun?: () => void;
}) {
  const { t } = useTranslation();
  return (
    <TooltipProvider delayDuration={400}>
      <div className="flex gap-1">
        {onCancel && (
          <Button
            size="sm"
            variant="ghost"
            onClick={onCancel}
            className="h-6 cursor-pointer px-2 text-xs"
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
                className={`h-6 cursor-pointer px-2 text-xs ${showRunButton ? "rounded-r-none border-r-0" : ""}`}
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
                  className="h-6 cursor-pointer gap-1 rounded-l-none px-2 text-xs"
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
  onSubmit,
  onCancel,
  onSubmitAndRun,
  isEditing = false,
  autoFocus = true,
}: CommentFormProps) {
  const { t } = useTranslation();
  const [content, setContent] = useState(initialContent);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    if (autoFocus && textareaRef.current) textareaRef.current.focus();
  }, [autoFocus]);

  const handleSubmit = () => {
    const trimmed = content.trim();
    if (trimmed) {
      onSubmit(trimmed);
      setContent("");
    }
  };

  const handleSubmitAndRun = () => {
    const trimmed = content.trim();
    if (trimmed && onSubmitAndRun) {
      onSubmitAndRun(trimmed);
      setContent("");
    }
  };

  const isMac = typeof navigator !== "undefined" && navigator.platform?.includes("Mac");
  const modKey = isMac ? "⌘" : "Ctrl";

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      if (e.shiftKey && onSubmitAndRun) handleSubmitAndRun();
      else handleSubmit();
    } else if (e.key === "Escape" && onCancel) {
      e.preventDefault();
      onCancel();
    }
  };

  const disabled = !content.trim();
  const showRunButton = !!onSubmitAndRun && !isEditing;

  return (
    <div className="flex flex-col gap-2 rounded-md border border-border bg-card p-2 shadow-md">
      <Textarea
        ref={textareaRef}
        value={content}
        onChange={(e) => setContent(e.target.value)}
        onKeyDown={handleKeyDown}
        placeholder={t("diff:addAComment")}
        className="min-h-[60px] resize-none text-xs"
        rows={2}
      />
      <div className="flex items-center justify-between gap-2">
        <span className="text-[10px] text-muted-foreground">
          {onSubmitAndRun
            ? t("diff:enterToAddShiftEnterToRun", { modKey })
            : t("diff:enterToAdd", { modKey })}
        </span>
        <ActionButtons
          disabled={disabled}
          modKey={modKey}
          isEditing={isEditing}
          showRunButton={showRunButton}
          onCancel={onCancel}
          onSubmit={handleSubmit}
          onSubmitAndRun={showRunButton ? handleSubmitAndRun : undefined}
        />
      </div>
    </div>
  );
}
