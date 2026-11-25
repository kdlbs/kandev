"use client";

import { useCallback, useState } from "react";
import { useToast } from "@/components/toast-provider";
import { useAppStoreApi } from "@/components/state-provider";
import { refreshPrompts } from "@/lib/state/prompts-loader";
import { t } from "@/lib/i18n";
import { checkPromptSync, runPromptSync } from "@/lib/api/domains/prompt-sync-api";
import { usePromptConflictReview } from "@/hooks/domains/settings/use-prompt-conflict-review";

// usePromptConflictResolver resolves conflicts left by earlier syncs: load
// checks the saved configuration for its conflicts, and apply runs a sync with
// the user's choices. A failure keeps the review open with toast feedback.
export function usePromptConflictResolver() {
  const { toast } = useToast();
  const store = useAppStoreApi();
  const review = usePromptConflictReview();
  const [busy, setBusy] = useState(false);
  const { open, close } = review;

  const load = useCallback(async () => {
    setBusy(true);
    try {
      const res = await checkPromptSync();
      open(res.conflicts);
      // No conflict left means the list's conflict flags are stale.
      if (res.conflicts.length === 0) void refreshPrompts(store);
      return res.conflicts.length > 0;
    } catch (err) {
      toast({
        description: t("settings:promptConflictLoadFailed", { error: String(err) }),
        variant: "error",
      });
      return false;
    } finally {
      setBusy(false);
    }
  }, [open, toast, store]);

  const apply = useCallback(async () => {
    setBusy(true);
    try {
      const res = await runPromptSync(review.choices);
      if (res.error) {
        toast({
          description: t("settings:promptSyncFailed", { error: res.error }),
          variant: "error",
        });
        return false;
      }
      if (res.result?.conflicts?.length) {
        const next = await checkPromptSync();
        open(next.conflicts);
        if (next.conflicts.length > 0) return false;
      }
      close();
      toast({ description: t("settings:promptConflictResolved"), variant: "success" });
      return true;
    } catch (err) {
      toast({
        description: t("settings:promptSyncFailed", { error: String(err) }),
        variant: "error",
      });
      return false;
    } finally {
      setBusy(false);
    }
  }, [review.choices, open, close, toast]);

  return { review, busy, load, apply };
}

export type PromptConflictResolver = ReturnType<typeof usePromptConflictResolver>;
