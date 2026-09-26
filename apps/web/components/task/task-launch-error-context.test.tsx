import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { TaskStatusSummary } from "@/lib/types/task-status-summary";
import { TaskLaunchErrorProvider, useTaskLaunchErrorContext } from "./task-launch-error-context";

const navigation = vi.hoisted(() => ({
  mobile: vi.fn(),
  activate: vi.fn(),
  add: vi.fn(),
  select: vi.fn(),
  getPanel: vi.fn(() => ({ api: { setActive: vi.fn() } })),
}));
const { toastMock } = vi.hoisted(() => ({
  toastMock: vi.fn(),
}));
const { statusSummaryMock } = vi.hoisted(() => ({
  statusSummaryMock: vi.fn(),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: toastMock }),
}));
vi.mock("@/lib/i18n", () => ({ t: (key: string) => key }));
vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({
    getState: () => ({
      setMobileSessionPanel: navigation.mobile,
      setActiveSession: navigation.select,
    }),
  }),
}));
vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: {
    getState: () => ({
      api: { getPanel: navigation.getPanel },
      centerGroupId: "center",
      addChatPanel: navigation.add,
    }),
  },
}));
vi.mock("@/hooks/domains/task/use-task-status-summary", () => ({
  useTaskStatusSummary: statusSummaryMock,
}));

const initialSummary: TaskStatusSummary = {
  revision: 1,
  updated_at: "2026-08-20T00:00:00Z",
  active_error: {
    stamp: "initial",
    occurred_at: "2026-08-20T00:00:00Z",
    preview: "initial error",
    category: "base_branch_missing",
  },
};

function SummaryStamp() {
  const context = useTaskLaunchErrorContext();
  return (
    <span data-testid="summary-stamp">{context?.statusSummary?.active_error?.stamp ?? "none"}</span>
  );
}

function renderProvider(statusSummary: TaskStatusSummary) {
  return render(
    <TaskLaunchErrorProvider value={{ taskId: "task-1", workspaceId: WORKSPACE_ID, statusSummary }}>
      <SummaryStamp />
    </TaskLaunchErrorProvider>,
  );
}

afterEach(() => {
  cleanup();
});

beforeEach(() => {
  navigation.getPanel.mockImplementation(() => ({ api: { setActive: navigation.activate } }));
  toastMock.mockReset();
  statusSummaryMock.mockReset();
  statusSummaryMock.mockImplementation(
    (_taskId: string, detail: TaskStatusSummary | null | undefined) => detail,
  );
});

describe("TaskLaunchErrorProvider", () => {
  it("does not toast for a typed launch error while its card is visible", () => {
    renderProvider(initialSummary);

    expect(toastMock).not.toHaveBeenCalled();
  });

  it.each(["provider_auth_required", "model_capacity"])(
    "does not toast for ordinary active error category %s",
    (category) => {
      renderProvider({
        ...initialSummary,
        active_error: { ...initialSummary.active_error!, category, stamp: `ordinary-${category}` },
      });

      expect(toastMock).not.toHaveBeenCalled();
    },
  );

  it("keeps the hydrated status summary as the initial context value", () => {
    renderProvider(initialSummary);

    expect(screen.getByTestId("summary-stamp").textContent).toBe("initial");
  });

  it("replaces the hydrated summary with a live task projection", () => {
    const liveSummary: TaskStatusSummary = {
      ...initialSummary,
      active_error: {
        ...initialSummary.active_error!,
        stamp: "live",
      },
    };
    statusSummaryMock.mockReturnValue(liveSummary);

    renderProvider(initialSummary);

    expect(screen.getByTestId("summary-stamp").textContent).toBe("live");
    expect(statusSummaryMock).toHaveBeenCalledWith("task-1", initialSummary);
  });
});

it("announces new session failure stamps without announcing loaded history", () => {
  const value = { taskId: "task-1", workspaceId: WORKSPACE_ID, statusSummary: initialSummary };
  const { rerender } = render(
    <TaskLaunchErrorProvider value={value}>
      <SummaryStamp />
    </TaskLaunchErrorProvider>,
  );
  expect(screen.queryByTestId("session-error-announcement")?.textContent ?? "").toBe("");
  const next = {
    ...value,
    statusSummary: {
      ...initialSummary,
      active_error: {
        ...initialSummary.active_error!,
        scope: "session" as const,
        session_id: "session-1",
        stamp: "new-error",
      },
    },
  };
  rerender(
    <TaskLaunchErrorProvider value={next}>
      <SummaryStamp />
    </TaskLaunchErrorProvider>,
  );
  expect(screen.getByTestId("session-error-announcement").textContent).not.toBe("");
});

it("treats late initial history as a baseline for announcements", () => {
  const value = { taskId: "task-1", workspaceId: WORKSPACE_ID };
  const { rerender } = render(
    <TaskLaunchErrorProvider value={value}>
      <SummaryStamp />
    </TaskLaunchErrorProvider>,
  );
  const summary = {
    ...initialSummary,
    active_error: {
      ...initialSummary.active_error!,
      scope: "session" as const,
      session_id: "session-1",
    },
  };
  rerender(
    <TaskLaunchErrorProvider value={{ ...value, statusSummary: summary }}>
      <SummaryStamp />
    </TaskLaunchErrorProvider>,
  );
  expect(screen.getByTestId("session-error-announcement").textContent).toBe("");
});

function RevealRecovery() {
  const context = useTaskLaunchErrorContext();
  return (
    <button onClick={() => context?.revealSessionRecovery?.("session-1")}>Reveal recovery</button>
  );
}
it("opens the session Chat panel and focuses its recovery owner", () => {
  render(
    <TaskLaunchErrorProvider value={{ taskId: "task-1", workspaceId: WORKSPACE_ID }}>
      <RevealRecovery />
      <div id="session-recovery-session-1" tabIndex={-1}>
        Recovery
      </div>
    </TaskLaunchErrorProvider>,
  );
  const owner = document.getElementById("session-recovery-session-1")!;
  Object.defineProperty(owner, "checkVisibility", { value: () => true });
  fireEvent.click(screen.getByRole("button", { name: "Reveal recovery" }));
  expect(navigation.mobile).toHaveBeenCalledWith("session-1", "chat");
  expect(navigation.activate).toHaveBeenCalled();
  expect(document.activeElement).toBe(owner);
});

vi.mock("@/lib/state/dockview-panel-actions", () => ({
  addSessionPanel: (...args: unknown[]) => navigation.add(...args),
}));
it("opens the requested failed session when its panel is closed", () => {
  navigation.getPanel.mockReturnValueOnce(undefined as never);
  render(
    <TaskLaunchErrorProvider value={{ taskId: "task-1", workspaceId: WORKSPACE_ID }}>
      <RevealRecovery />
    </TaskLaunchErrorProvider>,
  );
  fireEvent.click(screen.getByRole("button", { name: "Reveal recovery" }));
  expect(navigation.select).toHaveBeenCalledWith("task-1", "session-1");
  expect(navigation.add).toHaveBeenCalledWith(
    expect.anything(),
    "center",
    "session-1",
    expect.any(String),
  );
});
const WORKSPACE_ID = "workspace-1";
