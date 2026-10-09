import type { DockviewApi } from "dockview-react";
import { canvasHref, getCanvas, type Canvas } from "@/lib/api/domains/canvas-api";
import type { CanvasLifecycleHint } from "@/lib/canvas-lifecycle";
import {
  recordCanvasPresentation,
  type CanvasPresentationIdentity,
  wasCanvasPresented,
} from "@/lib/canvas-presentation-storage";
import { type AppRouter } from "@/lib/routing/client-router";
import type {
  TaskCanvasesLoadStatus,
  TaskCanvasesState,
} from "@/hooks/domains/task/use-task-canvases";

export type DockviewLayoutSnapshot = {
  api: DockviewApi | null;
  isRestoringLayout: boolean;
  currentLayoutEnvId: string | null;
};

export type CanvasInventoryRef = { current: TaskCanvasesState };
export type ProvidedCanvasInventoryRef = {
  current: { canvases?: readonly Canvas[]; status?: TaskCanvasesLoadStatus };
};
export type ReconcileRef = {
  current: ((hints: CanvasLifecycleHint[], attempt: number) => void) | null;
};

type CanvasInventorySnapshot = {
  canvases: readonly Canvas[];
  status: TaskCanvasesLoadStatus;
};

export function readCanvasInventory(
  inventoryRef: CanvasInventoryRef,
  providedRef: ProvidedCanvasInventoryRef,
): CanvasInventorySnapshot {
  const provided = providedRef.current;
  if (provided.canvases !== undefined) {
    return { canvases: provided.canvases, status: provided.status ?? "success" };
  }
  return inventoryRef.current;
}

type ResolveHintOptions = {
  hint: CanvasLifecycleHint;
  attempt: number;
  generation: number;
  taskId: string;
  workspaceId: string;
  inventoryRef: CanvasInventoryRef;
  providedInventoryRef: ProvidedCanvasInventoryRef;
  inFlight: Set<string>;
  isCurrent: () => boolean;
  hintKey: (hint: CanvasLifecycleHint, taskId: string) => string;
  canvasLifecycleActivationDecision: (
    hint: CanvasLifecycleHint,
    canvas: Canvas,
  ) => "eligible" | "retry" | "stale";
  markHandled: (hint: CanvasLifecycleHint, taskId: string) => void;
  isHandled: (hint: CanvasLifecycleHint, taskId: string) => boolean;
  scheduleRetry: (hint: CanvasLifecycleHint, attempt: number) => void;
};

async function resolveTaskCanvasHint(options: ResolveHintOptions): Promise<Canvas | null> {
  const { hint, taskId, workspaceId } = options;
  const key = `${options.generation}\u0000${options.hintKey(hint, taskId)}`;
  if (options.isHandled(hint, taskId) || options.inFlight.has(key)) return null;

  options.inFlight.add(key);
  try {
    const snapshot = readCanvasInventory(options.inventoryRef, options.providedInventoryRef);
    const listed =
      snapshot.status === "success"
        ? snapshot.canvases.find((canvas) => canvas.id === hint.payload.canvas_id)
        : undefined;
    const canvas = listed ?? (await getCanvas(hint.payload.canvas_id));
    if (!options.isCurrent()) return null;
    if (canvas.workspace_id !== workspaceId) {
      options.markHandled(hint, taskId);
      return null;
    }

    const decision = options.canvasLifecycleActivationDecision(hint, canvas);
    if (decision === "retry") {
      options.scheduleRetry(hint, options.attempt);
      return null;
    }
    options.markHandled(hint, taskId);
    return decision === "eligible" ? canvas : null;
  } catch {
    if (options.isCurrent()) options.scheduleRetry(hint, options.attempt);
    return null;
  } finally {
    options.inFlight.delete(key);
  }
}

export type ReconcileTaskCanvasOptions = {
  hints: CanvasLifecycleHint[];
  attempt: number;
  generation: number;
  taskId: string;
  workspaceId: string;
  identity: Omit<CanvasPresentationIdentity, "canvasId">;
  readTaskEnvironmentId: () => string | null;
  readDockviewLayout: () => DockviewLayoutSnapshot;
  isMobile: boolean;
  router: AppRouter;
  mobileNavigationRef: { current: string | null };
  inventoryRef: CanvasInventoryRef;
  providedInventoryRef: ProvidedCanvasInventoryRef;
  inFlight: Set<string>;
  isCurrent: () => boolean;
  scheduleRetry: (hint: CanvasLifecycleHint, attempt: number) => void;
};

export type ReconcileTaskCanvasDependencies = {
  hintKey: (hint: CanvasLifecycleHint, taskId: string) => string;
  isHandled: (hint: CanvasLifecycleHint, taskId: string) => boolean;
  markHandled: (hint: CanvasLifecycleHint, taskId: string) => void;
  layoutOwnsTask: (layout: DockviewLayoutSnapshot, taskEnvironmentId: string | null) => boolean;
  isTaskCanvasPresentationEligible: (
    canvas: Canvas,
    taskId: string,
    workspaceId: string,
  ) => boolean;
  sortTaskCanvasPresentationCandidates: (canvases: readonly Canvas[]) => Canvas[];
  canvasIdentity: (
    identity: Omit<CanvasPresentationIdentity, "canvasId">,
    canvasId: string,
  ) => CanvasPresentationIdentity;
  canvasLifecycleActivationDecision: (
    hint: CanvasLifecycleHint,
    canvas: Canvas,
  ) => "eligible" | "retry" | "stale";
  selectMobileCanvasPresentation: (
    canvases: readonly Canvas[],
    identity: Omit<CanvasPresentationIdentity, "canvasId">,
  ) => { canvas: Canvas; offered: Canvas[] } | null;
  reconcileTaskCanvasPanels: (
    api: DockviewApi,
    canvases: readonly Canvas[],
    identity: Omit<CanvasPresentationIdentity, "canvasId">,
  ) => Canvas[];
};

export async function reconcileTaskCanvasCandidates(
  options: ReconcileTaskCanvasOptions,
  dependencies: ReconcileTaskCanvasDependencies,
): Promise<void> {
  if (!options.isCurrent()) return;
  if (
    !options.isMobile &&
    !dependencies.layoutOwnsTask(options.readDockviewLayout(), options.readTaskEnvironmentId())
  ) {
    return;
  }

  const snapshot = readCanvasInventory(options.inventoryRef, options.providedInventoryRef);
  const listedCandidates =
    snapshot.status === "success"
      ? snapshot.canvases.filter((canvas) =>
          dependencies.isTaskCanvasPresentationEligible(
            canvas,
            options.taskId,
            options.workspaceId,
          ),
        )
      : [];
  const hintedCandidates = (
    await Promise.all(
      options.hints.map((hint) =>
        resolveTaskCanvasHint({
          ...options,
          hint,
          hintKey: dependencies.hintKey,
          isHandled: dependencies.isHandled,
          markHandled: dependencies.markHandled,
          canvasLifecycleActivationDecision: dependencies.canvasLifecycleActivationDecision,
        }),
      ),
    )
  ).filter((canvas): canvas is Canvas => canvas !== null);
  if (!options.isCurrent()) return;

  const currentLayout = options.readDockviewLayout();
  const currentTaskEnvironmentId = options.readTaskEnvironmentId();
  if (!options.isMobile && !dependencies.layoutOwnsTask(currentLayout, currentTaskEnvironmentId)) {
    return;
  }

  const uniqueCanvases = new Map<string, Canvas>();
  [...listedCandidates, ...hintedCandidates].forEach((canvas) => {
    if (
      dependencies.isTaskCanvasPresentationEligible(canvas, options.taskId, options.workspaceId)
    ) {
      uniqueCanvases.set(canvas.id, canvas);
    }
  });
  const candidates = dependencies
    .sortTaskCanvasPresentationCandidates([...uniqueCanvases.values()])
    .filter(
      (canvas) => !wasCanvasPresented(dependencies.canvasIdentity(options.identity, canvas.id)),
    );
  if (candidates.length === 0) return;

  if (options.isMobile) {
    const decision = dependencies.selectMobileCanvasPresentation(candidates, options.identity);
    if (!decision || options.mobileNavigationRef.current) return;
    options.mobileNavigationRef.current = decision.canvas.id;
    try {
      options.router.push(canvasHref(decision.canvas.id), {
        onNavigated: () => {
          if (!options.isCurrent()) return;
          decision.offered.forEach((canvas) =>
            recordCanvasPresentation(
              dependencies.canvasIdentity(options.identity, canvas.id),
              "automatic",
            ),
          );
          options.mobileNavigationRef.current = null;
        },
      });
    } catch {
      options.mobileNavigationRef.current = null;
    }
    return;
  }

  if (currentLayout.api) {
    dependencies.reconcileTaskCanvasPanels(currentLayout.api, candidates, options.identity);
  }
}
