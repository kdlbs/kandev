"use client";

/**
 * KanbanTaskShell — kanban shell wrapper for /t/:taskId.
 *
 * Resolves the body mode from URL params (advanced is the default for
 * this route), renders the existing TaskPageContent for the advanced
 * path, and exposes a cross-link to the office shell.
 *
 * The simple path is a graceful fallback: kanban tasks don't have the
 * full office data (decisions, project, run-status badges), so the
 * simple pane just shows a minimal "use ?simple to flip back" hint
 * while pointing at the office shell where the simple view lives in
 * its full form. Per the prompt: don't over-design the kanban-simple
 * path.
 */

import Link from "next/link";
import { TaskPageContent } from "@/components/task/task-page-content";
import { TaskBody, resolveTaskBodyMode } from "@/components/task/TaskBody";
import { TaskHeader } from "@/components/task/TaskHeader";
import type { Repository, RepositoryScript, Task } from "@/lib/types/http";
import type { Terminal } from "@/hooks/domains/session/use-terminals";
import type { Layout } from "react-resizable-panels";

type KanbanTaskShellProps = {
  task: Task | null;
  taskId: string;
  sessionId: string | null;
  initialRepositories: Repository[];
  initialScripts: RepositoryScript[];
  initialTerminals?: Terminal[];
  defaultLayouts: Record<string, Layout>;
  initialLayout?: string | null;
  urlSimple?: string;
  urlMode?: string;
};

export function KanbanTaskShell({
  task,
  taskId,
  sessionId,
  initialRepositories,
  initialScripts,
  initialTerminals,
  defaultLayouts,
  initialLayout,
  urlSimple,
  urlMode,
}: KanbanTaskShellProps) {
  // Kanban shell defaults to advanced. ?simple flips to simple.
  const mode = resolveTaskBodyMode({ simple: urlSimple, mode: urlMode }, "advanced");

  const advancedSlot = (
    <TaskPageContent
      task={task}
      taskId={taskId}
      sessionId={sessionId}
      initialRepositories={initialRepositories}
      initialScripts={initialScripts}
      initialTerminals={initialTerminals}
      defaultLayouts={defaultLayouts}
      initialLayout={initialLayout}
      officeTaskHref={`/office/tasks/${taskId}`}
    />
  );

  const simpleSlot = (
    <div className="flex h-screen w-full flex-col overflow-y-auto bg-background p-6">
      <CrossLinkRow taskId={taskId} target="office" />
      <div className="mt-4 max-w-3xl">
        <TaskHeader
          identifier={task?.id?.slice(0, 8)}
          title={task?.title ?? "Loading..."}
          state={task?.state ?? null}
        />
        <p className="mt-4 text-sm text-muted-foreground">
          Simple view for kanban tasks shows the chat that&apos;s already in the panels. For the
          full Linear-style experience (comments, properties, activity timeline), open this task in
          the office view.
        </p>
      </div>
    </div>
  );

  if (mode === "advanced") {
    return <TaskBody mode={mode} simpleSlot={simpleSlot} advancedSlot={advancedSlot} />;
  }

  return <TaskBody mode={mode} simpleSlot={simpleSlot} advancedSlot={advancedSlot} />;
}

function CrossLinkRow({ taskId, target }: { taskId: string; target: "office" | "kanban" }) {
  const href = target === "office" ? `/office/tasks/${taskId}` : `/t/${taskId}`;
  const label = target === "office" ? "Open in office view" : "Open in advanced view";
  return (
    <Link
      href={href}
      className="text-xs text-muted-foreground underline-offset-2 hover:underline cursor-pointer"
      data-testid="task-cross-link"
    >
      {label}
    </Link>
  );
}
