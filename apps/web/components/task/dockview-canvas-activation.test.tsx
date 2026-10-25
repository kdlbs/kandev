import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Canvas } from "@/lib/api/domains/canvas-api";
import type { CanvasLifecycleHint } from "@/lib/canvas-lifecycle";

const {
  mockGetCanvas,
  mockListTaskCanvases,
  mockRouterPush,
  mockRouter,
  mockGetHints,
  mockState,
  mockValues,
  mockAppState,
  mockDockviewStore,
  mockAppStoreApi,
} = vi.hoisted(() => {
  const mockRouterPush = vi.fn();
  const mockState = {
    current: {
      api: null as unknown,
      isRestoringLayout: false,
      currentLayoutEnvId: null as string | null,
    },
  };
  const mockAppState = {
    auth: { mode: "disabled", user: null },
    connection: { status: "connected" },
    environmentIdBySessionId: { "session-1": "env-1", "session-2": "env-2" },
    taskSessionsByTask: {
      itemsByTaskId: {} as Record<string, Array<{ id: string; task_environment_id?: string }>>,
    },
    taskSessions: {
      items: {} as Record<string, { task_environment_id?: string }>,
    },
  };
  const mockDockviewStore = Object.assign(
    (selector: (state: typeof mockState.current) => unknown) => selector(mockState.current),
    { getState: () => mockState.current },
  );
  const mockAppStoreApi = { getState: () => mockAppState };
  return {
    mockGetCanvas: vi.fn(),
    mockListTaskCanvases: vi.fn().mockResolvedValue({ canvases: [] }),
    mockRouterPush,
    mockRouter: { push: mockRouterPush },
    mockGetHints: vi.fn(),
    mockState,
    mockValues: { revision: 1, featureEnabled: true },
    mockAppState,
    mockDockviewStore,
    mockAppStoreApi,
  };
});

vi.mock("@/lib/api/domains/canvas-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/canvas-api")>(
    "@/lib/api/domains/canvas-api",
  );
  return { ...actual, getCanvas: mockGetCanvas, listTaskCanvases: mockListTaskCanvases };
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
  useRouter: () => mockRouter,
}));
vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: mockDockviewStore,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mockAppState) => unknown) => selector(mockAppState),
  useAppStoreApi: () => mockAppStoreApi,
}));

import {
  activateCanvasPanel,
  canvasLifecycleActivationDecision,
  isTaskCanvasPresentationEligible,
  reconcileTaskCanvasPanels,
  shouldActivateCanvasForTask,
  useTaskCanvasLifecycleActivation,
} from "./dockview-canvas-activation";
import { wasCanvasPresented } from "@/lib/canvas-presentation-storage";

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

function presentedIdentity(canvasId: string, taskId = TASK_ID) {
  return {
    userId: "anonymous",
    workspaceId: WORKSPACE_ID,
    taskId,
    canvasId,
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
  mockState.current = { api, isRestoringLayout: false, currentLayoutEnvId: "env-1" };
  return { api, addPanel };
}

afterEach(() => {
  cleanup();
  mockGetCanvas.mockReset();
  mockListTaskCanvases.mockReset().mockResolvedValue({ canvases: [] });
  mockRouterPush.mockReset();
  mockGetHints.mockReset();
  mockGetHints.mockReturnValue([]);
  mockValues.revision += 1;
  mockValues.featureEnabled = true;
  mockState.current = { api: null, isRestoringLayout: false, currentLayoutEnvId: null };
  mockAppState.environmentIdBySessionId = { "session-1": "env-1", "session-2": "env-2" };
  mockAppState.taskSessionsByTask.itemsByTaskId = {};
  mockAppState.taskSessions.items = {};
  mockAppState.connection.status = "connected";
  window.sessionStorage.clear();
  vi.useRealTimers();
});

describe("task-entry inventory filtering", () => {
  it("opens a published inventory canvas when lifecycle hints are empty", async () => {
    const { addPanel } = lifecycleApi();
    mockGetHints.mockReturnValue([]);
    mockListTaskCanvases.mockResolvedValue({
      canvases: [
        canvas({
          created_at: "2026-09-21T10:00:00Z",
          active_release_id: "release-1",
          active_release_status: "valid",
        }),
      ],
    });

    renderHook(() =>
      useTaskCanvasLifecycleActivation({
        taskId: TASK_ID,
        workspaceId: WORKSPACE_ID,
        sessionId: "session-1",
        isMobile: false,
      }),
    );

    await vi.waitFor(() => expect(addPanel).toHaveBeenCalledTimes(1));
    expect(addPanel).toHaveBeenCalledWith(
      expect.objectContaining({
        id: "canvas:canvas-1",
        component: "canvas",
        params: { canvasId: "canvas-1" },
        position: { referenceGroup: "center" },
      }),
    );
  });

  it("filters inventory to eligible canvases in the current task", async () => {
    const { addPanel } = lifecycleApi();
    const eligible = canvas({
      id: "canvas-eligible",
      active_release_id: "release-eligible",
      active_release_status: "valid",
    });
    const permissionReview = canvas({
      id: "canvas-permission",
      status: "pending",
      pending_release: { id: "release-pending", validation_status: "pending_permission" },
    });
    const wrongTask = canvas({
      id: "canvas-other-task",
      task_id: "task-other",
      active_release_id: "release-other",
      active_release_status: "valid",
    });
    const archived = canvas({
      id: "canvas-archived",
      status: "archived",
      active_release_id: "release-archived",
      active_release_status: "valid",
    });
    mockListTaskCanvases.mockResolvedValue({
      canvases: [eligible, permissionReview, wrongTask, archived],
    });

    renderHook(() =>
      useTaskCanvasLifecycleActivation({
        taskId: TASK_ID,
        workspaceId: WORKSPACE_ID,
        sessionId: "session-1",
        isMobile: false,
      }),
    );

    await vi.waitFor(() => expect(addPanel).toHaveBeenCalledTimes(2));
    expect(addPanel.mock.calls.map(([options]) => options.id)).toEqual([
      "canvas:canvas-eligible",
      "canvas:canvas-permission",
    ]);
    expect(isTaskCanvasPresentationEligible(wrongTask, TASK_ID, WORKSPACE_ID)).toBe(false);
  });
});

describe("task-entry layout ownership", () => {
  it("uses a hydrated task session environment when no session is selected", async () => {
    const { addPanel } = lifecycleApi();
    const canvasWithoutSelectedSession = canvas({
      active_release_id: "release-no-session",
      active_release_status: "valid",
    });
    mockState.current.currentLayoutEnvId = "env-3";
    mockAppState.taskSessionsByTask.itemsByTaskId[TASK_ID] = [
      { id: "session-3", task_environment_id: "env-3" },
    ];

    renderHook(() =>
      useTaskCanvasLifecycleActivation({
        taskId: TASK_ID,
        workspaceId: WORKSPACE_ID,
        sessionId: null,
        isMobile: false,
        taskCanvases: [canvasWithoutSelectedSession],
        taskCanvasesStatus: "success",
      }),
    );

    await vi.waitFor(() => expect(addPanel).toHaveBeenCalledTimes(1));
  });

  it("waits for the target task environment before recording a presentation", async () => {
    const { api, addPanel } = lifecycleApi();
    const taskB = "task-2";
    const canvasB = canvas({
      id: "canvas-b",
      task_id: taskB,
      active_release_id: "release-b",
      active_release_status: "valid",
    });
    const props = {
      taskId: taskB,
      workspaceId: WORKSPACE_ID,
      sessionId: "session-2",
      isMobile: false,
      taskCanvases: [canvasB],
      taskCanvasesStatus: "success" as const,
    };

    const hook = renderHook(() => useTaskCanvasLifecycleActivation(props));
    await Promise.resolve();
    expect(addPanel).not.toHaveBeenCalled();
    expect(wasCanvasPresented(presentedIdentity(canvasB.id, taskB))).toBe(false);

    mockState.current = { api, isRestoringLayout: false, currentLayoutEnvId: "env-2" };
    hook.rerender();

    await vi.waitFor(() => expect(addPanel).toHaveBeenCalledTimes(1));
    expect(wasCanvasPresented(presentedIdentity(canvasB.id, taskB))).toBe(true);
  });

  it("rechecks layout ownership after an authoritative hint lookup", async () => {
    const { api, addPanel } = lifecycleApi();
    const taskB = "task-2";
    const canvasB = canvas({
      id: "canvas-b",
      task_id: taskB,
      active_release_id: "release-b",
      active_release_status: "valid",
    });
    mockState.current = { api, isRestoringLayout: false, currentLayoutEnvId: "env-2" };
    mockGetHints.mockReturnValue([
      hint({
        revision: 70,
        payload: { canvas_id: canvasB.id, task_id: taskB, workspace_id: WORKSPACE_ID },
      }),
    ]);
    let resolveCanvas!: (value: Canvas) => void;
    mockGetCanvas.mockReturnValue(
      new Promise<Canvas>((resolve) => {
        resolveCanvas = resolve;
      }),
    );

    const props = {
      taskId: taskB,
      workspaceId: WORKSPACE_ID,
      sessionId: "session-2",
      isMobile: false,
      taskCanvases: [] as Canvas[],
      taskCanvasesStatus: "success" as const,
    };
    const hook = renderHook(() => useTaskCanvasLifecycleActivation(props));
    await vi.waitFor(() => expect(mockGetCanvas).toHaveBeenCalledTimes(1));

    mockState.current = { api, isRestoringLayout: false, currentLayoutEnvId: "env-1" };
    hook.rerender();
    resolveCanvas(canvasB);
    await Promise.resolve();
    await Promise.resolve();

    expect(addPanel).not.toHaveBeenCalled();
    expect(wasCanvasPresented(presentedIdentity(canvasB.id, taskB))).toBe(false);
  });
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

  it("adds a batch in creation order and focuses only the final panel", () => {
    const addedIds = new Set<string>();
    const addPanel = vi.fn((options: { id: string }) => addedIds.add(options.id));
    const api = {
      addPanel,
      getPanel: vi.fn((id: string) =>
        addedIds.has(id) ? { api: { setActive: vi.fn() } } : undefined,
      ),
      groups: [{ id: "center" }],
      panels: [],
    };
    const older = canvas({
      id: "canvas-older",
      created_at: "2026-09-21T10:00:00Z",
      active_release_id: "release-older",
      active_release_status: "valid",
    });
    const newer = canvas({
      id: "canvas-newer",
      created_at: "2026-09-21T11:00:00Z",
      active_release_id: "release-newer",
      active_release_status: "valid",
    });

    reconcileTaskCanvasPanels(api as never, [newer, older], {
      userId: "anonymous",
      workspaceId: WORKSPACE_ID,
      taskId: TASK_ID,
    });

    expect(addPanel).toHaveBeenCalledTimes(2);
    expect(addPanel.mock.calls.map(([options]) => options.id)).toEqual([
      "canvas:canvas-older",
      "canvas:canvas-newer",
    ]);
    expect(addPanel.mock.calls[0][0]).toMatchObject({ inactive: true });
    expect(addPanel.mock.calls[1][0]).not.toHaveProperty("inactive");
    expect(wasCanvasPresented(presentedIdentity(older.id))).toBe(true);
    expect(wasCanvasPresented(presentedIdentity(newer.id))).toBe(true);
  });

  it("records a restored receipt for a canvas already open in dockview", () => {
    const addPanel = vi.fn();
    const api = {
      addPanel,
      getPanel: vi.fn().mockReturnValue({ api: { setActive: vi.fn() } }),
      groups: [{ id: "center" }],
      panels: [],
    };

    reconcileTaskCanvasPanels(api as never, [canvas({ id: "canvas-restored" })], {
      userId: "anonymous",
      workspaceId: WORKSPACE_ID,
      taskId: TASK_ID,
    });

    expect(addPanel).not.toHaveBeenCalled();
    expect(wasCanvasPresented(presentedIdentity("canvas-restored"))).toBe(true);
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
    mockState.current = { api, isRestoringLayout: false, currentLayoutEnvId: "env-1" };
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
        sessionId: "session-1",
        isMobile: false,
      }),
    );

    await vi.waitFor(() => expect(mockGetCanvas).toHaveBeenCalledTimes(2));
    expect(addPanel).not.toHaveBeenCalled();
  });

  it("does not add a deferred result after the active task changes", async () => {
    const { api, addPanel } = lifecycleApi();
    mockState.current = { api, isRestoringLayout: false, currentLayoutEnvId: "env-1" };
    mockGetHints.mockReturnValue([hint({ revision: 30 })]);
    let resolveCanvas!: (value: Canvas) => void;
    const deferredCanvas = new Promise<Canvas>((resolve) => {
      resolveCanvas = resolve;
    });
    mockGetCanvas.mockReturnValue(deferredCanvas);

    const hook = renderHook(
      ({ taskId }: { taskId: string }) =>
        useTaskCanvasLifecycleActivation({
          taskId,
          workspaceId: WORKSPACE_ID,
          sessionId: "session-1",
          isMobile: false,
        }),
      { initialProps: { taskId: TASK_ID } },
    );
    hook.rerender({ taskId: "task-2" });
    resolveCanvas(canvas());

    await vi.waitFor(() => expect(mockGetCanvas).toHaveBeenCalledTimes(1));
    expect(addPanel).not.toHaveBeenCalled();
  });

  it("retries a hint lookup when a newer generation invalidates the pending one", async () => {
    const { api, addPanel } = lifecycleApi();
    mockState.current = { api, isRestoringLayout: false, currentLayoutEnvId: "env-1" };
    mockGetHints.mockReturnValue([hint({ revision: 31 })]);
    let resolveFirst!: (value: Canvas) => void, resolveSecond!: (value: Canvas) => void;
    const first = new Promise<Canvas>((resolve) => (resolveFirst = resolve));
    const second = new Promise<Canvas>((resolve) => (resolveSecond = resolve));
    mockGetCanvas.mockReturnValueOnce(first).mockReturnValueOnce(second);

    const hook = renderHook(() =>
      useTaskCanvasLifecycleActivation({
        taskId: TASK_ID,
        workspaceId: WORKSPACE_ID,
        sessionId: "session-1",
        isMobile: false,
        taskCanvases: [],
        taskCanvasesStatus: "success",
      }),
    );
    await vi.waitFor(() => expect(mockGetCanvas).toHaveBeenCalledTimes(1));

    mockValues.revision += 1;
    hook.rerender();
    await vi.waitFor(() => expect(mockGetCanvas).toHaveBeenCalledTimes(2));

    resolveSecond(canvas({ active_release_id: "release-1", active_release_status: "valid" }));
    await vi.waitFor(() => expect(addPanel).toHaveBeenCalledTimes(1));
    resolveFirst(canvas({ active_release_id: "release-1", active_release_status: "valid" }));
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
        sessionId: "session-1",
        isMobile: true,
      }),
    );

    expect(mockRouterPush).not.toHaveBeenCalled();
    await vi.waitFor(() =>
      expect(mockRouterPush).toHaveBeenCalledWith(
        "/canvases/canvas-1",
        expect.objectContaining({ onNavigated: expect.any(Function) }),
      ),
    );
  });

  it("retries a transient authoritative lookup", async () => {
    vi.useFakeTimers();
    const { api, addPanel } = lifecycleApi();
    mockState.current = { api, isRestoringLayout: false, currentLayoutEnvId: "env-1" };
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
        sessionId: "session-1",
        isMobile: false,
      }),
    );
    await Promise.resolve();
    await Promise.resolve();
    expect(mockGetCanvas).toHaveBeenCalledTimes(1);

    vi.advanceTimersByTime(500);
    await vi.waitFor(() => expect(mockGetCanvas).toHaveBeenCalledTimes(2));
    await vi.waitFor(() => expect(addPanel).toHaveBeenCalledTimes(1));
  });
});
