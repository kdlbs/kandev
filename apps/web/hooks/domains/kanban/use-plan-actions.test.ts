import { createElement, type ReactNode } from "react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { QueryClientProvider, type QueryClient } from "@tanstack/react-query";
import { makeQueryClient } from "@/lib/query/client";
import { qk } from "@/lib/query/keys";
import type { ContextFile } from "@/lib/state/context-files-store";

const ids = vi.hoisted(() => ({
  PLAN_STEP_ID: "plan-step",
  SESSION_ID: "session-1",
  TASK_ID: "task-1",
  WORKFLOW_ID: "workflow-1",
  WORK_STEP_ID: "work-step",
}));
const mockGetState = vi.fn();
const mockMoveTask = vi.hoisted(() => vi.fn());
const mockAppState = vi.hoisted(() => ({
  value: {
    kanban: { workflowId: ids.WORKFLOW_ID, steps: [], tasks: [] },
    tasks: { activeSessionId: ids.SESSION_ID },
    taskSessions: { items: {} },
    setActiveSession: vi.fn(),
    setPlanMode: vi.fn(),
    setActiveDocument: vi.fn(),
    setTaskPlan: vi.fn(),
  } as Record<string, unknown>,
}));
const mockContextFilesStore = vi.hoisted(() => ({
  value: {
    removeFile: vi.fn(),
  },
}));

vi.mock("@/lib/state/context-files-store", () => ({
  useContextFilesStore: Object.assign(
    (selector: (state: typeof mockContextFilesStore.value) => unknown) =>
      selector(mockContextFilesStore.value),
    { getState: () => mockGetState() },
  ),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mockAppState.value) => unknown) =>
    selector(mockAppState.value),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: vi.fn() }),
}));

vi.mock("@/lib/local-storage", () => ({
  setChatDraftContent: vi.fn(),
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  moveTask: mockMoveTask,
}));

vi.mock("@/lib/api/domains/plan-api", () => ({
  markPlanImplementationStarted: vi.fn(),
}));

vi.mock("@/lib/state/layout-store", () => ({
  useLayoutStore: (selector: (state: { closeDocument: () => void }) => unknown) =>
    selector({ closeDocument: vi.fn() }),
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: (selector: (state: { applyBuiltInPreset: () => void }) => unknown) =>
    selector({ applyBuiltInPreset: vi.fn() }),
}));

vi.mock("./use-implement-fresh", () => ({
  useImplementFresh: () => vi.fn(),
}));

import {
  buildImplementPlanContent,
  collectImplementPlanInput,
  readContextFilesMeta,
  usePlanActions,
} from "./use-plan-actions";

describe("buildImplementPlanContent", () => {
  it("appends the kandev-system block to user text", () => {
    const out = buildImplementPlanContent("ship it");
    expect(out.startsWith("ship it\n\n")).toBe(true);
    expect(out).toContain("<kandev-system>");
    expect(out).toContain("get_task_plan_kandev");
    expect(out).toContain("</kandev-system>");
  });

  it("trims whitespace from user text", () => {
    const out = buildImplementPlanContent("   hello  \n");
    expect(out.startsWith("hello\n\n")).toBe(true);
  });

  it("uses default visible text when input is empty", () => {
    expect(buildImplementPlanContent("")).toMatch(/^Implement the plan\n\n/);
    expect(buildImplementPlanContent("   ")).toMatch(/^Implement the plan\n\n/);
  });
});

function file(path: string, name: string, pinned?: boolean): ContextFile {
  return { path, name, pinned };
}

function queryWrapper(queryClient: QueryClient) {
  return function QueryWrapper({ children }: { children: ReactNode }) {
    return createElement(QueryClientProvider, { client: queryClient }, children);
  };
}

function createPlanActionsQueryClient() {
  const queryClient = makeQueryClient();
  queryClient.setQueryData(qk.tasks.detail(ids.TASK_ID), {
    id: ids.TASK_ID,
    workflow_id: ids.WORKFLOW_ID,
    workflow_step_id: ids.PLAN_STEP_ID,
  });
  queryClient.setQueryData(qk.workflows.steps(ids.WORKFLOW_ID), [
    {
      id: ids.PLAN_STEP_ID,
      name: "Plan",
      position: 1,
      events: { on_enter: [{ type: "enable_plan_mode" }] },
    },
    {
      id: ids.WORK_STEP_ID,
      name: "Work",
      position: 2,
      events: { on_enter: [{ type: "auto_start_agent" }] },
    },
  ]);
  return queryClient;
}

function renderPlanActions(args: Parameters<typeof usePlanActions>[0]) {
  const queryClient = createPlanActionsQueryClient();
  return renderHook(() => usePlanActions(args), { wrapper: queryWrapper(queryClient) });
}

describe("readContextFilesMeta", () => {
  const sessionId = "sess-1";
  const appFilePath = "src/app.ts";
  const appFileName = "app.ts";

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("returns an empty array when the session has no context files", () => {
    mockGetState.mockReturnValue({ filesBySessionId: {} });
    expect(readContextFilesMeta(sessionId)).toEqual([]);
  });

  it("maps real files to {path, name} pairs", () => {
    mockGetState.mockReturnValue({
      filesBySessionId: {
        [sessionId]: [file(appFilePath, appFileName), file("README.md", "README.md")],
      },
    });
    expect(readContextFilesMeta(sessionId)).toEqual([
      { path: appFilePath, name: appFileName },
      { path: "README.md", name: "README.md" },
    ]);
  });

  it("filters out the special plan:context path", () => {
    mockGetState.mockReturnValue({
      filesBySessionId: {
        [sessionId]: [file("plan:context", "Plan"), file(appFilePath, appFileName)],
      },
    });
    expect(readContextFilesMeta(sessionId)).toEqual([{ path: appFilePath, name: appFileName }]);
  });

  it("filters out prompt: prefixed paths", () => {
    mockGetState.mockReturnValue({
      filesBySessionId: {
        [sessionId]: [file("prompt:my-prompt", "My Prompt"), file(appFilePath, appFileName)],
      },
    });
    expect(readContextFilesMeta(sessionId)).toEqual([{ path: appFilePath, name: appFileName }]);
  });
});

describe("collectImplementPlanInput", () => {
  it("uses default empty input when no chat input handle exists", () => {
    mockGetState.mockReturnValue({ filesBySessionId: {} });
    expect(collectImplementPlanInput(null, "sess-1")).toEqual({
      userText: "",
      attachments: [],
      contextFilesMeta: [],
    });
  });
});

describe("usePlanActions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockMoveTask.mockResolvedValue(undefined);
    mockAppState.value = {
      ...mockAppState.value,
      kanban: {
        workflowId: ids.WORKFLOW_ID,
        steps: [
          {
            id: ids.PLAN_STEP_ID,
            title: "Plan",
            position: 1,
            events: { on_enter: [{ type: "enable_plan_mode" }] },
          },
          {
            id: ids.WORK_STEP_ID,
            title: "Work",
            position: 2,
            events: { on_enter: [{ type: "auto_start_agent" }] },
          },
        ],
        tasks: [{ id: ids.TASK_ID, workflowStepId: ids.PLAN_STEP_ID }],
      },
    };
  });

  it("keeps the implement handler available in plan mode before an auto-start work step", () => {
    const chatInputRef = {
      current: {
        getValue: () => "",
        getAttachments: () => [],
        clear: vi.fn(),
      },
    };

    const { result } = renderPlanActions({
      resolvedSessionId: ids.SESSION_ID,
      taskId: ids.TASK_ID,
      planModeEnabled: true,
      handlePlanModeChange: vi.fn(),
      chatInputRef: chatInputRef as never,
    });

    expect(result.current.implementPlanHandler).toEqual(expect.any(Function));
  });

  it("routes implement through the next auto-start work step", async () => {
    const { result } = renderPlanActions({
      resolvedSessionId: ids.SESSION_ID,
      taskId: ids.TASK_ID,
      planModeEnabled: true,
      handlePlanModeChange: vi.fn(),
      chatInputRef: { current: null },
    });

    await act(async () => {
      await result.current.implementPlanHandler?.(false);
    });

    expect(mockMoveTask).toHaveBeenCalledWith(ids.TASK_ID, {
      workflow_id: ids.WORKFLOW_ID,
      workflow_step_id: ids.WORK_STEP_ID,
      position: 0,
    });
    expect(mockAppState.value.setPlanMode).toHaveBeenCalledWith(ids.SESSION_ID, false);
  });

  it("keeps plan mode enabled when moving to the work step fails", async () => {
    mockMoveTask.mockRejectedValueOnce(new Error("move failed"));
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    const { result } = renderPlanActions({
      resolvedSessionId: ids.SESSION_ID,
      taskId: ids.TASK_ID,
      planModeEnabled: true,
      handlePlanModeChange: vi.fn(),
      chatInputRef: { current: null },
    });

    await act(async () => {
      await result.current.implementPlanHandler?.(false);
    });

    expect(mockMoveTask).toHaveBeenCalledTimes(1);
    expect(mockAppState.value.setPlanMode).not.toHaveBeenCalled();
  });

  it("does not expose the implement handler outside plan mode", () => {
    const { result } = renderPlanActions({
      resolvedSessionId: ids.SESSION_ID,
      taskId: ids.TASK_ID,
      planModeEnabled: false,
      handlePlanModeChange: vi.fn(),
      chatInputRef: { current: null },
    });

    expect(result.current.implementPlanHandler).toBeUndefined();
  });
});
