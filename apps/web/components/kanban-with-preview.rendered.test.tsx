import { cleanup, render, screen } from "@testing-library/react";
import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type * as SessionApi from "@/lib/api/domains/session-api";

const previewState = vi.hoisted(() => ({
  selectedTaskId: "task-1",
  isOpen: true,
  previewWidthPx: 360,
  open: vi.fn(),
  close: vi.fn(),
  updatePreviewWidth: vi.fn(),
  displace: vi.fn(),
}));
const responsiveState = vi.hoisted(() => ({ isMobile: false, isFinePointer: true }));
const TEST_IDS = vi.hoisted(() => ({
  panel: "task-preview-panel",
  task: "task-1",
}));

vi.mock("@/hooks/use-kanban-preview", () => ({
  useKanbanPreview: () => previewState,
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => responsiveState,
}));
vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: vi.fn() }),
}));
vi.mock("@/hooks/use-kanban-layout", () => ({
  useKanbanLayout: () => ({
    containerRef: { current: null },
    shouldFloat: false,
    kanbanWidth: 640,
  }),
}));
vi.mock("@/hooks/domains/session/use-ensure-task-session", () => ({
  useEnsureTaskSession: () => ({}),
}));
vi.mock("@/hooks/domains/kanban/use-preview-workflow-step-move", () => ({
  usePreviewWorkflowStepMove: () => ({
    workflowSteps: [],
    currentStepId: null,
    taskWorkflowId: null,
    isArchived: false,
    movingToStepId: null,
    handleMove: vi.fn(),
    handleDisclosureOpenChange: vi.fn(),
    isDisclosureOpen: () => false,
    moveError: null,
  }),
}));
vi.mock("./kanban-board", () => ({
  KanbanBoard: () => <div data-testid="kanban-board" />,
}));
vi.mock("./task-preview-panel", () => ({
  TaskPreviewPanel: () => <div data-testid={TEST_IDS.panel} />,
}));
vi.mock("@/lib/api/domains/session-api", async (original) => ({
  ...(await original<typeof SessionApi>()),
  listTaskSessions: vi.fn(async () => ({ sessions: [], total: 0 })),
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

import { KanbanWithPreview } from "./kanban-with-preview";

const TASK = {
  id: TEST_IDS.task,
  title: "Preview task",
  workflowStepId: "step-1",
  state: "TODO",
  description: "",
  position: 0,
};
const INITIAL_STATE = {
  kanban: { workflowId: "workflow-1", steps: [], tasks: [TASK] },
  kanbanMulti: { snapshots: {} },
} as never;

afterEach(() => {
  cleanup();
  previewState.open.mockReset();
  previewState.close.mockReset();
  previewState.updatePreviewWidth.mockReset();
  previewState.displace.mockReset();
  previewState.previewWidthPx = 360;
  responsiveState.isFinePointer = true;
  window.history.replaceState({}, "", "/");
});

describe("KanbanWithPreview rendered preview width", () => {
  it("updates for pointer mode without changing the chosen width", () => {
    previewState.previewWidthPx = 320;
    responsiveState.isFinePointer = true;
    const { rerender } = render(
      <StateProvider initialState={INITIAL_STATE}>
        <KanbanWithPreview />
      </StateProvider>,
    );

    const getPanelShell = () => {
      const panel = screen.getByTestId(TEST_IDS.panel);
      const shell = panel.parentElement?.parentElement;
      if (!(shell instanceof HTMLElement)) throw new Error("preview panel shell is missing");
      return shell;
    };

    expect(getPanelShell().style.width).toBe("320px");

    act(() => {
      responsiveState.isFinePointer = false;
      rerender(
        <StateProvider initialState={INITIAL_STATE}>
          <KanbanWithPreview />
        </StateProvider>,
      );
    });

    expect(getPanelShell().style.width).toBe("380px");

    act(() => {
      responsiveState.isFinePointer = true;
      rerender(
        <StateProvider initialState={INITIAL_STATE}>
          <KanbanWithPreview />
        </StateProvider>,
      );
    });

    expect(getPanelShell().style.width).toBe("320px");
  });
});
