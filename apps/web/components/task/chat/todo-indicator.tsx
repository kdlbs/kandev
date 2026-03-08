"use client";

import { IconCheck, IconCircle, IconCircleFilled, IconListCheck, IconX } from "@tabler/icons-react";
import { HoverCard, HoverCardTrigger, HoverCardContent } from "@kandev/ui/hover-card";
import { cn } from "@/lib/utils";

type TodoDisplayItem = {
  text: string;
  done?: boolean;
  status?: "pending" | "in_progress" | "completed" | "failed";
};

type TodoIndicatorProps = {
  todos: TodoDisplayItem[];
};

function resolveStatus(todo: TodoDisplayItem): "pending" | "in_progress" | "completed" | "failed" {
  if (todo.status) return todo.status;
  return todo.done ? "completed" : "pending";
}

function StatusIcon({ status, className }: { status: string; className?: string }) {
  switch (status) {
    case "completed":
      return <IconCheck className={cn("h-3.5 w-3.5 text-green-500", className)} />;
    case "in_progress":
      return <IconCircleFilled className={cn("h-3.5 w-3.5 text-blue-500", className)} />;
    case "failed":
      return <IconX className={cn("h-3.5 w-3.5 text-red-500", className)} />;
    default:
      return <IconCircle className={cn("h-3.5 w-3.5 text-muted-foreground/40", className)} />;
  }
}

export function TodoIndicator({ todos }: TodoIndicatorProps) {
  if (!todos.length) return null;

  const completed = todos.filter((t) => resolveStatus(t) === "completed").length;
  const allComplete = completed === todos.length;
  const progress = Math.round((completed / todos.length) * 100);

  return (
    <HoverCard openDelay={200} closeDelay={100}>
      <HoverCardTrigger asChild>
        <button
          type="button"
          className={cn(
            "flex items-center gap-1.5 px-2 py-0.5 text-xs transition-colors rounded cursor-pointer",
            allComplete
              ? "text-green-500 hover:text-green-400"
              : "text-muted-foreground hover:text-foreground",
          )}
        >
          {allComplete ? <IconCheck className="h-3 w-3" /> : <IconListCheck className="h-3 w-3" />}
          <span>
            {completed}/{todos.length}
          </span>
        </button>
      </HoverCardTrigger>
      <HoverCardContent side="top" align="start" className="w-72 p-3">
        <div className="flex items-center justify-between text-xs mb-2">
          <span className="font-medium text-foreground">Todos</span>
          <span className="text-muted-foreground">
            {completed}/{todos.length} completed
          </span>
        </div>
        <div className="h-1.5 rounded-full bg-muted/70 mb-3">
          <div
            className="h-full rounded-full bg-primary/80 transition-[width]"
            style={{ width: `${progress}%` }}
          />
        </div>
        <div className="space-y-1.5 max-h-48 overflow-y-auto">
          {todos.map((todo, idx) => {
            const s = resolveStatus(todo);
            return (
              <div key={idx} className="flex items-start gap-2 text-xs">
                <StatusIcon status={s} className="mt-0.5 shrink-0 h-3 w-3" />
                <span className={cn(s === "completed" && "line-through text-muted-foreground")}>
                  {todo.text}
                </span>
              </div>
            );
          })}
        </div>
      </HoverCardContent>
    </HoverCard>
  );
}

export { StatusIcon, resolveStatus };
export type { TodoDisplayItem };
