"use client";

import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { useStore } from "zustand";
import { RightSidePanel } from "@/components/right-side-panel";
import { useWorkspaceScope } from "@/components/workspace-scope-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import {
  createWorkspaceCopilotStore,
  type WorkspaceCopilotStore,
} from "@/hooks/domains/coordinator/workspace-copilot-store";
import { WorkspaceCopilotStoreProvider } from "@/hooks/domains/coordinator/workspace-copilot-context";
import { hasScope, SCOPE } from "@/lib/types/team-access";
import { useResolvedSpaRoute } from "@/src/spa-routes";
import { useCoordinatorList } from "../use-coordinator-list";
import {
  chooseCoordinator,
  clearLastUsed,
  readLastUsed,
  writeLastUsed,
} from "./coordinator-choice";
import {
  pageRouteKey,
  useActiveWorkflowId,
  usePageContext,
  type WorkspacePageRoute,
} from "./use-page-context";
import { useCopilotPanelWidth } from "./use-copilot-panel-width";
import { WorkspaceCopilotLauncher } from "./workspace-copilot-launcher";
import { WorkspaceCopilotPanel } from "./workspace-copilot-panel";

function toPageRoute(route: ReturnType<typeof useResolvedSpaRoute>): WorkspacePageRoute | null {
  if (route.kind === "kanban" || route.kind === "needsYouInbox") return { kind: route.kind };
  if (route.kind === "taskDetail") return { kind: "taskDetail", taskId: route.taskId };
  return null;
}

/** Coordinators the workspace no longer has, per workspace: a workspace change forgets them. */
function useGoneIds(workspaceId: string | null) {
  const [state, setState] = useState<{ workspaceId: string | null; ids: readonly string[] }>({
    workspaceId,
    ids: [],
  });
  const ids = state.workspaceId === workspaceId ? state.ids : [];
  const add = useCallback(
    (id: string) =>
      setState((prev) => {
        const base = prev.workspaceId === workspaceId ? prev.ids : [];
        return base.includes(id) ? prev : { workspaceId, ids: [...base, id] };
      }),
    [workspaceId],
  );
  return { ids, add };
}

function useHostEligibility() {
  const { workspace, workspaceId, mode } = useWorkspaceScope();
  const route = useResolvedSpaRoute();
  const pageRoute = toPageRoute(route);
  const canManage = hasScope(workspace?.scopes, SCOPE.workspaceManage);
  const onWorkspacePage =
    mode === "kanban" && workspaceId !== null && canManage && pageRoute !== null;

  const list = useCoordinatorList(onWorkspacePage ? workspaceId : null);
  const gone = useGoneIds(workspaceId);
  const coordinators = useMemo(
    () => list.coordinators?.filter((c) => !gone.ids.includes(c.id)),
    [list.coordinators, gone.ids],
  );
  const eligible = onWorkspacePage && coordinators !== undefined && coordinators.length > 0;
  return { workspaceId, route, pageRoute, list, gone, coordinators, eligible };
}

type Eligibility = ReturnType<typeof useHostEligibility>;

function usePanelActions(store: WorkspaceCopilotStore, e: Eligibility) {
  const { workspaceId, coordinators, list, gone } = e;
  const launcherRef = useRef<HTMLButtonElement>(null);
  const returnFocusRef = useRef(false);
  const open = useStore(store, (s) => s.open);

  const handleGone = useCallback(
    (id: string) => {
      if (workspaceId && readLastUsed(workspaceId) === id) clearLastUsed(workspaceId);
      gone.add(id);
      list.retry();
    },
    [workspaceId, gone, list],
  );

  const openPanel = useCallback(() => {
    if (!workspaceId || !coordinators) return;
    const chosen = chooseCoordinator(workspaceId, coordinators);
    if (!chosen) return;
    store.getState().openFor(workspaceId, chosen.id);
    list.retry();
  }, [store, workspaceId, coordinators, list]);

  const closePanel = useCallback(() => {
    returnFocusRef.current = true;
    const id = store.getState().coordinatorId;
    if (id) store.getState().setOpen(id, false);
  }, [store]);

  const switchTo = useCallback(
    (id: string) => {
      if (workspaceId) writeLastUsed(workspaceId, id);
      store.getState().switchTo(id);
    },
    [store, workspaceId],
  );

  useEffect(() => {
    if (open || !returnFocusRef.current) return;
    returnFocusRef.current = false;
    launcherRef.current?.focus();
  }, [open]);

  return { launcherRef, handleGone, openPanel, closePanel, switchTo };
}

type HostProps = { children: ReactNode };

/**
 * The workspace shell's copilot: a launcher and right panel around the page on
 * the board, task pages and the Inbox. Eligibility, in order: flags, kanban
 * workspace, `workspace.manage`, a workspace-page route, then a loaded
 * non-empty coordinator list; an earlier failure issues no request. It renders
 * the page as the panel's `main` at all times so the route tree never remounts.
 */
function EnabledHost({ children }: HostProps) {
  const { t } = useTranslation();
  const elig = useHostEligibility();
  const { workspaceId, route, pageRoute, coordinators, eligible } = elig;

  const [store] = useState(createWorkspaceCopilotStore);
  const open = useStore(store, (s) => s.open);
  const coordinatorId = useStore(store, (s) => s.coordinatorId);
  const { widthPx, updateWidth } = useCopilotPanelWidth();
  const { launcherRef, handleGone, openPanel, closePanel, switchTo } = usePanelActions(store, elig);

  useEffect(() => {
    const state = store.getState();
    if (!eligible || (state.workspaceId !== null && state.workspaceId !== workspaceId)) {
      state.reset();
    }
  }, [store, eligible, workspaceId]);

  const active =
    open && eligible ? (coordinators?.find((c) => c.id === coordinatorId) ?? null) : null;
  useEffect(() => {
    if (!open || !eligible || !workspaceId || active) return;
    const next = coordinators?.[0];
    if (next) store.getState().switchTo(next.id);
  }, [store, open, eligible, workspaceId, active, coordinators]);

  const activeWorkflowId = useActiveWorkflowId();
  const contextRoute: WorkspacePageRoute = pageRoute ?? { kind: "needsYouInbox" };
  const routeKey = pageRouteKey(contextRoute, activeWorkflowId);
  const context = usePageContext(contextRoute, workspaceId ?? "");
  useEffect(() => {
    store.getState().clearDismissalUnless(routeKey);
  }, [store, routeKey]);

  return (
    <WorkspaceCopilotStoreProvider store={store}>
      <RightSidePanel
        open={active !== null}
        onClose={closePanel}
        widthPx={widthPx}
        onWidthChange={updateWidth}
        backdropLabel={t("coordinator:copilotClose")}
        mainSizing="fluid"
        mobileFullScreen
        closeOnEscape
        panelTestId="workspace-copilot-panel"
        main={children}
      >
        {active && workspaceId && (
          <WorkspaceCopilotPanel
            key={active.id}
            store={store}
            workspaceId={workspaceId}
            coordinator={active}
            coordinators={coordinators ?? []}
            context={context}
            routeKey={routeKey}
            onClose={closePanel}
            onSwitch={switchTo}
            onGone={handleGone}
          />
        )}
      </RightSidePanel>
      {eligible && active === null && (
        <WorkspaceCopilotLauncher
          ref={launcherRef}
          onOpen={openPanel}
          aboveWalkthrough={route.kind === "taskDetail"}
        />
      )}
    </WorkspaceCopilotStoreProvider>
  );
}

export function WorkspaceCopilotHost({ children }: HostProps) {
  const coordinatorOn = useFeature("coordinator");
  const phase2On = useFeature("coordinatorPhase2");
  if (!coordinatorOn || !phase2On) return <>{children}</>;
  return <EnabledHost>{children}</EnabledHost>;
}
