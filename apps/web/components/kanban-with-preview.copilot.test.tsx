import { cleanup, render, screen, waitFor, act } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { createWorkspaceCopilotStore } from "@/hooks/domains/coordinator/workspace-copilot-store";
import { WorkspaceCopilotStoreProvider } from "@/hooks/domains/coordinator/workspace-copilot-context";

const IDS = vi.hoisted(() => ({
  surface: "right-side-panel",
  previewButton: "preview-task-2",
  closeButton: "close-preview",
  openKey: "kandev.kanban.preview.open",
  taskKey: "kandev.kanban.preview.selectedTask",
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false, isFinePointer: true }),
}));
vi.mock("@/lib/routing/client-router", () => ({ useRouter: () => ({ push: vi.fn() }) }));
vi.mock("@/hooks/use-task-session", () => ({ useTaskSession: () => ({ sessionId: null }) }));
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
vi.mock("./right-side-panel", () => ({
  RightSidePanel: ({
    open,
    onClose,
    main,
  }: {
    open: boolean;
    onClose: () => void;
    main: React.ReactNode;
  }) => (
    <div data-testid={IDS.surface} data-open={String(open)}>
      <button type="button" data-testid={IDS.closeButton} onClick={onClose} />
      {main}
    </div>
  ),
}));
vi.mock("./kanban-board", () => ({
  KanbanBoard: ({ onPreviewTask }: { onPreviewTask?: (task: { id: string }) => void }) => (
    <button
      type="button"
      data-testid={IDS.previewButton}
      onClick={() => onPreviewTask?.({ id: "task-2" })}
    />
  ),
}));
vi.mock("./task-preview-panel", () => ({ TaskPreviewPanel: () => null }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

import { KanbanWithPreview } from "./kanban-with-preview";

const tasks = ["task-1", "task-2"].map((id) => ({
  id,
  title: id,
  workflowStepId: "s",
  state: "TODO",
  description: "",
  position: 0,
}));
const INITIAL_STATE = {
  kanban: { workflowId: "wf-1", steps: [], tasks },
  kanbanMulti: { snapshots: {} },
} as never;

function renderBoard(
  store: ReturnType<typeof createWorkspaceCopilotStore>,
  initialTaskId?: string,
) {
  return render(
    <StateProvider initialState={INITIAL_STATE}>
      <WorkspaceCopilotStoreProvider store={store}>
        <KanbanWithPreview initialTaskId={initialTaskId} />
      </WorkspaceCopilotStoreProvider>
    </StateProvider>,
  );
}

const isOpen = () => screen.getByTestId(IDS.surface).getAttribute("data-open");

beforeEach(() => {
  window.localStorage.clear();
  window.localStorage.setItem(IDS.openKey, "true");
  window.localStorage.setItem(IDS.taskKey, JSON.stringify("task-1"));
});
afterEach(() => {
  cleanup();
  window.history.replaceState({}, "", "/");
});

describe("KanbanWithPreview alongside the workspace copilot", () => {
  it("yields a saved preview to the copilot and restores it when the copilot closes", async () => {
    const store = createWorkspaceCopilotStore();
    renderBoard(store);
    await waitFor(() => expect(isOpen()).toBe("true"));

    act(() => store.getState().openFor("ws-1", "c-1"));
    await waitFor(() => expect(isOpen()).toBe("false"));
    expect(window.localStorage.getItem(IDS.openKey)).toBe("true");
    expect(window.localStorage.getItem(IDS.taskKey)).toBe(JSON.stringify("task-1"));

    act(() => store.getState().setOpen("c-1", false));
    await waitFor(() => expect(isOpen()).toBe("true"));
  });

  it("keeps a saved preview when the copilot already holds the panel at mount", async () => {
    const store = createWorkspaceCopilotStore();
    store.getState().openFor("ws-1", "c-1");
    renderBoard(store);
    await waitFor(() => expect(isOpen()).toBe("false"));
    expect(store.getState().rightPanel).toBe("copilot");
    expect(window.localStorage.getItem(IDS.taskKey)).toBe(JSON.stringify("task-1"));
  });

  it("an explicit preview open takes the panel from the copilot", async () => {
    const store = createWorkspaceCopilotStore();
    store.getState().openFor("ws-1", "c-1");
    renderBoard(store);
    act(() => screen.getByTestId(IDS.previewButton).click());
    await waitFor(() => expect(isOpen()).toBe("true"));
    expect(store.getState()).toMatchObject({ open: false, rightPanel: "preview" });
  });

  it("a URL-driven preview claims the panel", async () => {
    const store = createWorkspaceCopilotStore();
    store.getState().openFor("ws-1", "c-1");
    renderBoard(store, "task-2");
    await waitFor(() => expect(store.getState().rightPanel).toBe("preview"));
    expect(store.getState().open).toBe(false);
  });

  it("closing the preview releases only its own claim", async () => {
    const store = createWorkspaceCopilotStore();
    renderBoard(store);
    await waitFor(() => expect(isOpen()).toBe("true"));
    act(() => screen.getByTestId(IDS.previewButton).click());
    expect(store.getState().rightPanel).toBe("preview");
    act(() => screen.getByTestId(IDS.closeButton).click());
    expect(store.getState().rightPanel).toBeNull();

    act(() => store.getState().openFor("ws-1", "c-1"));
    act(() => screen.getByTestId(IDS.closeButton).click());
    expect(store.getState().rightPanel).toBe("copilot");
  });
});
