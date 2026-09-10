"use client";

import { useEffect, useMemo, useState } from "react";
import { useShallow } from "zustand/react/shallow";
import { PageClient } from "@/app/page-client";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { workspaceHomeHref } from "@/lib/navigation/workspace-home";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { fetchUserSettings } from "@/lib/api/domains/settings-api";
import { listWorkflows } from "@/lib/api/domains/kanban-api";
import { listRepositories, listWorkspaces } from "@/lib/api/domains/workspace-api";
import { resolveDesiredWorkflowId } from "@/lib/kanban/resolve-workflow";
import { hasHydratedKanbanRouteState } from "@/lib/routing/kanban-route-hydration";
import {
  mapWorkspaceItem,
  promoteLegacyWorkspaceSelection,
  readActiveWorkspaceCookie,
} from "@/lib/routing/route-bootstrap";
import { useRouter } from "@/lib/routing/client-router";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import { isOfficeWorkspace } from "@/lib/state/slices/workspace/selectors";
import type { WorkspaceState } from "@/lib/state/slices/workspace/types";
import type { Workflow } from "@/lib/types/http";

export type KanbanRouteSelection = {
  workspaceId?: string;
  workflowId?: string;
  taskId?: string;
  sessionId?: string;
};

/**
 * Honours an explicitly named workspace — route param, cookie, stored
 * preference — whatever its type, so an Office workspace named by any of them
 * reaches the store and the mismatch redirect below can act on it. Only the
 * no-signal fallback prefers a kanban workspace.
 *
 * Resolving explicit candidates against kanban workspaces alone is what made
 * "go Home from Office" silently reassign the active workspace: the office id
 * matched nothing, so the list fell through to some unrelated board.
 */
export function resolveKanbanRouteWorkspaceId(
  items: WorkspaceState["items"],
  ...preferredIds: (string | null | undefined)[]
): string | null {
  for (const id of preferredIds) {
    if (!id) continue;
    const match = items.find((workspace) => workspace.id === id);
    if (match) return match.id;
  }
  return items.find((workspace) => !workspace.office_workflow_id)?.id ?? items[0]?.id ?? null;
}

/**
 * The kanban board for a workspace that turns out to be an Office workspace is
 * a mismatch between the URL and the thing the URL is about. The workspace
 * wins: the URL goes to that workspace's Office home, rather than the app
 * quietly activating some other workspace whose board it can render.
 *
 * The check reads resolved store state rather than living inside the bootstrap
 * fetch, because a server-booted load hydrates kanban state up front and
 * `hasHydratedKanbanRouteState` then skips the bootstrap entirely — a check in
 * there would never fire on the common path.
 */
function useKanbanWorkspaceMismatchRedirect(route: KanbanRouteSelection): string | null {
  const router = useRouter();
  const officeEnabled = useFeature("office");
  const { items: workspaceItems, activeId: activeWorkspaceId } = useAppStore(
    useShallow((state) => state.workspaces),
  );
  const targetId = route.workspaceId ?? activeWorkspaceId;
  const targetWorkspace = workspaceItems.find((workspace) => workspace.id === targetId);
  // With the feature off there is no Office surface to send anyone to, so an
  // office workspace degrades to its kanban board instead.
  const redirectHref =
    officeEnabled && isOfficeWorkspace(targetWorkspace) ? workspaceHomeHref(targetWorkspace) : null;

  useEffect(() => {
    if (redirectHref) router.replace(redirectHref);
  }, [redirectHref, router]);

  return redirectHref;
}

/**
 * Hydrates workspaces, workflows and repositories for a board-backed route.
 * Exported because Threads renders the same workspace data from a different
 * arrangement, and a second bootstrap would be a second source of truth for
 * which workspace is active.
 */
export function useKanbanRouteBootstrap(route: KanbanRouteSelection, skip: boolean) {
  const store = useAppStoreApi();
  const selection = useMemo(
    () => ({ workspaceId: route.workspaceId, workflowId: route.workflowId }),
    [route.workspaceId, route.workflowId],
  );
  const [completedSelection, setCompletedSelection] = useState<typeof selection | null>(null);
  const hydrated = useAppStore(
    (state) => state.userSettings.loaded && hasHydratedKanbanRouteState(state, selection),
  );

  useEffect(() => {
    promoteLegacyWorkspaceSelection(store.getState().workspaces.items);
    if (skip) return;
    if (
      store.getState().userSettings.loaded &&
      hasHydratedKanbanRouteState(store.getState(), selection)
    ) {
      setCompletedSelection(selection);
      return;
    }

    let cancelled = false;

    async function bootstrap() {
      const [workspacesResponse, settingsResponse] = await Promise.all([
        listWorkspaces({ cache: "no-store" }).catch(() => ({ workspaces: [], total: 0 })),
        fetchUserSettings({ cache: "no-store" }).catch(() => null),
      ]);
      if (cancelled) return;

      const settingsWorkspaceId = settingsResponse?.settings?.workspace_id || null;
      const settingsWorkflowId = settingsResponse?.settings?.workflow_filter_id || null;
      const workspaceItems = workspacesResponse.workspaces.map(mapWorkspaceItem);
      promoteLegacyWorkspaceSelection(workspaceItems);
      const activeWorkspaceId = resolveKanbanRouteWorkspaceId(
        workspaceItems,
        selection.workspaceId,
        readActiveWorkspaceCookie(),
        settingsWorkspaceId,
      );

      store.getState().hydrate({
        workspaces: { items: workspaceItems, activeId: activeWorkspaceId },
        userSettings: {
          ...mapUserSettingsResponse(settingsResponse),
          workspaceId: activeWorkspaceId,
        },
      });

      if (!activeWorkspaceId) return;

      // The mismatch redirect above takes over once an Office workspace is
      // hydrated as active, so the board data below would be fetched for a
      // view that never renders.
      if (isOfficeWorkspace(workspaceItems.find((item) => item.id === activeWorkspaceId))) {
        return;
      }

      const [workflowsResponse, repositoriesResponse] = await Promise.all([
        listWorkflows(activeWorkspaceId, { cache: "no-store", includeHidden: true }).catch(() => ({
          workflows: [],
        })),
        listRepositories(activeWorkspaceId, undefined, { cache: "no-store" }).catch(() => ({
          repositories: [],
        })),
      ]);
      if (cancelled) return;

      const workflowId = resolveDesiredWorkflowId({
        activeWorkflowId: selection.workflowId ?? null,
        settingsWorkflowId,
        workspaceWorkflows: workflowsResponse.workflows,
      });

      store.getState().hydrate({
        userSettings: {
          ...mapUserSettingsResponse(settingsResponse),
          workspaceId: activeWorkspaceId,
          workflowId,
        },
        workflows: {
          items: workflowsResponse.workflows.map(mapWorkflowItem),
          activeId: workflowId,
        },
      });
      store.getState().setRepositories(activeWorkspaceId, repositoriesResponse.repositories);
    }

    void bootstrap().then(() => {
      if (!cancelled) setCompletedSelection(selection);
    });
    return () => {
      cancelled = true;
    };
  }, [selection, skip, store]);

  // Completion is tied to this route request, including empty and failed fetches.
  return hydrated || completedSelection === selection;
}

function mapWorkflowItem(workflow: Workflow) {
  return {
    id: workflow.id,
    workspaceId: workflow.workspace_id,
    name: workflow.name,
    description: workflow.description ?? null,
    sortOrder: workflow.sort_order ?? 0,
    ...(workflow.agent_profile_id ? { agent_profile_id: workflow.agent_profile_id } : {}),
    ...(workflow.hidden !== undefined ? { hidden: workflow.hidden } : {}),
    ...(workflow.style !== undefined ? { style: workflow.style } : {}),
  };
}

export function KanbanRoute({
  route,
  fallback,
}: {
  route: KanbanRouteSelection;
  /** Shown while the mismatch redirect above is in flight. */
  fallback: React.ReactNode;
}) {
  const redirectHref = useKanbanWorkspaceMismatchRedirect(route);
  const ready = useKanbanRouteBootstrap(route, redirectHref !== null);
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);

  if (redirectHref || !ready) return <>{fallback}</>;

  return (
    <PageClient
      workspaceId={activeWorkspaceId ?? undefined}
      initialTaskId={route.taskId}
      initialSessionId={route.sessionId}
    />
  );
}
