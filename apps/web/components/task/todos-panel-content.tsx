"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useSessionMessages } from "@/hooks/domains/session/use-session-messages";
import { buildTodoItems } from "@/hooks/use-processed-messages";
import { TodoIndicatorContent, resolveStatus } from "./chat/todo-indicator";
import { useSessionTodoItems } from "./chat/use-chat-panel-state";

/**
 * Todos panel content shared by every Dockview registry that renders the
 * `todos` component (the desktop workbench's `dockview-panel-content.tsx`
 * and the Office workbench's `dockview-shared.tsx`). Extracted so the
 * message-history fallback, status semantics, and empty state can't drift
 * between the two copies — see docs/plans/agent-todo-list-panel/task-05-e2e-coverage.md
 * for the duplication this replaces.
 *
 * Sources the same live `sessionTodos.bySessionId` store slice `TodoIndicator`
 * uses, falling back to the latest persisted `todo`-type message
 * (`buildTodoItems`) for a session that already completed before this panel
 * mounted, since the live slice is populated exclusively by a WS handler and
 * is never backfilled from history.
 */
export function TodosContent() {
  const { t } = useTranslation();
  const sessionId = useAppStore((state) => state.tasks.activeSessionId);
  const { messages } = useSessionMessages(sessionId);
  const messageTodos = useMemo(() => buildTodoItems(messages), [messages]);
  const todos = useSessionTodoItems(sessionId, messageTodos);

  if (todos.length === 0) {
    return (
      <div data-testid="todos-panel-empty-state" className="p-4 text-sm text-muted-foreground">
        {t("chat:noTodosYet")}
      </div>
    );
  }

  const completed = todos.filter((todo) => resolveStatus(todo) === "completed").length;
  const progress = Math.round((completed / todos.length) * 100);
  return (
    <div data-testid="todos-panel" className="p-3">
      <TodoIndicatorContent todos={todos} completed={completed} progress={progress} />
    </div>
  );
}
