"use client";

import { Button } from "@kandev/ui/button";

import type { UtilityGenerationResult } from "@/hooks/use-utility-agent-generator";
import { useTranslation } from "react-i18next";

type PromptResultRecoveryProps = {
  pendingResult: UtilityGenerationResult | null;
  onApply: () => void;
  onCopy: () => Promise<void> | void;
};

export function PromptResultRecovery({
  pendingResult,
  onApply,
  onCopy,
}: PromptResultRecoveryProps) {
  const { t } = useTranslation();
  if (!pendingResult) {
    return null;
  }

  return (
    <div
      aria-live="polite"
      data-testid="prompt-result-recovery"
      className="flex flex-col gap-2 rounded-md border border-border/60 bg-muted/30 p-3"
    >
      <p className="text-sm text-muted-foreground">{t("common:anEnhancedPromptIsAvailable")}</p>
      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="button"
          variant="outline"
          className="cursor-pointer"
          onClick={() => void onCopy()}
        >
          {t("common:copy")}
        </Button>
        <Button type="button" className="cursor-pointer" onClick={onApply}>
          {t("common:apply")}
        </Button>
      </div>
    </div>
  );
}
