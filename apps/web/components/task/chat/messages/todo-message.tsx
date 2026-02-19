"use client";

import { IconListCheck } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import type { Message } from "@/lib/types/http";
import type { RichMetadata } from "@/components/task/chat/types";

export function TodoMessage({ comment }: { comment: Message }) {
  const metadata = comment.metadata as RichMetadata | undefined;
  const todos = metadata?.todos ?? [];
  const todoItems = todos
    .map((item) => (typeof item === "string" ? { text: item, done: false } : item))
    .filter((item) => item.text);

  if (!todoItems.length) {
    return null;
  }

  const completed = todoItems.filter((todo) => todo.done).length;
  const progress = Math.round((completed / todoItems.length) * 100);

  return (
    <div className="w-full rounded-lg border border-border/60 bg-background/60 px-4 py-3 text-xs">
      <div className="flex items-center gap-2 text-muted-foreground mb-2 uppercase tracking-wide text-[11px]">
        <IconListCheck className="h-3.5 w-3.5" />
        <span>Todos</span>
      </div>
      <div className="mb-2">
        <div className="flex items-center justify-between text-[11px] text-muted-foreground mb-1">
          <span>
            {completed} of {todoItems.length} done
          </span>
          <span>{progress}%</span>
        </div>
        <div className="h-1.5 rounded-full bg-muted/70">
          <div
            className="h-full rounded-full bg-primary/80 transition-[width]"
            style={{ width: `${progress}%` }}
          />
        </div>
      </div>
      <div className="space-y-1 text-xs">
        {todoItems.map((todo) => (
          <div key={todo.text} className="flex items-center gap-2">
            <span
              className={cn(
                "h-1.5 w-1.5 rounded-full",
                todo.done ? "bg-green-500" : "bg-muted-foreground/60",
              )}
            />
            <span className={cn(todo.done && "line-through text-muted-foreground")}>
              {todo.text}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}
