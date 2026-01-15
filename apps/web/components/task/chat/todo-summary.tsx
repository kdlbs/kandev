'use client';

import { IconCheck, IconChevronDown } from '@tabler/icons-react';
import { cn } from '@/lib/utils';

type TodoItem = {
  text: string;
  done?: boolean;
};

type TodoSummaryProps = {
  todos: TodoItem[];
};

export function TodoSummary({ todos }: TodoSummaryProps) {
  if (!todos.length) return null;

  const completed = todos.filter((todo) => todo.done).length;

  return (
    <div className="rounded-md border border-border/60 bg-muted/40 px-3 py-2 text-xs">
      <div className="flex items-center justify-between text-[11px] uppercase tracking-wide text-muted-foreground">
        <span>Todos ({todos.length})</span>
        <IconChevronDown className="h-3.5 w-3.5" />
      </div>
      <div className="mt-2 space-y-1">
        {todos.map((todo) => (
          <div key={todo.text} className="flex items-center gap-2">
            {todo.done ? (
              <IconCheck className="h-3.5 w-3.5 text-green-500" />
            ) : (
              <span className="h-3.5 w-3.5" />
            )}
            <span className={cn(todo.done && 'line-through text-muted-foreground')}>
              {todo.text}
            </span>
          </div>
        ))}
      </div>
      <div className="mt-2 text-[11px] text-muted-foreground">
        {completed} of {todos.length} completed
      </div>
    </div>
  );
}
