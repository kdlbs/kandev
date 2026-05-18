"use client";

import { useState } from "react";
import { IconChevronDown, IconChevronRight, IconListCheck } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import type { Message } from "@/lib/types/http";
import type { RichMetadata, TodoMetadata, TodoSnapshot } from "@/components/task/chat/types";
import { ExpandableRow } from "./expandable-row";
import { StatusIcon, resolveStatus } from "../todo-indicator";

type TodoItem = {
  text: string;
  done?: boolean;
  status?: "pending" | "in_progress" | "completed" | "failed";
};

function normalizeTodos(raw: TodoMetadata[]): TodoItem[] {
  return raw
    .map((item) => (typeof item === "string" ? { text: item, done: false } : item))
    .filter((item) => item.text);
}

function parseTodos(comment: Message): TodoItem[] {
  const metadata = comment.metadata as RichMetadata | undefined;
  return normalizeTodos(metadata?.todos ?? []);
}

function parseSnapshots(comment: Message): TodoSnapshot[] {
  const metadata = comment.metadata as RichMetadata | undefined;
  return metadata?.previous_todo_snapshots ?? [];
}

function countCompleted(items: TodoItem[]): number {
  return items.filter((t) => resolveStatus(t) === "completed").length;
}

function SnapshotHistory({ snapshots }: { snapshots: TodoSnapshot[] }) {
  const [isOpen, setIsOpen] = useState(false);
  return (
    <div className="mt-3 pt-2 border-t border-border/30">
      <button
        type="button"
        onClick={() => setIsOpen((prev) => !prev)}
        className="flex items-center gap-1 text-[10px] uppercase tracking-wide text-muted-foreground/60 hover:text-muted-foreground cursor-pointer"
        aria-expanded={isOpen}
      >
        {isOpen ? (
          <IconChevronDown className="h-3 w-3" />
        ) : (
          <IconChevronRight className="h-3 w-3" />
        )}
        Earlier updates ({snapshots.length})
      </button>
      {isOpen && (
        <div className="mt-1.5 space-y-1 text-[11px] text-muted-foreground/80">
          {snapshots.map((snap, i) => {
            const items = normalizeTodos(snap.todos);
            const done = countCompleted(items);
            // Todos may not be ordered with completed-first, so find the first
            // non-completed entry rather than indexing by the completed count.
            const nextItem = items.find((t) => resolveStatus(t) !== "completed");
            return (
              <div key={i} className="flex items-baseline gap-2">
                <span className="font-mono shrink-0">#{i + 1}</span>
                <span className="shrink-0">
                  {done}/{items.length}
                </span>
                {nextItem && (
                  <span className="truncate text-muted-foreground/60">- {nextItem.text}</span>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

export function TodoMessage({
  comment,
  defaultExpanded = false,
}: {
  comment: Message;
  defaultExpanded?: boolean;
}) {
  const todoItems = parseTodos(comment);
  const snapshots = parseSnapshots(comment);
  const [isExpanded, setIsExpanded] = useState(defaultExpanded);

  if (!todoItems.length) return null;

  const completed = countCompleted(todoItems);
  const currentTask = todoItems.find((t) => resolveStatus(t) === "in_progress");
  const totalUpdates = snapshots.length + 1;

  return (
    <ExpandableRow
      icon={<IconListCheck className="h-4 w-4 text-muted-foreground" />}
      header={
        <div className="flex items-center gap-2 text-xs min-w-0">
          <span className="text-muted-foreground text-[11px] uppercase tracking-wide shrink-0">
            Updated Todos
          </span>
          <span className="text-muted-foreground text-[11px] shrink-0">
            ({completed}/{todoItems.length})
          </span>
          {totalUpdates > 1 && (
            <span className="text-muted-foreground/60 text-[10px] shrink-0 bg-muted/60 px-1.5 rounded">
              {totalUpdates}x
            </span>
          )}
          {currentTask && (
            <>
              <span className="text-muted-foreground/40 shrink-0">·</span>
              <span className="text-muted-foreground text-[11px] truncate">{currentTask.text}</span>
            </>
          )}
        </div>
      }
      hasExpandableContent={todoItems.length > 0}
      isExpanded={isExpanded}
      onToggle={() => setIsExpanded((prev) => !prev)}
    >
      <div className="space-y-1.5 pb-2">
        {todoItems.map((todo, idx) => {
          const s = resolveStatus(todo);
          return (
            <div key={idx} className="flex items-start gap-2">
              <StatusIcon status={s} className="mt-0.5 shrink-0 h-3.5 w-3.5" />
              <span className={cn(s === "completed" && "line-through text-muted-foreground")}>
                {todo.text}
              </span>
            </div>
          );
        })}
        {snapshots.length > 0 && <SnapshotHistory snapshots={snapshots} />}
      </div>
    </ExpandableRow>
  );
}
