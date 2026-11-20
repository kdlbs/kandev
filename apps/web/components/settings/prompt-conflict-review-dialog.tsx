"use client";

import { useTranslation } from "react-i18next";
import { IconAlertTriangle } from "@tabler/icons-react";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { PromptConflictReviewList } from "@/components/settings/prompt-conflict-review";
import type { PromptConflictResolver } from "@/hooks/domains/settings/use-prompt-conflict-resolver";

type BannerProps = { count: number; busy: boolean; onReview: () => void };

// PromptConflictBanner keeps unresolved conflicts visible on Settings > Prompts
// until the user chooses a version for each.
export function PromptConflictBanner({ count, busy, onReview }: BannerProps) {
  const { t } = useTranslation();
  return (
    <Alert data-testid="prompt-conflict-banner" className="flex flex-wrap items-center gap-3">
      <IconAlertTriangle className="h-4 w-4 text-amber-500" aria-hidden />
      <AlertDescription className="min-w-0 flex-1">
        {t("settings:promptConflictBanner", { count })}
      </AlertDescription>
      <Button
        type="button"
        size="sm"
        variant="outline"
        onClick={onReview}
        disabled={busy}
        className="cursor-pointer max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
        data-testid="prompt-conflict-banner-review"
      >
        {t("settings:promptConflictReview")}
      </Button>
    </Alert>
  );
}

type DialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  resolver: PromptConflictResolver;
};

// PromptConflictReviewDialog resolves conflicts left by earlier syncs with the
// same review list the Prompt Sync dialog shows before saving.
export function PromptConflictReviewDialog({ open, onOpenChange, resolver }: DialogProps) {
  const { t } = useTranslation();
  const changeOpen = (next: boolean) => {
    if (!next) resolver.review.close();
    onOpenChange(next);
  };
  const apply = async () => {
    if (await resolver.apply()) onOpenChange(false);
  };
  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogContent
        className="sm:max-w-lg max-h-[calc(100dvh-2rem)] flex flex-col overflow-hidden"
        data-testid="prompt-conflict-dialog"
      >
        <div className="min-h-0 flex-1 space-y-4 overflow-y-auto">
          <DialogHeader>
            <DialogTitle>{t("settings:promptConflictReviewTitle")}</DialogTitle>
            <DialogDescription>{t("settings:promptConflictResolveDescription")}</DialogDescription>
          </DialogHeader>
          <PromptConflictReviewList review={resolver.review} />
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => changeOpen(false)}
              disabled={resolver.busy}
              className="cursor-pointer"
            >
              {t("common:cancel")}
            </Button>
            <Button
              type="button"
              onClick={apply}
              disabled={resolver.busy || !resolver.review.active}
              className="cursor-pointer"
              data-testid="prompt-conflict-apply"
              data-dialog-default-action
            >
              {t("settings:promptConflictApply")}
            </Button>
          </DialogFooter>
        </div>
      </DialogContent>
    </Dialog>
  );
}
