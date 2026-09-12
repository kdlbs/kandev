import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";

const previewState = vi.hoisted(() => ({
  selectedTaskId: "task-1",
  isOpen: true,
  previewWidthPx: 360,
  open: vi.fn(),
  close: vi.fn(),
  updatePreviewWidth: vi.fn(),
}));
const TEST_IDS = vi.hoisted(() => ({
  panel: "task-preview-panel",
  secondarySessionButton: "select-secondary-session",
  task: "task-1",
  primarySession: "session-primary",
  secondarySession: "session-secondary",
}));

vi.mock("@/hooks/use-kanban-preview", () => ({
  useKanbanPreview: () => previewState,
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false }),
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
vi.mock("@/hooks/use-task-session", () => ({
  useTaskSession: () => ({ sessionId: "session-primary" }),
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
  TaskPreviewPanel: ({
    sessionId,
    onSessionChange,
  }: {
    sessionId?: string | null;
    onSessionChange?: (sessionId: string | null) => void;
  }) => (
    <div data-testid={TEST_IDS.panel} data-session-id={sessionId ?? ""}>
      <button
        type="button"
        data-testid={TEST_IDS.secondarySessionButton}
        onClick={() => onSessionChange?.(TEST_IDS.secondarySession)}
      >
        Select secondary
      </button>
    </div>
  ),
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
  primarySessionId: TEST_IDS.primarySession,
};

function StoreCapture({
  onStore,
}: {
  onStore: (store: ReturnType<typeof useAppStoreApi>) => void;
}) {
  onStore(useAppStoreApi());
  return null;
}

afterEach(() => {
  previewState.open.mockReset();
  previewState.close.mockReset();
  previewState.updatePreviewWidth.mockReset();
  window.history.replaceState({}, "", "/");
});

describe("KanbanWithPreview removal recovery", () => {
  it("restores a non-primary session after a failed preview removal", async () => {
    let store!: ReturnType<typeof useAppStoreApi>;
    render(
      <StateProvider
        initialState={
          {
            kanban: { workflowId: "workflow-1", steps: [], tasks: [TASK] },
            kanbanMulti: { snapshots: {} },
          } as never
        }
      >
        <StoreCapture onStore={(value) => (store = value)} />
        <KanbanWithPreview />
      </StateProvider>,
    );

    await waitFor(() =>
      expect(screen.getByTestId(TEST_IDS.panel).getAttribute("data-session-id")).toBe(
        TEST_IDS.primarySession,
      ),
    );

    fireEvent.click(screen.getByTestId(TEST_IDS.secondarySessionButton));
    await waitFor(() =>
      expect(screen.getByTestId(TEST_IDS.panel).getAttribute("data-session-id")).toBe(
        TEST_IDS.secondarySession,
      ),
    );
    expect(store.getState().tasks.activeSessionId).toBe(TEST_IDS.secondarySession);
    expect(new URL(window.location.href).searchParams.get("sessionId")).toBe(
      TEST_IDS.secondarySession,
    );

    let token: string | null = null;
    act(() => {
      token = store.getState().beginTaskRemoval({
        action: "archive",
        workspaceId: "ws-1",
        taskIds: [TEST_IDS.task],
        requestIds: [TEST_IDS.task],
        departure: null,
      });
    });

    await waitFor(() => expect(screen.queryByTestId(TEST_IDS.panel)).toBeNull());

    act(() => {
      store.getState().releaseTaskRemoval(token!);
    });

    await waitFor(() =>
      expect(screen.getByTestId(TEST_IDS.panel).getAttribute("data-session-id")).toBe(
        TEST_IDS.secondarySession,
      ),
    );
    expect(store.getState().tasks.activeSessionId).toBe(TEST_IDS.secondarySession);
    expect(new URL(window.location.href).searchParams.get("sessionId")).toBe(
      TEST_IDS.secondarySession,
    );
  });
});
