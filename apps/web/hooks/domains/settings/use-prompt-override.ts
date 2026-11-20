"use client";

import { useCallback, useState } from "react";
import { useToast } from "@/components/toast-provider";
import { t } from "@/lib/i18n";
import { overridePrompt, revertPromptOverride } from "@/lib/api/domains/prompt-sync-api";

// usePromptOverride owns the Override and Revert actions for synced prompts.
// The prompt list refreshes through the prompts.changed WebSocket event.
export function usePromptOverride() {
  const { toast } = useToast();
  const [pendingId, setPendingId] = useState<string | null>(null);

  const run = useCallback(
    async (
      promptId: string,
      action: (id: string) => Promise<unknown>,
      doneKey: string,
      failedKey: string,
    ) => {
      setPendingId(promptId);
      try {
        await action(promptId);
        toast({ description: t(doneKey), variant: "success" });
        return true;
      } catch (err) {
        toast({ description: t(failedKey, { error: String(err) }), variant: "error" });
        return false;
      } finally {
        setPendingId(null);
      }
    },
    [toast],
  );

  const override = useCallback(
    (promptId: string) =>
      run(promptId, overridePrompt, "settings:promptOverrideDone", "settings:promptOverrideFailed"),
    [run],
  );
  const revert = useCallback(
    (promptId: string) =>
      run(
        promptId,
        revertPromptOverride,
        "settings:promptRevertDone",
        "settings:promptRevertFailed",
      ),
    [run],
  );

  return { pendingId, override, revert };
}
