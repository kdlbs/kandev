"use client";

import { useTranslation } from "react-i18next";
import { usePromptConflictCount } from "@/hooks/domains/settings/use-prompt-conflict-count";
import { cn } from "@/lib/utils";

// PromptConflictIndicator is the dot shown on Settings entry points while a
// prompt conflict waits for the user's choice. It renders nothing otherwise.
export function PromptConflictIndicator({ className }: { className?: string }) {
  const { t } = useTranslation();
  const count = usePromptConflictCount();
  if (count === 0) return null;
  return (
    <span
      role="img"
      aria-label={t("settings:promptConflictIndicator")}
      title={t("settings:promptConflictIndicator")}
      className={cn("size-2 shrink-0 rounded-full bg-amber-500", className)}
      data-testid="prompt-conflict-indicator"
    />
  );
}
