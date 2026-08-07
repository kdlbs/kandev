"use client";

import { useTranslation } from "react-i18next";
import { useState } from "react";
import { cn } from "@/lib/utils";

/**
 * What this automation is told to do, pinned above its transcript.
 *
 * The instruction is the same on every run, so it belongs to the automation
 * rather than to any one of them — it reads as the standing brief the agent
 * works from, which is what makes the turns underneath legible. Collapsed by
 * default because these prompts are long and the reader came for the answer,
 * not the question.
 */
const COLLAPSED_CLASS = "max-h-[9.5rem] overflow-hidden";

export function AutomationPromptCard({ prompt }: { prompt: string }) {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  if (!prompt.trim()) return null;

  return (
    <div
      className="rounded-lg border border-border/60 bg-muted/30 px-4 py-3"
      data-testid="automation-prompt"
    >
      <div className={cn("relative", !expanded && COLLAPSED_CLASS)}>
        <p className="whitespace-pre-wrap text-sm leading-relaxed text-foreground/90">{prompt}</p>
        {!expanded && (
          // Fades the clipped edge rather than cutting mid-line, so it reads as
          // "there is more" instead of as a rendering fault.
          <div
            aria-hidden="true"
            className="pointer-events-none absolute inset-x-0 bottom-0 h-10 bg-gradient-to-t from-muted/60 to-transparent"
          />
        )}
      </div>
      <button
        type="button"
        onClick={() => setExpanded((open) => !open)}
        className="mt-1 cursor-pointer text-xs font-medium text-muted-foreground hover:text-foreground transition-colors"
        data-testid="automation-prompt-toggle"
      >
        {expanded ? t("automations:showLess") : t("automations:showMore")}
      </button>
    </div>
  );
}
