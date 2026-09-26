"use client";

import { IconColumns } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useIsDeckThread } from "@/hooks/domains/threads/use-deck-thread";
import { linkToThreads, normalizePathname } from "@/lib/links";
import { usePathname, useRouter } from "@/lib/routing/client-router";
import { useTranslation } from "react-i18next";

/** Matches the sibling icon-only controls in the chat status row. */
const STATUS_ROW_BUTTON_CLASS =
  "h-6 w-6 p-0 cursor-pointer text-muted-foreground hover:bg-muted/70 hover:text-foreground focus-visible:ring-1 focus-visible:ring-ring";

const THREADS_PATHNAME = "/threads";

/**
 * Hands this discussion back to the Threads deck, scrolled to its column.
 *
 * Rendered only when the session actually has a column, so the control is
 * never a dead end; its presence doubles as a signal that the thread is live.
 * It also hides inside the deck's own columns, which render the same chat
 * panel and would otherwise offer a jump to the view already on screen.
 */
export function OpenInThreadsButton({
  taskId,
  sessionId,
}: {
  taskId: string | null;
  sessionId: string | null;
}) {
  const { t } = useTranslation();
  const router = useRouter();
  const pathname = usePathname();
  const isDeckThread = useIsDeckThread(taskId, sessionId);

  if (!isDeckThread || !taskId || normalizePathname(pathname) === THREADS_PATHNAME) return null;
  const label = t("threads:openInThreads");

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className={STATUS_ROW_BUTTON_CLASS}
          aria-label={label}
          data-testid="open-in-threads-button"
          onClick={() => router.push(linkToThreads(undefined, taskId, sessionId ?? undefined))}
        >
          <IconColumns className="h-6 w-6" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}
