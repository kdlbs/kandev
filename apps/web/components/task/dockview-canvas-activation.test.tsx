import { renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Canvas } from "@/lib/api/domains/canvas-api";
import type { CanvasLifecycleHint } from "@/lib/canvas-lifecycle";

const { mockGetCanvas, mockRouterPush, mockGetHints, mockState, mockValues } = vi.hoisted(() => ({
  mockGetCanvas: vi.fn(),
  mockRouterPush: vi.fn(),
  mockGetHints: vi.fn(),
  mockState: { current: { api: null as unknown } },
  mockValues: { revision: 1, featureEnabled: true },
}));

vi.mock("@/lib/api/domains/canvas-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/canvas-api")>(
    "@/lib/api/domains/canvas-api",
  );
  return { ...actual, getCanvas: mockGetCanvas };
});
vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: () => mockValues.featureEnabled,
}));
vi.mock("@/lib/canvas-lifecycle", async () => {
  const actual =
    await vi.importActual<typeof import("@/lib/canvas-lifecycle")>("@/lib/canvas-lifecycle");
  return {
    ...actual,
    getCanvasLifecycleHints: mockGetHints,
    useCanvasLifecycleRevision: () => mockValues.revision,
  };
});
vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: mockRouterPush }),
}));
vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: (selector: (state: { api: unknown }) => unknown) => selector(mockState.current),
}));

import {
  activateCanvasPanel,
  canvasLifecycleActivationDecision,
  shouldActivateCanvasForTask,
  useTaskCanvasLifecycleActivation,
} from "./dockview-canvas-activation";

const TASK_ID = "task-1";
const WORKSPACE_ID = "workspace-1";

function canvas(overrides: Partial<Canvas> = {}): Canvas {
  return {
    id: "canvas-1",
    plugin_instance_id: "instance-1",
    plugin_id: "plugin-1",
    workspace_id: WORKSPACE_ID,
    task_id: TASK_ID,
    scope_kind: "task",
    title: "Project board",
    status: "active",
    ...overrides,
  };
}

function hint(overrides: Partial<CanvasLifecycleHint> = {}): CanvasLifecycleHint {
  return {
    revision: 1,
    action: "canvas.release.activated",
    payload: {
      canvas_id: "canvas-1",
      task_id: TASK_ID,
      workspace_id: WORKSPACE_ID,
    },
    ...overrides,
  };
}

function lifecycleApi() {
  const addPanel = vi.fn();
  const api = {
    addPanel,
    getPanel: vi.fn().mockReturnValue(undefined),
    groups: [{ id: "center" }],
    panels: [],
  };
  return { api, addPanel };
}

afterEach(() => {
  mockGetCanvas.mockReset();
  mockRouterPush.mockReset();
  mockGetHints.mockReset();
  mockValues.revision += 1;
  mockValues.featureEnabled = true;
  mockState.current = { api: null };
  vi.useRealTimers();
});

describe("canvas lifecycle matching", () => {
  it("matches only a release event for the active task and workspace", () => {
    expect(shouldActivateCanvasForTask(hint(), TASK_ID, WORKSPACE_ID)).toBe(true);
    expect(
      shouldActivateCanvasForTask(hint({ action: "canvas.created" }), TASK_ID, WORKSPACE_ID),
    ).toBe(false);
    expect(
      shouldActivateCanvasForTask(
        hint({ payload: { canvas_id: "canvas-1", task_id: "task-2" } }),
        TASK_ID,
        WORKSPACE_ID,
      ),
    ).toBe(false);
    expect(
      shouldActivateCanvasForTask(
        hint({ payload: { canvas_id: "canvas-1", task_id: TASK_ID, workspace_id: "workspace-2" } }),
        TASK_ID,
        WORKSPACE_ID,
      ),
    ).toBe(false);
  });
});

describe("canvas lifecycle panels", () => {
  it("adds and activates one task canvas panel", () => {
    const addPanel = vi.fn();
    const api = {
      addPanel,
      getPanel: vi.fn().mockReturnValue(undefined),
      groups: [{ id: "center" }],
      panels: [],
    };

    const added = activateCanvasPanel(api as never, canvas(), "center");

    expect(added).toBe(true);
    expect(addPanel).toHaveBeenCalledWith({
      id: "canvas:canvas-1",
      component: "canvas",
      title: "Project board",
      params: { canvasId: "canvas-1" },
      position: { referenceGroup: "center" },
    });
  });

  it("does not focus or add a panel that is already open", () => {
    const setActive = vi.fn();
    const addPanel = vi.fn();
    const api = {
      addPanel,
      getPanel: vi.fn().mockReturnValue({ api: { setActive } }),
      groups: [{ id: "center" }],
      panels: [],
    };

    expect(activateCanvasPanel(api as never, canvas(), "center")).toBe(false);
    expect(addPanel).not.toHaveBeenCalled();
    expect(setActive).not.toHaveBeenCalled();
  });
});

describe("canvas lifecycle decisions", () => {
  it("requires the authoritative active release before activating an event", () => {
    expect(
      canvasLifecycleActivationDecision(
        hint(),
        canvas({ active_release_id: "release-1", active_release_status: "valid" }),
      ),
    ).toBe("eligible");
    expect(
      canvasLifecycleActivationDecision(
        hint(),
        canvas({
          status: "archived",
          active_release_id: "release-1",
          active_release_status: "valid",
        }),
      ),
    ).toBe("stale");
    expect(
      canvasLifecycleActivationDecision(
        hint({ action: "canvas.release.permission_required" }),
        canvas({ active_release_id: "release-1", active_release_status: "valid" }),
      ),
    ).toBe("stale");
    expect(
      canvasLifecycleActivationDecision(
        hint({ action: "canvas.release.permission_required" }),
        canvas({
          status: "pending",
          pending_release: { id: "release-2", validation_status: "pending_permission" },
        }),
      ),
    ).toBe("eligible");
  });
});

describe("canvas lifecycle asynchronous activation", () => {
  it("does not replay an archived or rejected canvas from a retained hint", async () => {
    const { api, addPanel } = lifecycleApi();
    mockState.current = { api };
    mockGetHints.mockReturnValue([
      hint({ revision: 20 }),
      hint({
        revision: 21,
        action: "canvas.release.permission_required",
        payload: { canvas_id: "canvas-2", task_id: TASK_ID, workspace_id: WORKSPACE_ID },
      }),
    ]);
    mockGetCanvas
      .mockResolvedValueOnce(
        canvas({
          status: "archived",
          active_release_id: "release-1",
          active_release_status: "valid",
        }),
      )
      .mockResolvedValueOnce(
        canvas({
          id: "canvas-2",
          active_release_id: "release-1",
          active_release_status: "valid",
        }),
      );

    renderHook(() =>
      useTaskCanvasLifecycleActivation({
        taskId: TASK_ID,
        workspaceId: WORKSPACE_ID,
        isMobile: false,
      }),
    );

    await vi.waitFor(() => expect(mockGetCanvas).toHaveBeenCalledTimes(2));
    expect(addPanel).not.toHaveBeenCalled();
  });

  it("does not add a deferred result after the active task changes", async () => {
    const { api, addPanel } = lifecycleApi();
    mockState.current = { api };
    mockGetHints.mockReturnValue([hint({ revision: 30 })]);
    let resolveCanvas!: (value: Canvas) => void;
    mockGetCanvas.mockReturnValue(
      new Promise<Canvas>((resolve) => {
        resolveCanvas = resolve;
      }),
    );

    const hook = renderHook(
      ({ taskId }: { taskId: string }) =>
        useTaskCanvasLifecycleActivation({
          taskId,
          workspaceId: WORKSPACE_ID,
          isMobile: false,
        }),
      { initialProps: { taskId: TASK_ID } },
    );
    hook.rerender({ taskId: "task-2" });
    resolveCanvas(canvas());

    await vi.waitFor(() => expect(mockGetCanvas).toHaveBeenCalledTimes(1));
    expect(addPanel).not.toHaveBeenCalled();
  });
});

describe("canvas lifecycle mobile and retry activation", () => {
  it("authoritatively activates on mobile instead of routing from the hint", async () => {
    mockGetHints.mockReturnValue([hint({ revision: 40 })]);
    mockGetCanvas.mockResolvedValue(
      canvas({ active_release_id: "release-1", active_release_status: "valid" }),
    );

    renderHook(() =>
      useTaskCanvasLifecycleActivation({
        taskId: TASK_ID,
        workspaceId: WORKSPACE_ID,
        isMobile: true,
      }),
    );

    expect(mockRouterPush).not.toHaveBeenCalled();
    await vi.waitFor(() => expect(mockRouterPush).toHaveBeenCalledWith("/canvases/canvas-1"));
  });

  it("retries a transient authoritative lookup", async () => {
    vi.useFakeTimers();
    const { api, addPanel } = lifecycleApi();
    mockState.current = { api };
    mockGetHints.mockReturnValue([hint({ revision: 50 })]);
    mockGetCanvas
      .mockRejectedValueOnce(new Error("temporary transport failure"))
      .mockResolvedValueOnce(
        canvas({ active_release_id: "release-1", active_release_status: "valid" }),
      );

    renderHook(() =>
      useTaskCanvasLifecycleActivation({
        taskId: TASK_ID,
        workspaceId: WORKSPACE_ID,
        isMobile: false,
      }),
    );
    await Promise.resolve();
    await Promise.resolve();
    expect(mockGetCanvas).toHaveBeenCalledTimes(1);

    vi.advanceTimersByTime(500);
    await vi.waitFor(() => expect(mockGetCanvas).toHaveBeenCalledTimes(2));
    expect(addPanel).toHaveBeenCalledTimes(1);
  });
});
