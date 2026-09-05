import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("dockview-react", () => ({
  DockviewDefaultTab: () => null,
}));

vi.mock("@kandev/ui/context-menu", () => ({
  ContextMenu: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  ContextMenuTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

vi.mock("@/hooks/domains/session/use-session-actions", () => ({
  useSessionActions: () => ({
    setPrimary: vi.fn(),
    stop: vi.fn(),
    resume: vi.fn(),
    remove: vi.fn(),
  }),
}));

vi.mock("./session-tab-menu", () => ({
  SessionContextMenuItems: ({
    actions,
  }: {
    actions: {
      handleCloseOthers: () => void;
      hideSessionPanel?: () => void;
    };
  }) => (
    <>
      {actions.hideSessionPanel && (
        <button type="button" onClick={actions.hideSessionPanel}>
          Hide
        </button>
      )}
      <button type="button" onClick={actions.handleCloseOthers}>
        Close Others
      </button>
    </>
  ),
  SessionTabDialogs: () => null,
}));

vi.mock("./use-tab-maximize", () => ({ useTabMaximizeOnDoubleClick: () => () => {} }));
vi.mock("./session-tab-close-action", () => ({ SessionTabCloseAction: () => null }));
vi.mock("./session-tab-title", () => ({ resolveSessionTabTitle: () => "Current" }));
vi.mock("./session-sort", () => ({ isSessionActive: () => false }));
vi.mock("./session-tab-activation-intent", () => ({
  markSessionTabUserActivationIntent: vi.fn(),
  shouldMarkSessionTabUserActivationIntent: () => false,
}));
vi.mock("@/components/task/share/share-button", () => ({
  shareableSessionStateClient: () => false,
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

import { SessionTab } from "./session-tab";
import { StateProvider } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import type { TaskId, TaskSession } from "@/lib/types/http";

afterEach(() => cleanup());

function renderSoleSessionTab() {
  const currentPanel = { id: "session:current" };
  const api = {
    id: currentPanel.id,
    isActive: true,
    group: { id: "group-A", panels: [currentPanel] },
    onDidActiveChange: () => ({ dispose: vi.fn() }),
    onDidTitleChange: () => ({ dispose: vi.fn() }),
    title: "Current",
    setTitle: vi.fn(),
  };
  const containerApi = {
    panels: [currentPanel],
    getPanel: (id: string) => (id === currentPanel.id ? currentPanel : undefined),
    removePanel: vi.fn(),
    onDidAddPanel: () => ({ dispose: vi.fn() }),
    onDidRemovePanel: () => ({ dispose: vi.fn() }),
  };
  const currentSession = {
    id: "current",
    state: "COMPLETED",
    name: "Current",
    task_id: "task-A",
  } as TaskSession;

  render(
    <StateProvider
      initialState={{
        ...defaultState,
        tasks: { ...defaultState.tasks, activeTaskId: "task-A" as TaskId },
        taskSessions: { ...defaultState.taskSessions, items: { current: currentSession } },
        taskSessionsByTask: {
          ...defaultState.taskSessionsByTask,
          itemsByTaskId: { "task-A": [currentSession] },
        },
      }}
    >
      <SessionTab
        api={api as never}
        containerApi={containerApi as never}
        params={{}}
        tabLocation="header"
      />
    </StateProvider>,
  );
}

describe("SessionTab Close Others", () => {
  it("keeps Hide available when it is the sole visible session panel", () => {
    renderSoleSessionTab();

    expect(screen.getByRole("button", { name: "Hide" })).not.toBeNull();
    expect(screen.getByRole("button", { name: "Close Others" })).not.toBeNull();
  });

  it("preserves non-session tabs and session tabs in other Dockview groups", () => {
    const currentPanel = { id: "session:current" };
    const siblingSessionPanel = { id: "session:sibling" };
    const nonSessionPanel = { id: "files" };
    const sessionInAnotherGroup = { id: "session:other-group" };
    const removePanel = vi.fn();
    const api = {
      id: currentPanel.id,
      isActive: true,
      group: { id: "group-A", panels: [currentPanel, siblingSessionPanel, nonSessionPanel] },
      onDidActiveChange: () => ({ dispose: vi.fn() }),
      onDidTitleChange: () => ({ dispose: vi.fn() }),
      title: "Current",
      setTitle: vi.fn(),
    };
    const allPanels = [currentPanel, siblingSessionPanel, nonSessionPanel, sessionInAnotherGroup];
    const containerApi = {
      panels: allPanels,
      getPanel: (id: string) => allPanels.find((panel) => panel.id === id),
      removePanel,
      onDidAddPanel: () => ({ dispose: vi.fn() }),
      onDidRemovePanel: () => ({ dispose: vi.fn() }),
    };

    const currentSession = {
      id: "current",
      state: "COMPLETED",
      name: "Current",
      task_id: "task-A",
    } as TaskSession;
    const siblingSession = {
      id: "sibling",
      state: "COMPLETED",
      name: "Sibling",
      task_id: "task-A",
    } as TaskSession;
    render(
      <StateProvider
        initialState={{
          ...defaultState,
          tasks: { ...defaultState.tasks, activeTaskId: "task-A" as TaskId },
          taskSessions: { ...defaultState.taskSessions, items: { current: currentSession } },
          taskSessionsByTask: {
            ...defaultState.taskSessionsByTask,
            itemsByTaskId: { "task-A": [currentSession, siblingSession] },
          },
        }}
      >
        <SessionTab
          api={api as never}
          containerApi={containerApi as never}
          params={{}}
          tabLocation="header"
        />
      </StateProvider>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Close Others" }));

    expect(removePanel).toHaveBeenCalledExactlyOnceWith(siblingSessionPanel);
    expect(removePanel).not.toHaveBeenCalledWith(nonSessionPanel);
    expect(removePanel).not.toHaveBeenCalledWith(sessionInAnotherGroup);
  });
});
