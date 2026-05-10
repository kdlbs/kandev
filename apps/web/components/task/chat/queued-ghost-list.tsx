"use client";

import { useCallback } from "react";
import { IconLayoutList, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui";
import { cn } from "@/lib/utils";
import { useQueue } from "@/hooks/domains/session/use-queue";
import { QueuedGhostMessage } from "./queued-ghost-message";
import type { QueuedMessage } from "@/lib/state/slices/session/types";

type QueuedGhostListProps = {
  sessionId: string | null;
  isArchived: boolean;
};

/** Inter-task messages flow through dispatchTaskMessage with queued_by="agent".
 *  We disable edit on those so a user can't rewrite another task's prompt mid-flight. */
function canUserEditEntry(entry: QueuedMessage): boolean {
  return entry.queued_by !== "agent";
}

/**
 * Renders the per-session FIFO queue as a stack of ghost messages plus a small
 * "n/max queued · clear all" pill above the chat input. Drained entries
 * disappear automatically when message.queue.status_changed lands.
 */
export function QueuedGhostList({ sessionId, isArchived }: QueuedGhostListProps) {
  const { entries, count, max, isFull, clearAll, editEntry, removeEntry } = useQueue(sessionId);

  const handleSave = useCallback(
    async (entryId: string, content: string) => {
      await editEntry(entryId, content);
    },
    [editEntry],
  );

  const handleRemove = useCallback(
    async (entryId: string) => {
      try {
        await removeEntry(entryId);
      } catch (err) {
        console.error("Failed to remove queued entry:", err);
      }
    },
    [removeEntry],
  );

  if (isArchived || !sessionId || entries.length === 0) {
    return null;
  }

  return (
    <div className="flex-shrink-0 space-y-1.5 bg-card px-3 pt-1.5 pb-1">
      <QueueCountPill count={count} max={max} isFull={isFull} onClear={clearAll} />
      <div className="space-y-1.5" data-testid="queued-ghost-list">
        {entries.map((entry) => (
          <QueuedGhostMessage
            key={entry.id}
            entry={entry}
            canEdit={canUserEditEntry(entry)}
            onSave={(content) => handleSave(entry.id, content)}
            onRemove={() => handleRemove(entry.id)}
          />
        ))}
      </div>
    </div>
  );
}

type QueueCountPillProps = {
  count: number;
  max: number;
  isFull: boolean;
  onClear: () => Promise<void>;
};

function QueueCountPill({ count, max, isFull, onClear }: QueueCountPillProps) {
  const handleClear = useCallback(() => {
    void onClear();
  }, [onClear]);
  return (
    <div className="flex items-center gap-2 text-xs text-muted-foreground">
      <span
        className={cn(
          "inline-flex items-center gap-1 rounded-full bg-muted px-2 py-0.5",
          isFull && "text-amber-600 dark:text-amber-400",
        )}
      >
        <IconLayoutList className="h-3 w-3" />
        {max > 0 ? `${count}/${max} queued` : `${count} queued`}
        {isFull ? " · full" : ""}
      </span>
      <Button
        variant="ghost"
        size="sm"
        className="h-6 cursor-pointer px-2 text-xs text-muted-foreground hover:text-foreground"
        onClick={handleClear}
        title="Clear all queued messages"
      >
        <IconTrash className="mr-1 h-3 w-3" />
        Clear all
      </Button>
    </div>
  );
}
