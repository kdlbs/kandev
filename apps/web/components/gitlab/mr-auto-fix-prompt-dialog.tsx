"use client";

import { Trans, useTranslation } from "react-i18next";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import Link from "@/components/routing/app-link";

const MR_FEEDBACK_PLACEHOLDER = "{{mr.feedback}}";

function insertMRFeedbackPlaceholder(prompt: string) {
  if (prompt.includes(MR_FEEDBACK_PLACEHOLDER)) return prompt;
  const trimmedEnd = prompt.trimEnd();
  if (!trimmedEnd) return MR_FEEDBACK_PLACEHOLDER;
  return `${trimmedEnd}\n\n${MR_FEEDBACK_PLACEHOLDER}`;
}

/**
 * Per-task auto-fix prompt editor, split out of mr-automation-controls.tsx
 * to keep that file under the 600-line cap. Mirrors GitHub's
 * CIAutomationPromptDialog.
 */
export function MRAutoFixPromptDialog({
  open,
  prompt,
  saving,
  onPromptChange,
  onClose,
  onSave,
  onReset,
}: {
  open: boolean;
  prompt: string;
  saving: boolean;
  onPromptChange: (value: string) => void;
  onClose: () => void;
  onSave: () => void;
  onReset: () => void;
}) {
  const { t } = useTranslation();
  const trimmed = prompt.trim();
  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t("gitlab:mrAutoFixPrompt")}</DialogTitle>
          <DialogDescription>
            <Trans
              i18nKey="gitlab:mrAutoFixPromptDescription"
              values={{ placeholder: MR_FEEDBACK_PLACEHOLDER }}
            >
              <code
                data-testid="mr-auto-fix-feedback-placeholder"
                className="rounded bg-muted px-1 py-0.5 text-[11px]"
              />
            </Trans>
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-3">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <Label htmlFor="task-mr-auto-fix-prompt" className="text-xs">
              {t("gitlab:mrAutomationTaskAutoFixPrompt")}
            </Label>
            <div className="flex shrink-0 flex-wrap items-center justify-end gap-2">
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="h-7 cursor-pointer px-2 text-xs"
                onClick={() => onPromptChange(insertMRFeedbackPlaceholder(prompt))}
              >
                {t("gitlab:mrAutoFixInsertMrFeedback")}
              </Button>
              <Link
                href="/settings/prompts"
                className="cursor-pointer text-xs text-primary hover:underline"
                onClick={(e) => e.stopPropagation()}
              >
                {t("gitlab:mrAutomationEditDefaultPrompt")}
              </Link>
            </div>
          </div>
          <div
            data-testid="mr-auto-fix-feedback-help"
            className="rounded-md border border-border/70 bg-muted/30 p-3 text-xs leading-relaxed text-muted-foreground"
          >
            <p>{t("gitlab:mrAutomationPlaceholderExplanation")}</p>
            <p className="mt-2">{t("gitlab:mrAutomationOmitPlaceholderExplanation")}</p>
          </div>
          <Textarea
            id="task-mr-auto-fix-prompt"
            value={prompt}
            onChange={(event) => onPromptChange(event.target.value)}
            rows={10}
            className="max-h-[50vh] min-h-48 resize-y font-mono text-xs"
          />
        </div>
        <DialogFooter>
          <Button variant="ghost" className="cursor-pointer" disabled={saving} onClick={onClose}>
            {t("common:cancel")}
          </Button>
          <Button variant="outline" className="cursor-pointer" disabled={saving} onClick={onReset}>
            {t("gitlab:mrAutomationUseDefault")}
          </Button>
          <Button
            className="cursor-pointer"
            disabled={saving || trimmed.length === 0}
            onClick={onSave}
          >
            {t("gitlab:mrAutomationSavePrompt")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
