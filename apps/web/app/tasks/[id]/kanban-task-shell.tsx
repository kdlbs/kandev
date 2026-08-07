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

import Link from "@/components/routing/app-link";
import { TaskPageContent } from "@/components/task/task-page-content";
import { TaskBody, resolveTaskBodyMode } from "@/components/task/TaskBody";
import { TaskHeader } from "@/components/task/TaskHeader";
import { useTaskPendingInput } from "@/hooks/use-task-pending-input";
import { TaskStateActions } from "@/components/task/task-state-actions";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { isFromOffice } from "@/lib/types/http";
import type { Repository, RepositoryScript, Task } from "@/lib/types/http";
import type { Terminal } from "@/hooks/domains/session/use-terminals";
import type { Layout } from "react-resizable-panels";
import { useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";

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
  const { t } = useTranslation();
  // Kanban shell defaults to advanced. ?simple flips to simple.
  const mode = resolveTaskBodyMode({ simple: urlSimple, mode: urlMode }, "advanced");
  // "Open in office view" only makes sense when (a) the office feature is
  // enabled, and (b) the task actually exists in office (has a project).
  // Kanban-origin tasks have no office row, so the link would 404.
  const officeEnabled = useFeature("office");
  const showOfficeLink = officeEnabled && isFromOffice(task);

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
      officeTaskHref={showOfficeLink ? `/office/tasks/${taskId}` : null}
    />
  );

  const simpleSlot = (
    <div className="flex h-full min-h-0 w-full flex-col overflow-y-auto bg-background p-6">
      {showOfficeLink && <CrossLinkRow taskId={taskId} target="office" />}
      <div className="mt-4 max-w-3xl">
        <SimpleTaskHeaderRow task={task} />
        <p className="mt-4 text-sm text-muted-foreground">
          {showOfficeLink
            ? t("tasks:simpleViewForKanbanTasksShows")
            : t("tasks:simpleViewShowsTheChatThat", { simpleQuery: "?simple=false" })}
        </p>
      </div>
    </div>
  );

  if (mode === "advanced") {
    return <TaskBody mode={mode} simpleSlot={simpleSlot} advancedSlot={advancedSlot} />;
  }

  return <TaskBody mode={mode} simpleSlot={simpleSlot} advancedSlot={advancedSlot} />;
}

// Open-task header row for the kanban simple view: a task-level status icon plus
// the shared TaskHeader. Both reflect the MOST-ACTIVE-WINS activity aggregate so a
// background-running task reads distinctly and never as done,
// and carry the sidebar's rich "needs me" reading — pending clarification /
// permission — so the header distinguishes waiting-for-input.
function simpleTaskHeaderData(task: Task | null) {
  return {
    primarySessionId: task?.primary_session_id,
    pendingFallback: {
      taskId: task?.id,
      taskPendingAction: task?.task_pending_action,
      primarySessionState: task?.primary_session_state,
      primarySessionPendingAction: task?.primary_session_pending_action,
    },
    identifier: task?.id?.slice(0, 8),
    title: task?.title ?? t("tasks:loading"),
    state: task?.state ?? null,
    foregroundActivity: task?.foreground_activity,
    interrupted: task?.interrupted ?? false,
  };
}

function SimpleTaskHeaderRow({ task }: { task: Task | null }) {
  const data = simpleTaskHeaderData(task);
  const pendingInput = useTaskPendingInput(data.primarySessionId, data.pendingFallback);
  return (
    <div className="flex items-center gap-2">
      <TaskStateActions
        state={data.state ?? undefined}
        className="shrink-0"
        foregroundActivity={data.foregroundActivity}
        hasPendingClarification={pendingInput.clarification}
        hasPendingPermission={pendingInput.permission}
        interrupted={data.interrupted}
      />
      <TaskHeader
        identifier={data.identifier}
        title={data.title}
        state={data.state}
        foregroundActivity={data.foregroundActivity}
        hasPendingClarification={pendingInput.clarification}
        hasPendingPermission={pendingInput.permission}
      />
    </div>
  );
}

function CrossLinkRow({ taskId, target }: { taskId: string; target: "office" | "kanban" }) {
  const { t } = useTranslation();
  const href = target === "office" ? `/office/tasks/${taskId}` : `/t/${taskId}`;
  const label = target === "office" ? t("tasks:openInOfficeView") : t("tasks:openInAdvancedView");
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
