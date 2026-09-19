import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { OpenTaskFolderButton } from "./open-task-folder-button";

const fixtures = vi.hoisted(() => ({
  folderOpeningAvailable: true,
  worktrees: [] as { id: string; repositoryId: string; path: string; branch: string }[],
  openSessionFolder: vi.fn(),
  toast: vi.fn(),
}));
vi.mock("@/hooks/domains/settings/use-editors", () => ({
  useEditors: () => ({ folderOpeningAvailable: fixtures.folderOpeningAvailable }),
}));
vi.mock("@/lib/api", () => ({ openSessionFolder: fixtures.openSessionFolder }));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: fixtures.toast }) }));
vi.mock("@/hooks/domains/session/use-session-worktrees", () => ({
  useSessionWorktrees: () => fixtures.worktrees,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (select: (state: unknown) => unknown) =>
    select({ repositories: { itemsByWorkspaceId: {} } }),
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false, isFinePointer: true }),
}));

const OPEN_FOLDER_LABEL = "Open folder";

function view(sessionId: string | null) {
  return (
    <TooltipProvider>
      <OpenTaskFolderButton sessionId={sessionId} />
    </TooltipProvider>
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  fixtures.worktrees = [];
  fixtures.folderOpeningAvailable = true;
  fixtures.openSessionFolder.mockResolvedValue({ success: true });
});
afterEach(cleanup);

// @covers AC-TASKS-OPEN-FOLDER-001.1, AC-TASKS-OPEN-FOLDER-001.2, AC-TASKS-OPEN-FOLDER-001.3
describe("task folder action", () => {
  it("opens without any editor configuration", async () => {
    render(view("s1"));
    fireEvent.click(screen.getByRole("button", { name: OPEN_FOLDER_LABEL }));
    await waitFor(() =>
      expect(fixtures.openSessionFolder).toHaveBeenCalledWith(
        "s1",
        { cache: "no-store" },
        undefined,
      ),
    );
  });
  it("disables opening without a session", () => {
    render(view(null));
    expect(
      (screen.getByRole("button", { name: OPEN_FOLDER_LABEL }) as HTMLButtonElement).disabled,
    ).toBe(true);
  });
  it("sends the only worktree explicitly", async () => {
    fixtures.worktrees = [{ id: "wt-1", repositoryId: "r1", path: "/repo-one", branch: "main" }];
    render(view("s1"));
    fireEvent.click(screen.getByRole("button", { name: OPEN_FOLDER_LABEL }));
    await waitFor(() =>
      expect(fixtures.openSessionFolder).toHaveBeenCalledWith(
        "s1",
        { cache: "no-store" },
        { worktree_id: "wt-1" },
      ),
    );
  });
  it("requires a selection and opens only the chosen worktree", async () => {
    fixtures.worktrees = [
      { id: "wt-1", repositoryId: "r1", path: "/repo-one", branch: "main" },
      { id: "wt-2", repositoryId: "r2", path: "/repo-two", branch: "feature" },
    ];
    render(view("s1"));
    fireEvent.click(screen.getByRole("button", { name: OPEN_FOLDER_LABEL }));
    expect(fixtures.openSessionFolder).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: /repo-two/ }));
    await waitFor(() =>
      expect(fixtures.openSessionFolder).toHaveBeenCalledWith(
        "s1",
        { cache: "no-store" },
        { worktree_id: "wt-2" },
      ),
    );
  });
  it("dismisses an open picker when the session changes", async () => {
    fixtures.worktrees = [
      { id: "wt-1", repositoryId: "r1", path: "/repo-one", branch: "main" },
      { id: "wt-2", repositoryId: "r2", path: "/repo-two", branch: "feature" },
    ];
    const { rerender } = render(view("s1"));
    fireEvent.click(screen.getByRole("button", { name: OPEN_FOLDER_LABEL }));
    expect(screen.getByRole("dialog")).toBeTruthy();
    rerender(view("s2"));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(fixtures.openSessionFolder).not.toHaveBeenCalled();
    rerender(view("s1"));
    expect(screen.queryByRole("dialog")).toBeNull();
  });
  it("cancels a picker without opening a folder", async () => {
    fixtures.worktrees = [
      { id: "wt-1", repositoryId: "r1", path: "/repo-one", branch: "main" },
      { id: "wt-2", repositoryId: "r2", path: "/repo-two", branch: "feature" },
    ];
    render(view("s1"));
    fireEvent.click(screen.getByRole("button", { name: OPEN_FOLDER_LABEL }));
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(fixtures.openSessionFolder).not.toHaveBeenCalled();
  });
  it("disables the action until opening completes", async () => {
    let finish!: (value: { success: boolean }) => void;
    fixtures.openSessionFolder.mockReturnValueOnce(
      new Promise((resolve) => {
        finish = resolve;
      }),
    );
    render(view("s1"));
    const button = screen.getByRole("button", { name: OPEN_FOLDER_LABEL });
    fireEvent.click(button);
    expect((button as HTMLButtonElement).disabled).toBe(true);
    await act(async () => {
      finish({ success: true });
    });
    expect((button as HTMLButtonElement).disabled).toBe(false);
  });
});

it("blocks the picker and requests when the host opener is missing", () => {
  fixtures.folderOpeningAvailable = false;
  fixtures.worktrees = [
    { id: "wt-1", repositoryId: "r1", path: "/one", branch: "main" },
    { id: "wt-2", repositoryId: "r2", path: "/two", branch: "main" },
  ];
  render(view("s1"));
  const button = screen.getByRole("button", { name: OPEN_FOLDER_LABEL }) as HTMLButtonElement;
  expect(button.disabled).toBe(true);
  fireEvent.click(button);
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(fixtures.openSessionFolder).not.toHaveBeenCalled();
});
