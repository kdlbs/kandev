import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { renderToString } from "react-dom/server";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const replaceMock = vi.hoisted(() => vi.fn());
const saveProfileMock = vi.hoisted(() => vi.fn());
const kanbanWithPreviewMock = vi.hoisted(() => vi.fn(() => null));
const startupPageMock = vi.hoisted(() => ({ value: "task_overview" }));
const recentTasksMock = vi.hoisted(() => ({
  entries: [] as Array<{ taskId: string; workspaceId: string }>,
}));
const getRecentTasksMock = vi.hoisted(() => vi.fn());
const searchMock = vi.hoisted(() => ({ value: "" }));
const preferredViewMock = vi.hoisted(() => ({ value: "list" }));
const breakpointMock = vi.hoisted(() => ({ isMobile: false }));

const ONBOARDING_COMPLETED_KEY = "kandev.onboarding.completed";
const CHANGED_PROFILE_MODEL = "changed-model";

vi.mock("@/components/kanban-with-preview", () => ({
  KanbanWithPreview: kanbanWithPreviewMock,
}));
vi.mock("@/components/onboarding-dialog", () => ({
  OnboardingDialog: ({ open, onComplete }: { open: boolean; onComplete: () => void }) => {
    const [profileModel, setProfileModel] = useState("default-model");
    if (!open) return null;
    return (
      <div role="dialog" aria-label="First-run onboarding">
        <button onClick={() => setProfileModel(CHANGED_PROFILE_MODEL)}>Make profile dirty</button>
        <output data-testid="onboarding-profile-model">{profileModel}</output>
        <button onClick={() => saveProfileMock(profileModel)}>Next</button>
        <button onClick={onComplete}>Get started</button>
      </div>
    );
  },
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => breakpointMock,
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
  breakpointMock.isMobile = false;
  window.localStorage.removeItem(ONBOARDING_COMPLETED_KEY);
});

afterEach(() => {
  cleanup();
  replaceMock.mockReset();
  saveProfileMock.mockReset();
  kanbanWithPreviewMock.mockClear();
  getRecentTasksMock.mockReset();
  startupPageMock.value = "task_overview";
  recentTasksMock.entries = [];
  searchMock.value = "";
  preferredViewMock.value = "list";
  breakpointMock.isMobile = false;
  window.localStorage.removeItem(ONBOARDING_COMPLETED_KEY);
});

describe("PageClient", () => {
  // @covers AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.4, 003.5, 003.7
  it.each(["kanban", "pipeline", "list", "threads"])(
    "prefers fixed Threads over remembered %s",
    async (view) => {
      startupPageMock.value = "threads";
      preferredViewMock.value = view;
      render(<PageClient workspaceId="workspace-1" />);
      await waitFor(() =>
        expect(replaceMock).toHaveBeenCalledWith("/threads?workspace=workspace-1"),
      );
      expect(
        replaceMock.mock.calls.every(([href]) => href === "/threads?workspace=workspace-1"),
      ).toBe(true);
    },
  );

  it("lets explicit overview use the remembered List despite fixed Threads", async () => {
    startupPageMock.value = "threads";
    searchMock.value = "home=overview";
    render(<PageClient workspaceId="workspace-1" />);
    await waitFor(() => expect(replaceMock).toHaveBeenCalledWith("/tasks?workspace=workspace-1"));
  });

  it("keeps explicit task and session props with fixed Threads", () => {
    startupPageMock.value = "threads";
    searchMock.value = "";
    const { rerender } = render(<PageClient workspaceId="workspace-1" initialTaskId="task-1" />);
    expect(replaceMock).not.toHaveBeenCalled();
    searchMock.value = "";
    rerender(<PageClient workspaceId="workspace-1" initialSessionId="session-1" />);
    expect(replaceMock).not.toHaveBeenCalled();
  });

  it.each(["workflowId=wf-1", "taskId=task-1", "sessionId=session-1"])(
    "does not restore a listing over query %s",
    (query) => {
      startupPageMock.value = "threads";
      searchMock.value = query;
      render(<PageClient workspaceId="workspace-1" />);
      expect(replaceMock).not.toHaveBeenCalled();
    },
  );

  it("keeps onboarding available without a resolved workspace", () => {
    startupPageMock.value = "threads";
    preferredViewMock.value = "kanban";
    render(<PageClient />);
    expect(replaceMock).not.toHaveBeenCalled();
  });

  // @covers AC-UI-FIRST-RUN-DIALOG-001.1, 001.2
  it.each([
    ["unfinished", null],
    ["completed", "true"],
  ])("does not present first-run onboarding on phones for %s users", (_state, marker) => {
    breakpointMock.isMobile = true;
    if (marker) window.localStorage.setItem(ONBOARDING_COMPLETED_KEY, marker);

    render(<PageClient />);

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(window.localStorage.getItem(ONBOARDING_COMPLETED_KEY)).toBe(marker);
  });

  // @covers AC-UI-FIRST-RUN-DIALOG-001.3, 001.4
  it("preserves an edited profile without completing the tour across phone resizes", () => {
    const page = render(<PageClient />);

    expect(screen.getByRole("dialog", { name: "First-run onboarding" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Make profile dirty" }));
    expect(screen.getByTestId("onboarding-profile-model").textContent).toBe(CHANGED_PROFILE_MODEL);

    breakpointMock.isMobile = true;
    page.rerender(<PageClient />);

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(window.localStorage.getItem(ONBOARDING_COMPLETED_KEY)).toBeNull();

    breakpointMock.isMobile = false;
    page.rerender(<PageClient />);

    expect(screen.getByRole("dialog", { name: "First-run onboarding" })).toBeTruthy();
    expect(screen.getByTestId("onboarding-profile-model").textContent).toBe(CHANGED_PROFILE_MODEL);
    expect(saveProfileMock).not.toHaveBeenCalled();
    expect(window.localStorage.getItem(ONBOARDING_COMPLETED_KEY)).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    expect(saveProfileMock).toHaveBeenCalledWith(CHANGED_PROFILE_MODEL);
  });

  // @covers AC-UI-FIRST-RUN-DIALOG-001.5
  it("keeps desktop completion behavior after the first-run tour reopens", () => {
    breakpointMock.isMobile = true;
    const page = render(<PageClient />);

    breakpointMock.isMobile = false;
    page.rerender(<PageClient />);
    fireEvent.click(screen.getByRole("button", { name: "Get started" }));

    expect(window.localStorage.getItem(ONBOARDING_COMPLETED_KEY)).toBe("true");
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});

describe("PageClient existing startup choices", () => {
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
