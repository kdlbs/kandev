"use client";

import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import AppLink from "@/components/routing/app-link";
import { Trans, useTranslation } from "react-i18next";

const PR_FEEDBACK_PLACEHOLDER = "{{pr.feedback}}";

function insertPRFeedbackPlaceholder(prompt: string) {
  if (prompt.includes(PR_FEEDBACK_PLACEHOLDER)) return prompt;
  const trimmedEnd = prompt.trimEnd();
  if (!trimmedEnd) return PR_FEEDBACK_PLACEHOLDER;
  return `${trimmedEnd}\n\n${PR_FEEDBACK_PLACEHOLDER}`;
}

/**
 * Auto-fix prompt override editor. This override remains task-level (it
 * applies to every linked PR's auto-fix prompt) even though the auto-fix
 * enable/disable switch itself is now per-PR.
 */
export function CIAutomationPromptDialog({
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
          <DialogTitle>{t("github:autoFixPrompt")}</DialogTitle>
          <DialogDescription>
            <Trans
              i18nKey="github:autoFixPromptDescription"
              values={{ placeholder: PR_FEEDBACK_PLACEHOLDER }}
            >
              <code
                data-testid="ci-auto-fix-pr-feedback-placeholder"
                className="rounded bg-muted px-1 py-0.5 text-[11px]"
              />
            </Trans>
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-3">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <Label htmlFor="task-ci-auto-fix-prompt" className="text-xs">
              {t("github:taskAutoFixPrompt")}
            </Label>
            <div className="flex shrink-0 flex-wrap items-center justify-end gap-2">
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="h-7 cursor-pointer px-2 text-xs"
                onClick={() => onPromptChange(insertPRFeedbackPlaceholder(prompt))}
              >
                {t("github:insertPrFeedback")}
              </Button>
              <AppLink
                href="/settings/prompts"
                className="cursor-pointer text-xs text-primary hover:underline"
                onClick={(e) => e.stopPropagation()}
              >
                {t("github:editDefaultPrompt")}
              </AppLink>
            </div>
          </div>
          <div
            data-testid="ci-auto-fix-pr-feedback-help"
            className="rounded-md border border-border/70 bg-muted/30 p-3 text-xs leading-relaxed text-muted-foreground"
          >
            <p>{t("github:thePlaceholderInsertsTheCurrentPr")}</p>
            <p className="mt-2">{t("github:omitThePlaceholderIfYouWant")}</p>
          </div>
          <Textarea
            id="task-ci-auto-fix-prompt"
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
            {t("github:useDefault")}
          </Button>
          <Button
            className="cursor-pointer"
            disabled={saving || trimmed.length === 0}
            onClick={onSave}
          >
            {t("github:savePrompt")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
