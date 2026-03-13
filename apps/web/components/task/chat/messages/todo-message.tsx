"use client";

import { useState } from "react";
import { IconListCheck } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import type { Message } from "@/lib/types/http";
import type { RichMetadata } from "@/components/task/chat/types";
import { ExpandableRow } from "./expandable-row";
import { StatusIcon, resolveStatus } from "../todo-indicator";

type TodoItem = {
  text: string;
  done?: boolean;
  status?: "pending" | "in_progress" | "completed" | "failed";
};

function parseTodos(comment: Message): TodoItem[] {
  const metadata = comment.metadata as RichMetadata | undefined;
  const todos = metadata?.todos ?? [];
  return todos
    .map((item) => (typeof item === "string" ? { text: item, done: false } : item))
    .filter((item) => item.text);
}

export function TodoMessage({
  comment,
  defaultExpanded = false,
}: {
  comment: Message;
  defaultExpanded?: boolean;
}) {
  const todoItems = parseTodos(comment);
  const [isExpanded, setIsExpanded] = useState(defaultExpanded);

  if (!todoItems.length) return null;

  const completed = todoItems.filter((t) => resolveStatus(t) === "completed").length;
  const currentTask = todoItems.find((t) => resolveStatus(t) === "in_progress");

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
      </div>
    </ExpandableRow>
  );
}
