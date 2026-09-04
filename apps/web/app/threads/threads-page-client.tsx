"use client";

import { useCallback, useEffect, useMemo } from "react";
import { useRouter, useSearchParams } from "@/lib/routing/client-router";
import { KanbanHeader } from "@/components/kanban/kanban-header";
import { ThreadsBoard } from "@/components/threads/threads-board";
import { ThreadsViewControls } from "@/components/threads/threads-view-controls";
import { useAppStore } from "@/components/state-provider";
import { useAllWorkflowSnapshots } from "@/hooks/domains/kanban/use-all-workflow-snapshots";
import { useKanbanDisplaySettings } from "@/hooks/use-kanban-display-settings";
import { useTaskListingView } from "@/hooks/use-task-listing-view";
import { linkToTask } from "@/lib/links";
import { resolveFocusedThreadId } from "@/lib/threads/active-threads";
import { useStableThreadOrder } from "@/lib/threads/stable-order";
import { DEFAULT_THREAD_VIEW } from "@/lib/state/slices/ui/thread-view-builtins";
import { queryThreadView } from "@/lib/threads/thread-view-query";
import { useKanbanRouteBootstrap } from "@/src/kanban-route";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";

type WorkspaceWorkflow = { id: string; workspaceId: string };

/**
 * Keep the derived deck scoped to the workspace that the route currently
 * names. Snapshot cleanup runs in an effect, so render-time filtering must not
 * trust the global map while a workspace transition is in progress.
 */
export function scopeSnapshotsToWorkspace(
  snapshots: Record<string, WorkflowSnapshotData>,
  workflows: readonly WorkspaceWorkflow[],
  workspaceId: string | null | undefined,
): Record<string, WorkflowSnapshotData> {
  if (!workspaceId) return {};
  const workflowIds = new Set(
    workflows
      .filter((workflow) => workflow.workspaceId === workspaceId)
      .map((workflow) => workflow.id),
  );
  return Object.fromEntries(
    Object.entries(snapshots).filter(([workflowId]) => workflowIds.has(workflowId)),
  );
}

/**
 * The Threads page: every live agent conversation in the workspace, side by
 * side. It reads the workflow snapshots the board already keeps in the store,
 * so switching to this view costs no extra request and stays live on the same
 * WebSocket updates the cards do.
 */
export function ThreadsPageClient() {
  const router = useRouter();
  const searchParams = useSearchParams();
  // Threads is reachable directly (bookmark, Home restore, a cross-workspace
  // link), so it owns the same workspace bootstrap the board does instead of
  // assuming the board already ran it. The requested workspace has to reach
  // the bootstrap: without it a `/threads?workspace=A` link silently loads
  // whichever workspace the cookie or saved setting last named.
  const requestedWorkspaceId = searchParams.get("workspace") ?? undefined;
  const bootstrapRoute = useMemo(
    () => ({ workspaceId: requestedWorkspaceId }),
    [requestedWorkspaceId],
  );
  useKanbanRouteBootstrap(bootstrapRoute, false);
  const { activeWorkspaceId, workflows, workspaces, repositories } = useKanbanDisplaySettings();
  const { setView } = useTaskListingView();
  const snapshots = useAppStore((state) => state.kanbanMulti.snapshots);
  const isLoading = useAppStore((state) => state.kanbanMulti.isLoading);
  const storedThreadViews = useAppStore((state) => state.threadViews);
  const threadViews = storedThreadViews ?? {
    views: [DEFAULT_THREAD_VIEW],
    activeViewId: DEFAULT_THREAD_VIEW.id,
    draft: null,
    syncError: null,
    orderResetGeneration: 0,
  };

  // Keep a valid deep-link workspace during the bootstrap transition, but use
  // the resolved active workspace when a stale or invalid link is supplied.
  const requestedWorkspaceIsKnown = requestedWorkspaceId
    ? workspaces.some((workspace) => workspace.id === requestedWorkspaceId)
    : false;
  const scopedWorkspaceId =
    requestedWorkspaceId && (workspaces.length === 0 || requestedWorkspaceIsKnown)
      ? requestedWorkspaceId
      : activeWorkspaceId;
  useAllWorkflowSnapshots(scopedWorkspaceId);

  const scopedSnapshots = useMemo(
    () => scopeSnapshotsToWorkspace(snapshots, workflows, scopedWorkspaceId),
    [snapshots, workflows, scopedWorkspaceId],
  );

  useEffect(() => {
    setView("threads");
  }, [setView]);

  const activeThreadView =
    threadViews.views.find((view) => view.id === threadViews.activeViewId) ?? DEFAULT_THREAD_VIEW;
  const requestedTaskId = searchParams.get("taskId");
  const query = useMemo(
    () =>
      queryThreadView(scopedSnapshots, activeThreadView, {
        workspaceId: scopedWorkspaceId,
        requestedTaskId,
        draft: threadViews.draft,
      }),
    [scopedSnapshots, scopedWorkspaceId, activeThreadView, requestedTaskId, threadViews.draft],
  );
  // Ranking decides where a column first appears; after that the slot is the
  // reader's, so replying to a thread cannot slide it across the deck.
  const threads = useStableThreadOrder(
    query.stableCandidates,
    `${query.fingerprint}:${requestedTaskId ?? ""}:${threadViews.orderResetGeneration}`,
    {
      resetThreads: query.admittedCandidates,
      maxItems: query.effectiveView.maxColumns,
    },
  );

  const handleOpenTask = useCallback((taskId: string) => router.push(linkToTask(taskId)), [router]);
  const handleInvalidRequestedSession = useCallback(
    (taskId: string, sessionId: string) => {
      if (searchParams.get("taskId") !== taskId || searchParams.get("sessionId") !== sessionId) {
        return;
      }
      const nextSearchParams = new URLSearchParams(searchParams.toString());
      nextSearchParams.delete("sessionId");
      const query = nextSearchParams.toString();
      router.replace(query ? `/threads?${query}` : "/threads", { scroll: false });
    },
    [router, searchParams],
  );

  // Resolved against the rendered deck rather than trusted from the URL: the
  // requested thread may have settled between the link being offered and
  // followed, and a focus id no column matches would ring nothing.
  const focusedTaskId = resolveFocusedThreadId(threads, searchParams.get("taskId"));
  const focusedSessionId = focusedTaskId ? searchParams.get("sessionId") : null;

  return (
    <div className="flex h-full min-h-0 w-full flex-col bg-background">
      <KanbanHeader
        workspaceId={activeWorkspaceId ?? undefined}
        currentPage="threads"
        taskListingControls={
          <ThreadsViewControls
            candidates={query.candidates}
            repositories={repositories}
            admittedCount={query.admittedCandidates.length}
            matchingCount={query.matchingCount + query.temporaryAdmissionCount}
            hiddenCount={query.hiddenCount}
          />
        }
      />
      <div className="min-h-0 flex-1">
        <ThreadsBoard
          threads={threads}
          isLoading={isLoading}
          focusedTaskId={focusedTaskId}
          focusedSessionId={focusedSessionId}
          onInvalidRequestedSession={handleInvalidRequestedSession}
          onOpenTask={handleOpenTask}
        />
      </div>
    </div>
  );
}
