"use client";

import { useEffect, useRef } from "react";
import type { DockviewApi } from "dockview-react";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { canvasHref, getCanvas, type Canvas } from "@/lib/api/domains/canvas-api";
import {
  getCanvasLifecycleHints,
  type CanvasLifecycleHint,
  useCanvasLifecycleRevision,
} from "@/lib/canvas-lifecycle";
import { useRouter } from "@/lib/routing/client-router";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { fallbackGroupPosition, focusOrAddPanel } from "@/lib/state/dockview-layout-builders";

const CANVAS_PANEL_ID_PREFIX = "canvas:";
const CANVAS_ACTIVATION_RETRY_DELAY_MS = 500;
const MAX_CANVAS_ACTIVATION_RETRIES = 5;
const CANVAS_RELEASE_ACTIONS = new Set<CanvasLifecycleHint["action"]>([
  "canvas.release.activated",
  "canvas.release.permission_required",
]);

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
): boolean {
  const id = canvasPanelId(canvas.id);
  if (api.getPanel(id)) return false;

  focusOrAddPanel(api, {
    id,
    component: "canvas",
    title: canvas.title,
    params: { canvasId: canvas.id },
    ...(groupId ? { position: { referenceGroup: groupId } } : {}),
  });
  return true;
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
  isMobile: boolean;
};

/** React to retained or newly-arrived release events for the active task.
 * Metadata remains authoritative, so the event only identifies which canvas
 * to refetch and open. */
export function useTaskCanvasLifecycleActivation({
  taskId,
  workspaceId,
  isMobile,
}: CanvasLifecycleActivationProps): void {
  const enabled = useFeature("canvases");
  const revision = useCanvasLifecycleRevision();
  const api = useDockviewStore((state) => state.api);
  const router = useRouter();
  const inFlightRef = useRef(new Set<string>());
  const generationRef = useRef(0);

  useEffect(() => {
    if (!enabled || !taskId || !workspaceId) return;

    const generation = ++generationRef.current;
    let disposed = false;
    const retryTimers = new Set<ReturnType<typeof setTimeout>>();
    const isCurrent = () => !disposed && generationRef.current === generation;

    const matchingHints = getCanvasLifecycleHints().filter((hint) =>
      shouldActivateCanvasForTask(hint, taskId, workspaceId),
    );

    const scheduleRetry = (hint: CanvasLifecycleHint, attempt: number) => {
      if (!isCurrent() || attempt >= MAX_CANVAS_ACTIVATION_RETRIES) return;
      const timer = setTimeout(() => {
        retryTimers.delete(timer);
        void fetchAndActivate(hint, attempt + 1);
      }, CANVAS_ACTIVATION_RETRY_DELAY_MS);
      retryTimers.add(timer);
    };

    const fetchAndActivate = async (hint: CanvasLifecycleHint, attempt: number) => {
      const key = hintKey(hint, taskId);
      if (isHandled(hint, taskId) || inFlightRef.current.has(key)) return;
      if (!isMobile && !api) return;

      inFlightRef.current.add(key);
      try {
        const canvas = await getCanvas(hint.payload.canvas_id);
        if (!isCurrent()) return;

        const decision = canvasLifecycleActivationDecision(hint, canvas);
        if (decision === "retry") {
          scheduleRetry(hint, attempt);
          return;
        }
        markHandled(hint, taskId);
        if (decision !== "eligible") return;

        if (isMobile) {
          router.push(canvasHref(canvas.id));
          return;
        }
        const panelId = canvasPanelId(canvas.id);
        if (api?.getPanel(panelId)) return;
        if (api) activateCanvasPanel(api, canvas, canvasPanelGroupId(api));
      } catch {
        if (isCurrent()) scheduleRetry(hint, attempt);
      } finally {
        inFlightRef.current.delete(key);
      }
    };

    for (const hint of matchingHints) void fetchAndActivate(hint, 0);

    return () => {
      disposed = true;
      generationRef.current += 1;
      retryTimers.forEach((timer) => clearTimeout(timer));
      retryTimers.clear();
    };
  }, [api, enabled, isMobile, revision, router, taskId, workspaceId]);
}
