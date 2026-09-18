import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { renderToString } from "react-dom/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ONBOARDING_CHANGED } from "@/hooks/use-kanban-onboarding-complete";
import { getLocalStorage, setLocalStorage } from "@/lib/local-storage";
import { STORAGE_KEYS } from "@/lib/settings/constants";

const replaceMock = vi.hoisted(() => vi.fn());
const kanbanWithPreviewMock = vi.hoisted(() => vi.fn(() => null));
const startupPageMock = vi.hoisted(() => ({ value: "task_overview" }));
const recentTasksMock = vi.hoisted(() => ({
  entries: [] as Array<{ taskId: string; workspaceId: string }>,
}));
const getRecentTasksMock = vi.hoisted(() => vi.fn());
const searchMock = vi.hoisted(() => ({ value: "" }));
const preferredViewMock = vi.hoisted(() => ({ value: "list" }));

vi.mock("@/components/kanban-with-preview", () => ({
  KanbanWithPreview: kanbanWithPreviewMock,
}));
vi.mock("@/components/onboarding-dialog", () => ({
  OnboardingDialog: ({ open, onComplete }: { open: boolean; onComplete: () => void }) =>
    open ? <button onClick={onComplete}>Finish setup</button> : null,
}));
vi.mock("@/hooks/use-task-listing-view", () => ({
  useTaskListingView: () => ({ preferredView: preferredViewMock.value }),
}));
vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ replace: replaceMock }),
  useSearchParams: () => new URLSearchParams(searchMock.value),
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: { userSettings: { startupPage: string } }) => unknown) =>
    selector({ userSettings: { startupPage: startupPageMock.value } }),
}));
vi.mock("@/lib/recent-tasks", () => ({
  getRecentTasks: getRecentTasksMock,
  findMostRecentTaskForWorkspace: (
    entries: Array<{ taskId: string; workspaceId: string }>,
    workspaceId?: string,
  ) => entries.find((entry) => entry.workspaceId === workspaceId) ?? null,
}));

import { PageClient } from "./page-client";

const WORKSPACE_ID = "workspace-1";
const FINISH_SETUP = "Finish setup";

beforeEach(() => {
  setLocalStorage(STORAGE_KEYS.ONBOARDING_COMPLETED, true);
  getRecentTasksMock.mockImplementation(() => recentTasksMock.entries);
});

afterEach(() => {
  localStorage.removeItem(STORAGE_KEYS.ONBOARDING_COMPLETED);
  replaceMock.mockReset();
  kanbanWithPreviewMock.mockClear();
  getRecentTasksMock.mockReset();
  startupPageMock.value = "task_overview";
  recentTasksMock.entries = [];
  searchMock.value = "";
  preferredViewMock.value = "list";
});

describe("PageClient", () => {
  it("keeps startup in onboarding and publishes completion before restoring List", async () => {
    setLocalStorage(STORAGE_KEYS.ONBOARDING_COMPLETED, false);
    const onComplete = vi.fn();
    window.addEventListener(ONBOARDING_CHANGED, onComplete);
    try {
      render(<PageClient workspaceId="workspace-1" />);

      expect(screen.getByRole("button", { name: FINISH_SETUP })).toBeTruthy();
      expect(replaceMock).not.toHaveBeenCalled();

      fireEvent.click(screen.getByRole("button", { name: FINISH_SETUP }));

      await waitFor(() => {
        expect(replaceMock).toHaveBeenCalledWith("/tasks?workspace=workspace-1");
      });
      expect(getLocalStorage(STORAGE_KEYS.ONBOARDING_COMPLETED, false)).toBe(true);
      expect(onComplete).toHaveBeenCalledTimes(1);
      expect(screen.queryByRole("button", { name: FINISH_SETUP })).toBeNull();
    } finally {
      window.removeEventListener(ONBOARDING_CHANGED, onComplete);
    }
  });

  it("waits for onboarding before reopening the last task", async () => {
    setLocalStorage(STORAGE_KEYS.ONBOARDING_COMPLETED, false);
    startupPageMock.value = "last_task";
    recentTasksMock.entries = [{ taskId: "last-task", workspaceId: WORKSPACE_ID }];

    render(<PageClient workspaceId="workspace-1" />);
    expect(replaceMock).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: FINISH_SETUP }));

    await waitFor(() => {
      expect(replaceMock).toHaveBeenCalledWith("/t/last-task");
    });
  });

  it("restores List in the resolved workspace", async () => {
    render(<PageClient workspaceId="workspace-1" />);

    await waitFor(() => {
      expect(replaceMock).toHaveBeenCalledWith("/tasks?workspace=workspace-1");
    });
  });

  it("does not restore List while opening a task", async () => {
    render(<PageClient workspaceId="workspace-1" initialTaskId="task-1" />);

    await waitFor(() => {
      expect(replaceMock).not.toHaveBeenCalled();
    });
  });

  it("does not restore List while opening a session", async () => {
    render(<PageClient workspaceId="workspace-1" initialSessionId="session-1" />);

    await waitFor(() => {
      expect(replaceMock).not.toHaveBeenCalled();
    });
  });

  it("replaces bare startup with the newest recent task in the active workspace", async () => {
    startupPageMock.value = "last_task";
    recentTasksMock.entries = [
      { taskId: "foreign-task", workspaceId: "workspace-2" },
      { taskId: "last-task", workspaceId: WORKSPACE_ID },
    ];

    render(<PageClient workspaceId="workspace-1" />);

    await waitFor(() => {
      expect(replaceMock).toHaveBeenCalledWith("/t/last-task");
    });
    expect(kanbanWithPreviewMock).not.toHaveBeenCalled();
  });

  it("does not read browser recent tasks during server rendering", () => {
    startupPageMock.value = "last_task";
    recentTasksMock.entries = [{ taskId: "last-task", workspaceId: WORKSPACE_ID }];

    const markup = renderToString(<PageClient workspaceId="workspace-1" />);

    expect(markup).toContain("Opening last task…");
    expect(getRecentTasksMock).not.toHaveBeenCalled();
  });

  it("restores Threads in the resolved workspace", async () => {
    preferredViewMock.value = "threads";

    render(<PageClient workspaceId="workspace-1" />);

    await waitFor(() => {
      expect(replaceMock).toHaveBeenCalledWith("/threads?workspace=workspace-1");
    });
  });

  it("stays on the board when the remembered view is Kanban", async () => {
    preferredViewMock.value = "kanban";

    render(<PageClient workspaceId="workspace-1" />);

    await waitFor(() => {
      expect(kanbanWithPreviewMock).toHaveBeenCalled();
    });
    expect(replaceMock).not.toHaveBeenCalled();
  });

  it("keeps an explicit overview entry from resuming the last task", async () => {
    startupPageMock.value = "last_task";
    recentTasksMock.entries = [{ taskId: "last-task", workspaceId: WORKSPACE_ID }];
    searchMock.value = "home=overview";

    render(<PageClient workspaceId="workspace-1" />);

    await waitFor(() => {
      expect(replaceMock).toHaveBeenCalledWith("/tasks?workspace=workspace-1");
    });
    expect(replaceMock).not.toHaveBeenCalledWith("/t/last-task");
  });
});
