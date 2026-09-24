"use client";

import { useCallback, useEffect, useRef } from "react";
import type { DockviewApi } from "dockview-react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import type { Canvas } from "@/lib/api/domains/canvas-api";
import {
  getCanvasLifecycleHints,
  type CanvasLifecycleHint,
  useCanvasLifecycleRevision,
} from "@/lib/canvas-lifecycle";
import {
  canvasPresentationUserId,
  recordCanvasPresentation,
  type CanvasPresentationIdentity,
  type CanvasPresentationReason,
  wasCanvasPresented,
} from "@/lib/canvas-presentation-storage";
import { useRouter, type AppRouter } from "@/lib/routing/client-router";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { fallbackGroupPosition, focusOrAddPanel } from "@/lib/state/dockview-layout-builders";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";
import {
  useTaskCanvasesState,
  type TaskCanvasesLoadStatus,
} from "@/hooks/domains/task/use-task-canvases";
import type { AppState } from "@/lib/state/store";
import {
  readCanvasInventory,
  reconcileTaskCanvasCandidates,
  type CanvasInventoryRef,
  type DockviewLayoutSnapshot,
  type ProvidedCanvasInventoryRef,
  type ReconcileRef,
} from "./dockview-canvas-reconciliation";

const CANVAS_PANEL_ID_PREFIX = "canvas:";
const CANVAS_ACTIVATION_RETRY_DELAY_MS = 500;
const MAX_CANVAS_ACTIVATION_RETRIES = 5;
const CANVAS_RELEASE_ACTIONS = new Set<CanvasLifecycleHint["action"]>([
  "canvas.release.activated",
  "canvas.release.permission_required",
]);
const DISCOVERABLE_TASK_CANVAS_STATUSES = new Set(["active", "pending", "error"]);

export function isDiscoverableTaskCanvas(canvas: Pick<Canvas, "status">): boolean {
  return DISCOVERABLE_TASK_CANVAS_STATUSES.has(canvas.status);
}

export function shouldActivateCanvasForTask(
  hint: CanvasLifecycleHint,
  taskId: string,
  workspaceId: string,
): boolean {
  return (
    CANVAS_RELEASE_ACTIONS.has(hint.action) &&
    hint.payload.canvas_id.length > 0 &&
    hint.payload.task_id === taskId &&
    (!hint.payload.workspace_id || hint.payload.workspace_id === workspaceId)
  );
}

function canvasPanelId(canvasId: string): string {
  return `${CANVAS_PANEL_ID_PREFIX}${canvasId}`;
}

function canvasPanelGroupId(api: DockviewApi): string | undefined {
  return fallbackGroupPosition(api)?.referenceGroup;
}

function readDockviewLayout(): DockviewLayoutSnapshot {
  const state = useDockviewStore.getState();
  return {
    api: state.api,
    isRestoringLayout: state.isRestoringLayout,
    currentLayoutEnvId: state.currentLayoutEnvId,
  };
}

function layoutOwnsTask(layout: DockviewLayoutSnapshot, taskEnvironmentId: string | null): boolean {
  return (
    layout.api !== null &&
    !layout.isRestoringLayout &&
    taskEnvironmentId !== null &&
    layout.currentLayoutEnvId === taskEnvironmentId
  );
}

export function isTaskCanvasPresentationEligible(
  canvas: Canvas,
  taskId: string,
  workspaceId: string,
): boolean {
  if (
    !isDiscoverableTaskCanvas(canvas) ||
    canvas.scope_kind !== "task" ||
    canvas.task_id !== taskId ||
    canvas.workspace_id !== workspaceId
  ) {
    return false;
  }
  if (
    canvas.status === "active" &&
    canvas.active_release_id &&
    canvas.active_release_status === "valid"
  ) {
    return true;
  }
  return (
    (canvas.status === "pending" || canvas.status === "active") &&
    (canvas.pending_release?.validation_status === "pending_permission" ||
      (!canvas.pending_release && canvas.active_release_status === "pending_permission"))
  );
}

function compareCanvasCreation(a: Canvas, b: Canvas): number {
  const aCreated = parseStrictRfc3339Timestamp(a.created_at);
  const bCreated = parseStrictRfc3339Timestamp(b.created_at);
  if (aCreated !== null && bCreated !== null && aCreated !== bCreated) {
    return aCreated < bCreated ? -1 : 1;
  }
  if (aCreated !== null && bCreated === null) return -1;
  if (aCreated === null && bCreated !== null) return 1;
  return a.id.localeCompare(b.id);
}

export function sortTaskCanvasPresentationCandidates(canvases: readonly Canvas[]): Canvas[] {
  return [...canvases].sort(compareCanvasCreation);
}

function canvasIdentity(
  identity: Omit<CanvasPresentationIdentity, "canvasId">,
  canvasId: string,
): CanvasPresentationIdentity {
  return { ...identity, canvasId };
}

export type CanvasPanelPresentationOptions = {
  focus?: boolean;
  focusExisting?: boolean;
  presentation?: {
    identity: CanvasPresentationIdentity;
    reason: CanvasPresentationReason;
  };
};

type CanvasLifecycleActivationDecision = "eligible" | "retry" | "stale";

function canvasMatchesLifecycleHint(hint: CanvasLifecycleHint, canvas: Canvas): boolean {
  return !(
    (hint.payload.workspace_id && canvas.workspace_id !== hint.payload.workspace_id) ||
    canvas.task_id !== hint.payload.task_id ||
    canvas.scope_kind !== "task"
  );
}

function activatedCanvasDecision(
  hint: CanvasLifecycleHint,
  canvas: Canvas,
): CanvasLifecycleActivationDecision {
  if (canvas.status !== "active") return "stale";
  if (!canvas.active_release_id || !canvas.active_release_status) return "retry";
  if (canvas.active_release_status !== "valid") return "stale";
  if (
    hint.payload.active_release_id &&
    hint.payload.active_release_id !== canvas.active_release_id
  ) {
    return "stale";
  }
  return "eligible";
}

function permissionCanvasDecision(canvas: Canvas): CanvasLifecycleActivationDecision {
  // A first release leaves the instance pending until its permission review
  // is approved. Existing canvases stay active while a replacement release
  // waits for review. Both states are host-eligible; archived, disabled,
  // error, and removed instances must never be opened from a retained hint.
  if (canvas.status !== "pending" && canvas.status !== "active") return "stale";
  if (canvas.pending_release?.validation_status === "pending_permission") {
    return "eligible";
  }
  if (canvas.pending_release) return "stale";
  if (canvas.active_release_status === "pending_permission") return "eligible";
  if (canvas.active_release_status === "valid") return "stale";
  return "retry";
}

/**
 * Lifecycle events are retained as hints and can outlive a rejection, archive,
 * or task switch. The HTTP projection is the authority for deciding if an
 * iframe may be opened. Keep this decision pure so every host surface tests
 * the same release/lifecycle rules.
 */
export function canvasLifecycleActivationDecision(
  hint: CanvasLifecycleHint,
  canvas: Canvas,
): CanvasLifecycleActivationDecision {
  if (!canvasMatchesLifecycleHint(hint, canvas)) return "stale";
  if (hint.action === "canvas.release.activated") {
    return activatedCanvasDecision(hint, canvas);
  }

  if (hint.action === "canvas.release.permission_required") {
    return permissionCanvasDecision(canvas);
  }

  return "stale";
}

/** Add a canvas panel exactly once. Existing panels are deliberately left in
 * their current focus state when a repeated lifecycle event arrives. */
export function activateCanvasPanel(
  api: DockviewApi,
  canvas: Pick<Canvas, "id" | "title">,
  groupId?: string,
  options: CanvasPanelPresentationOptions = {},
): boolean {
  const id = canvasPanelId(canvas.id);
  const existing = api.getPanel(id);
  if (existing) {
    if (options.focusExisting) existing.api.setActive();
    if (options.presentation) {
      recordCanvasPresentation(options.presentation.identity, options.presentation.reason);
    }
    return false;
  }

  focusOrAddPanel(
    api,
    {
      id,
      component: "canvas",
      title: canvas.title,
      params: { canvasId: canvas.id },
      ...(groupId ? { position: { referenceGroup: groupId } } : {}),
    },
    options.focus === false,
  );
  if (options.presentation) {
    recordCanvasPresentation(options.presentation.identity, options.presentation.reason);
  }
  return true;
}

export function reconcileTaskCanvasPanels(
  api: DockviewApi,
  canvases: readonly Canvas[],
  identity: Omit<CanvasPresentationIdentity, "canvasId">,
): Canvas[] {
  const candidates = sortTaskCanvasPresentationCandidates(canvases);
  const groupId = canvasPanelGroupId(api);
  const newCanvases = candidates.filter((canvas) => !api.getPanel(canvasPanelId(canvas.id)));
  const added: Canvas[] = [];
  newCanvases.forEach((canvas, index) => {
    try {
      if (
        activateCanvasPanel(api, canvas, groupId, {
          focus: index === newCanvases.length - 1,
          presentation: {
            identity: canvasIdentity(identity, canvas.id),
            reason: "automatic",
          },
        })
      ) {
        added.push(canvas);
      }
    } catch {
      // A panel that fails to insert has not been presented and remains eligible.
    }
  });
  const addedIds = new Set(added.map((canvas) => canvas.id));
  candidates.forEach((canvas) => {
    if (!addedIds.has(canvas.id) && api.getPanel(canvasPanelId(canvas.id))) {
      recordCanvasPresentation(canvasIdentity(identity, canvas.id), "restored");
    }
  });
  return added;
}

export type MobileCanvasPresentationDecision = {
  canvas: Canvas;
  offered: Canvas[];
};

export function selectMobileCanvasPresentation(
  canvases: readonly Canvas[],
  identity: Omit<CanvasPresentationIdentity, "canvasId">,
): MobileCanvasPresentationDecision | null {
  const unseen = sortTaskCanvasPresentationCandidates(canvases).filter(
    (canvas) => !wasCanvasPresented(canvasIdentity(identity, canvas.id)),
  );
  const canvas = unseen.at(-1);
  return canvas ? { canvas, offered: unseen } : null;
}

const handledHints = new Set<string>();
const MAX_HANDLED_HINTS = 128;

function hintKey(hint: CanvasLifecycleHint, taskId: string): string {
  return `${hint.revision}:${taskId}:${hint.payload.canvas_id}`;
}

function isHandled(hint: CanvasLifecycleHint, taskId: string): boolean {
  return handledHints.has(hintKey(hint, taskId));
}

function markHandled(hint: CanvasLifecycleHint, taskId: string): void {
  const key = hintKey(hint, taskId);
  if (handledHints.has(key)) return;
  handledHints.add(key);
  if (handledHints.size > MAX_HANDLED_HINTS) {
    const oldest = handledHints.values().next().value;
    if (oldest) handledHints.delete(oldest);
  }
}

type CanvasLifecycleActivationProps = {
  taskId: string | null;
  workspaceId: string | null;
  sessionId?: string | null;
  isMobile: boolean;
  userId?: string | null;
  taskCanvases?: readonly Canvas[];
  taskCanvasesStatus?: TaskCanvasesLoadStatus;
};

type CanvasActivationEffectOptions = {
  enabled: boolean;
  taskId: string | null;
  workspaceId: string | null;
  sessionId?: string | null;
  taskEnvironmentId: string | null;
  isMobile: boolean;
  userId?: string | null;
  defaultUserId: string | null;
  layoutApi: DockviewApi | null;
  layoutRestoring: boolean;
  layoutEnvId: string | null;
  revision: number;
  router: AppRouter;
  inventoryRef: CanvasInventoryRef;
  providedInventoryRef: ProvidedCanvasInventoryRef;
  readTaskEnvironmentId: () => string | null;
  inFlightRef: { current: Set<string> };
  generationRef: { current: number };
  mobileNavigationRef: { current: string | null };
  reconcileRef: ReconcileRef;
};

function useCanvasActivationEffect(options: CanvasActivationEffectOptions): void {
  useEffect(() => {
    const { taskId, workspaceId } = options;
    if (!options.enabled || !taskId || !workspaceId) return;
    const presentationUserId =
      options.userId === undefined ? options.defaultUserId : options.userId;
    if (!presentationUserId) return;

    const generation = ++options.generationRef.current;
    let disposed = false;
    const retryTimers = new Set<ReturnType<typeof setTimeout>>();
    const isCurrent = () => !disposed && options.generationRef.current === generation;
    const identity = { userId: presentationUserId, workspaceId, taskId };
    const retryReconcile: ReconcileRef = { current: null };
    const scheduleRetry = (hint: CanvasLifecycleHint, attempt: number) => {
      if (!isCurrent() || attempt >= MAX_CANVAS_ACTIVATION_RETRIES) return;
      const timer = setTimeout(() => {
        retryTimers.delete(timer);
        retryReconcile.current?.([hint], attempt + 1);
      }, CANVAS_ACTIVATION_RETRY_DELAY_MS);
      retryTimers.add(timer);
    };
    const runReconcile = (hints: CanvasLifecycleHint[], attempt: number) => {
      void reconcileTaskCanvasCandidates(
        {
          hints,
          attempt,
          generation,
          taskId,
          workspaceId,
          identity,
          readTaskEnvironmentId: options.readTaskEnvironmentId,
          readDockviewLayout,
          isMobile: options.isMobile,
          router: options.router,
          mobileNavigationRef: options.mobileNavigationRef,
          inventoryRef: options.inventoryRef,
          providedInventoryRef: options.providedInventoryRef,
          inFlight: options.inFlightRef.current,
          isCurrent,
          scheduleRetry,
        },
        {
          hintKey,
          isHandled,
          markHandled,
          layoutOwnsTask,
          isTaskCanvasPresentationEligible,
          sortTaskCanvasPresentationCandidates,
          canvasIdentity,
          canvasLifecycleActivationDecision,
          selectMobileCanvasPresentation,
          reconcileTaskCanvasPanels,
        },
      );
    };
    retryReconcile.current = runReconcile;
    options.reconcileRef.current = runReconcile;
    runReconcile(
      getCanvasLifecycleHints().filter((hint) =>
        shouldActivateCanvasForTask(hint, taskId, workspaceId),
      ),
      0,
    );

    return () => {
      disposed = true;
      options.generationRef.current += 1;
      if (options.reconcileRef.current === runReconcile) options.reconcileRef.current = null;
      retryTimers.forEach((timer) => clearTimeout(timer));
      retryTimers.clear();
    };
  }, [
    options.defaultUserId,
    options.enabled,
    options.isMobile,
    options.layoutApi,
    options.layoutEnvId,
    options.layoutRestoring,
    options.readTaskEnvironmentId,
    options.revision,
    options.router,
    options.sessionId,
    options.taskEnvironmentId,
    options.taskId,
    options.userId,
    options.workspaceId,
  ]);
}

function resolveTaskEnvironmentId(
  state: Pick<AppState, "environmentIdBySessionId" | "taskSessions" | "taskSessionsByTask">,
  taskId: string | null,
  sessionId: string | null | undefined,
): string | null {
  if (sessionId) {
    const mapped = state.environmentIdBySessionId[sessionId];
    if (mapped) return mapped;
    const sessionEnvironment = state.taskSessions.items[sessionId]?.task_environment_id;
    if (sessionEnvironment) return sessionEnvironment;
  }

  const sessions = taskId ? (state.taskSessionsByTask.itemsByTaskId[taskId] ?? []) : [];
  const candidates = sessionId ? sessions.filter((session) => session.id === sessionId) : sessions;
  return candidates.find((session) => session.task_environment_id)?.task_environment_id ?? null;
}

/** React to retained or newly-arrived release events for the active task.
 * Metadata remains authoritative, so the event only identifies which canvas
 * to refetch and open. */
export function useTaskCanvasLifecycleActivation({
  taskId,
  workspaceId,
  sessionId,
  isMobile,
  userId,
  taskCanvases,
  taskCanvasesStatus,
}: CanvasLifecycleActivationProps): void {
  const enabled = useFeature("canvases");
  const revision = useCanvasLifecycleRevision();
  const inventory = useTaskCanvasesState(
    taskId,
    workspaceId,
    enabled && taskCanvases === undefined,
  );
  const api = useDockviewStore((state) => state.api);
  const isRestoringLayout = useDockviewStore((state) => state.isRestoringLayout);
  const currentLayoutEnvId = useDockviewStore((state) => state.currentLayoutEnvId);
  const defaultUserId = useAppStore((state) => canvasPresentationUserId(state.auth));
  const appStoreApi = useAppStoreApi();
  const taskEnvironmentId = useAppStore((state) =>
    resolveTaskEnvironmentId(state, taskId, sessionId),
  );
  const readTaskEnvironmentId = useCallback(
    () => resolveTaskEnvironmentId(appStoreApi.getState(), taskId, sessionId),
    [appStoreApi, sessionId, taskId],
  );
  const router = useRouter();
  const inFlightRef = useRef(new Set<string>());
  const generationRef = useRef(0);
  const mobileNavigationRef = useRef<string | null>(null);
  const inventoryRef = useRef(inventory);
  inventoryRef.current = inventory;
  const providedInventoryRef = useRef<{
    canvases?: readonly Canvas[];
    status?: TaskCanvasesLoadStatus;
  }>({ canvases: taskCanvases, status: taskCanvasesStatus });
  providedInventoryRef.current = { canvases: taskCanvases, status: taskCanvasesStatus };
  const reconcileRef = useRef<ReconcileRef["current"]>(null);

  useCanvasActivationEffect({
    enabled,
    taskId,
    workspaceId,
    sessionId,
    taskEnvironmentId,
    isMobile,
    userId,
    defaultUserId,
    layoutApi: api,
    layoutRestoring: isRestoringLayout,
    layoutEnvId: currentLayoutEnvId,
    revision,
    router,
    inventoryRef,
    providedInventoryRef,
    readTaskEnvironmentId,
    inFlightRef,
    generationRef,
    mobileNavigationRef,
    reconcileRef,
  });

  useEffect(() => {
    const snapshot = readCanvasInventory(inventoryRef, providedInventoryRef);
    if (snapshot.status !== "success") return;
    reconcileRef.current?.(
      getCanvasLifecycleHints().filter((hint) =>
        taskId && workspaceId ? shouldActivateCanvasForTask(hint, taskId, workspaceId) : false,
      ),
      0,
    );
  }, [inventory.canvases, inventory.status, taskCanvases, taskCanvasesStatus, taskId, workspaceId]);
}
