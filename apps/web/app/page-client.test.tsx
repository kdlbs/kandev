import { render, waitFor } from "@testing-library/react";
import { renderToString } from "react-dom/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

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
  OnboardingDialog: () => null,
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

beforeEach(() => {
  getRecentTasksMock.mockImplementation(() => recentTasksMock.entries);
});

afterEach(() => {
  replaceMock.mockReset();
  kanbanWithPreviewMock.mockClear();
  getRecentTasksMock.mockReset();
  startupPageMock.value = "task_overview";
  recentTasksMock.entries = [];
  searchMock.value = "";
  preferredViewMock.value = "list";
});

describe("PageClient", () => {
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
      { taskId: "last-task", workspaceId: "workspace-1" },
    ];

    render(<PageClient workspaceId="workspace-1" />);

    await waitFor(() => {
      expect(replaceMock).toHaveBeenCalledWith("/t/last-task");
    });
    expect(kanbanWithPreviewMock).not.toHaveBeenCalled();
  });

  it("does not read browser recent tasks during server rendering", () => {
    startupPageMock.value = "last_task";
    recentTasksMock.entries = [{ taskId: "last-task", workspaceId: "workspace-1" }];

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
    recentTasksMock.entries = [{ taskId: "last-task", workspaceId: "workspace-1" }];
    searchMock.value = "home=overview";

    render(<PageClient workspaceId="workspace-1" />);

    await waitFor(() => {
      expect(replaceMock).toHaveBeenCalledWith("/tasks?workspace=workspace-1");
    });
    expect(replaceMock).not.toHaveBeenCalledWith("/t/last-task");
  });
});
