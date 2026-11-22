"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconAlertTriangle } from "@tabler/icons-react";
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
import { useAppStore } from "@/components/state-provider";
import { isConflictedPrompt } from "@/hooks/domains/settings/use-prompt-conflict-count";
import {
  usePromptConflictResolver,
  type PromptConflictResolver,
} from "@/hooks/domains/settings/use-prompt-conflict-resolver";

type BannerProps = { names: string[]; busy: boolean; onReview: () => void };

// PromptConflictBanner keeps unresolved conflicts in view at the top of
// Settings > Prompts until the user chooses a version for each.
function PromptConflictBanner({ names, busy, onReview }: BannerProps) {
  const { t } = useTranslation();
  const count = names.length;
  return (
    <div
      role="alert"
      className="flex flex-wrap items-center gap-3 rounded-lg border border-amber-500/60 bg-amber-500/10 p-4"
      data-testid="prompt-conflict-banner"
    >
      <IconAlertTriangle className="h-5 w-5 shrink-0 text-amber-500" aria-hidden />
      <div className="min-w-0 flex-1 space-y-1">
        <div className="text-sm font-semibold text-foreground">
          {t("settings:promptConflictBannerTitle", { count })}
        </div>
        <div className="text-sm text-muted-foreground">
          {t("settings:promptConflictBanner", {
            count,
            names: names.map((name) => `@${name}`).join(", "),
          })}
        </div>
      </div>
      <Button
        type="button"
        size="sm"
        onClick={onReview}
        disabled={busy}
        className="cursor-pointer max-md:min-h-11 max-md:w-full [@media(pointer:coarse)]:min-h-11"
        data-testid="prompt-conflict-banner-review"
      >
        {t("settings:promptConflictReview")}
      </Button>
    </div>
  );
}

type DialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  resolver: PromptConflictResolver;
};

// PromptConflictReviewDialog resolves conflicts left by earlier syncs with the
// same review list the Prompt Sync dialog shows before saving.
function PromptConflictReviewDialog({ open, onOpenChange, resolver }: DialogProps) {
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

// PromptConflictNotice shows the conflict banner at the top of Settings >
// Prompts while any override is conflicted, and resolves conflicts in the
// review dialog. The Prompt Sync card refreshes its own status.
export function PromptConflictNotice() {
  const prompts = useAppStore((state) => state.prompts.items);
  const names = prompts.filter(isConflictedPrompt).map((prompt) => prompt.name);
  const resolver = usePromptConflictResolver();
  const [open, setOpen] = useState(false);
  const startReview = async () => {
    if (await resolver.load()) setOpen(true);
  };
  return (
    <>
      {names.length > 0 && (
        <PromptConflictBanner names={names} busy={resolver.busy} onReview={startReview} />
      )}
      <PromptConflictReviewDialog open={open} onOpenChange={setOpen} resolver={resolver} />
    </>
  );
}
