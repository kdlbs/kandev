"use client";

import { IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import type { PreviewDraftEntry } from "@/hooks/use-preview-drafts";
import { PreviewFeedbackEvidence, evidenceForDraft } from "./preview-feedback-evidence";

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
}: PreviewUnsavedDraftsProps & { touch: boolean }) {
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
            className="rounded-md border border-dashed p-3"
            data-testid="preview-unsaved-draft"
          >
            <div className="flex items-start gap-2">
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
          </li>
        ))}
      </ul>
    </div>
  );
}
