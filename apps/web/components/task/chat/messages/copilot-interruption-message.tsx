"use client";

import { memo, useState } from "react";
import { IconChevronDown, IconChevronRight, IconInfoCircle } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import type { Message } from "@/lib/types/http";

type CopilotInterruptionKind = "clarification" | "tool";

export const CopilotInterruptionMessage = memo(function CopilotInterruptionMessage({
  comment,
  kind,
}: {
  comment: Message;
  kind: CopilotInterruptionKind;
}) {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  const detailsId = `copilot-interruption-details-${comment.id}`;
  const summary = t(
    kind === "clarification"
      ? "task:copilotInterruptionClarification"
      : "task:copilotInterruptionTool",
  );

  return (
    <div
      className="w-full text-muted-foreground"
      data-testid="copilot-interruption-status"
      role="status"
      aria-live="polite"
    >
      <div className="flex min-w-0 flex-wrap items-start gap-x-2 gap-y-1">
        <IconInfoCircle className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
        <p className="min-w-0 flex-1 text-xs leading-5">{summary}</p>
        <button
          type="button"
          className={cn(
            "inline-flex h-6 min-h-6 shrink-0 items-center gap-1 rounded px-1 text-xs",
            "text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
            "max-md:h-auto max-md:min-h-11 [@media(pointer:coarse)]:h-auto [@media(pointer:coarse)]:min-h-11",
          )}
          aria-expanded={expanded}
          aria-controls={expanded ? detailsId : undefined}
          onClick={() => setExpanded((value) => !value)}
        >
          {t(expanded ? "task:copilotInterruptionHideDetails" : "task:copilotInterruptionDetails")}
          {expanded ? (
            <IconChevronDown className="size-3.5" aria-hidden="true" />
          ) : (
            <IconChevronRight className="size-3.5" aria-hidden="true" />
          )}
        </button>
      </div>
      {expanded ? (
        <p
          id={detailsId}
          className="mt-1 max-w-full whitespace-pre-wrap break-words text-xs leading-5 text-foreground [overflow-wrap:anywhere]"
          data-testid="copilot-interruption-details"
        >
          {comment.content}
        </p>
      ) : null}
    </div>
  );
});
