"use client";

import { IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import type { PreviewDraftEntry } from "@/hooks/use-preview-drafts";
import { cn } from "@/lib/utils";
import { PreviewFeedbackEvidence, evidenceForDraft } from "./preview-feedback-evidence";
import { CaptureNumber, type PreviewCommentList } from "./preview-comment-list";

export type PreviewUnsavedDraftsProps = {
  entries: PreviewDraftEntry[];
  onResume: (id: string) => void;
  onDiscard: (id: string) => void;
};

/** Captures dismissed before saving; each can be resumed or discarded. */
export function PreviewUnsavedDrafts({
  entries,
  onResume,
  onDiscard,
  touch,
  list,
}: PreviewUnsavedDraftsProps & { touch: boolean; list?: PreviewCommentList }) {
  const { t } = useTranslation();
  if (entries.length === 0) return null;
  const actionClass = touch ? "h-11 min-w-11" : "h-7";
  return (
    <div className="space-y-2" data-testid="preview-unsaved-drafts">
      <h3 className="text-sm font-medium">{t("task:previewUnsavedGroup")}</h3>
      <ul className="space-y-2">
        {entries.map((entry) => (
          <li
            key={entry.id}
            className={cn(
              "rounded-md border border-dashed p-3",
              list?.activeId === entry.id && "border-primary",
            )}
            data-testid="preview-unsaved-draft"
            data-active={list?.activeId === entry.id ? "true" : undefined}
          >
            <DraftRow
              entry={entry}
              actionClass={actionClass}
              number={list?.numbers.get(entry.id)}
              onResume={onResume}
              onDiscard={onDiscard}
            />
          </li>
        ))}
      </ul>
    </div>
  );
}

function DraftRow({
  entry,
  actionClass,
  number,
  onResume,
  onDiscard,
}: {
  entry: PreviewDraftEntry;
  actionClass: string;
  number: number | undefined;
  onResume: (id: string) => void;
  onDiscard: (id: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <div className="flex items-start gap-2">
        <CaptureNumber value={number} />
        <div className="min-w-0 flex-1">
          <p className="truncate text-xs text-muted-foreground">{entry.payload.page_route}</p>
          <div className="mt-1">
            <PreviewFeedbackEvidence evidence={evidenceForDraft(entry.payload, "")} />
          </div>
        </div>
        <Button
          type="button"
          size="sm"
          className={actionClass}
          data-testid="preview-unsaved-resume"
          onClick={() => onResume(entry.id)}
        >
          {t("task:previewResumeDraft")}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          className={actionClass}
          aria-label={t("task:previewDiscardSelection")}
          data-testid="preview-unsaved-discard"
          onClick={() => onDiscard(entry.id)}
        >
          <IconX className="h-4 w-4" />
        </Button>
      </div>
      {entry.comment && <p className="mt-2 break-words text-xs">{entry.comment}</p>}
    </>
  );
}
