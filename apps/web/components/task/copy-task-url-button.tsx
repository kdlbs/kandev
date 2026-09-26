"use client";

import { useEffect, useState } from "react";
import { IconCheck, IconCopy } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { copyToClipboard } from "@/lib/utils/copy-to-clipboard";
import { linkToTask } from "@/lib/links";

const COPIED_FEEDBACK_DURATION_MS = 1500;

/**
 * Copies the previewed task's detail URL. Icon and tooltip must stay visually
 * distinct from the Link submenu's `IconLink` (linking an external PR/issue),
 * a different action this control is not a shortcut for.
 */
export function CopyTaskUrlButton({ taskId }: { taskId: string }) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (!copied) return;
    const timeout = window.setTimeout(() => setCopied(false), COPIED_FEEDBACK_DURATION_MS);
    return () => window.clearTimeout(timeout);
  }, [copied]);

  const stableLabel = t("task:copyTaskLink");
  const tooltipLabel = t(copied ? "task:taskLinkCopied" : "task:copyTaskLinkHint");

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="h-8 w-8 cursor-pointer"
          onClick={() => {
            const url = `${window.location.origin}${linkToTask(taskId)}`;
            void copyToClipboard(url).then((success) => {
              if (success) setCopied(true);
            });
          }}
          aria-label={stableLabel}
          data-testid="task-preview-copy-url"
        >
          {copied ? (
            <IconCheck className="h-4 w-4 text-green-500" />
          ) : (
            <IconCopy className="h-4 w-4" />
          )}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{tooltipLabel}</TooltipContent>
      <span role="status" className="sr-only">
        {copied ? t("task:taskLinkCopied") : ""}
      </span>
    </Tooltip>
  );
}
