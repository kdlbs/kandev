"use client";

import { useTranslation } from "react-i18next";
import { IconFile, IconPhoto } from "@tabler/icons-react";
import { attachmentContentUrl } from "@/lib/api/domains/attachment-api";
import { AttachmentPicker } from "./attachment-picker";
import type { ClarificationAnswerAttachmentController } from "./use-clarification-answer-attachments";

export function ClarificationAnswerAttachments({
  questionId,
  controller,
  disabled,
  variant,
}: {
  questionId: string;
  controller: ClarificationAnswerAttachmentController;
  disabled?: boolean;
  variant: "input" | "header";
}) {
  const { t } = useTranslation();
  return (
    <AttachmentPicker
      files={controller.filesFor(questionId)}
      onAddFiles={(files) => controller.addFiles(questionId, files)}
      onRetry={(id) => controller.retry(questionId, id)}
      onRemove={(id) => controller.remove(questionId, id)}
      title={t("task:answerAttachments")}
      hint={t("task:answerAttachmentsHint")}
      testId="clarification-attachment"
      disabled={disabled}
      variant={variant}
    />
  );
}

export function ClarificationAttachmentSummary({
  attachments,
}: {
  attachments: readonly { attachment_id: string; type: string; name: string }[];
}) {
  if (attachments.length === 0) return null;
  return (
    <ul
      data-testid="clarification-answer-attachment-summary"
      className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs"
    >
      {attachments.map((attachment) => (
        <li key={attachment.attachment_id}>
          <a
            href={attachmentContentUrl(attachment.attachment_id)}
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center gap-1 underline underline-offset-2 text-muted-foreground hover:text-foreground"
          >
            {attachment.type === "image" ? (
              <IconPhoto className="h-3 w-3" />
            ) : (
              <IconFile className="h-3 w-3" />
            )}
            {attachment.name}
          </a>
        </li>
      ))}
    </ul>
  );
}
