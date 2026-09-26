"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconInfoCircle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { cn } from "@/lib/utils";
import type { Automation } from "@/lib/types/automation";
import { AutomationPromptCard } from "./automation-prompt-card";
import { nextFiring } from "./automation-rows";

/**
 * The detail behind a button, rather than a block above the conversation.
 *
 * The standing instruction used to sit pinned at the top of the run view. It is
 * the same text on every run and it is long, so it pushed the thing the reader
 * came for — what the agent actually said — down the page on every visit, on
 * every run. It has not been deleted: it moved beside the transcript into the
 * runs rail, where the reader asks for it once and it stays out of the way the
 * rest of the time.
 *
 * The next-firing line rides along because the topbar deliberately hides it on
 * a phone for want of width, and the drawer is where that note said it would
 * be. Reusing `AutomationPromptCard` rather than restating the markup keeps the
 * expand/clamp behaviour identical to what it replaced.
 *
 * Shared by the rail and the mobile drawer for the same reason `RunGroup` is:
 * two copies of "what a run's detail looks like" would drift.
 */
export function RunDetailDisclosure({
  automation,
  openRuns,
  className,
}: {
  automation: Automation;
  /** Feeds the next-firing line, which is a fact about the automation's queue. */
  openRuns: number;
  className?: string;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const next = nextFiring(automation, openRuns);

  return (
    <div className={cn("flex flex-col", className)} data-testid="run-detail-disclosure">
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="h-7 cursor-pointer justify-start gap-1.5 px-2 text-xs text-muted-foreground hover:text-foreground"
        aria-expanded={open}
        onClick={() => setOpen((shown) => !shown)}
        data-testid="run-detail-toggle"
      >
        <IconInfoCircle className="h-3.5 w-3.5" />
        {open ? t("automations:hideRunDetail") : t("automations:showRunDetail")}
      </Button>
      {open && (
        <div className="flex flex-col gap-2 px-2 pb-2 pt-1" data-testid="run-detail-panel">
          <p
            className={cn(
              "text-xs",
              next.kind === "reason"
                ? "text-amber-600 dark:text-amber-500"
                : "text-muted-foreground",
            )}
            data-testid="run-detail-next-run"
          >
            {next.text}
          </p>
          <AutomationPromptCard prompt={automation.prompt ?? ""} />
        </div>
      )}
    </div>
  );
}
