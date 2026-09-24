import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { EditorsMenu } from "./editors-menu";
import type { EditorOption } from "@/lib/types/http";

const fixtures = vi.hoisted(() => ({
  folderOpeningAvailable: true,
  editors: [] as EditorOption[],
  openEditor: vi.fn(),
  worktrees: [] as { id: string; repositoryId: string; path: string; branch: string }[],
  openSessionFolder: vi.fn(),
  toast: vi.fn(),
}));
vi.mock("@/hooks/domains/settings/use-editors", () => ({
  useEditors: () => ({
    editors: fixtures.editors,
    folderOpeningAvailable: fixtures.folderOpeningAvailable,
  }),
}));
vi.mock("@/hooks/use-open-session-in-editor", () => ({
  useOpenSessionInEditor: () => ({ open: fixtures.openEditor, isLoading: false }),
}));
vi.mock("@/lib/api", () => ({ openSessionFolder: fixtures.openSessionFolder }));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: fixtures.toast }) }));
vi.mock("@/hooks/domains/session/use-session-worktrees", () => ({
  useSessionWorktrees: () => fixtures.worktrees,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (select: (state: unknown) => unknown) =>
    select({
      repositories: { itemsByWorkspaceId: {} },
      userSettings: { defaultEditorId: "editor-1" },
    }),
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false, isFinePointer: true }),
}));

const OPEN_FOLDER_LABEL = "Open folder";

function view(sessionId: string | null) {
  return (
    <TooltipProvider>
      <EditorsMenu activeSessionId={sessionId} embeddedVscodeSupported={false} />
    </TooltipProvider>
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  fixtures.worktrees = [];
  fixtures.editors = [];
  fixtures.folderOpeningAvailable = true;
  fixtures.openSessionFolder.mockResolvedValue({ success: true });
});
afterEach(cleanup);

// @covers AC-TASKS-OPEN-FOLDER-001.1, AC-TASKS-OPEN-FOLDER-001.2, AC-TASKS-OPEN-FOLDER-001.3
async function openMenu() {
  fireEvent.keyDown(screen.getByTestId("editors-menu-list"), { key: "ArrowDown" });
  return screen.findByRole("menu");
}
async function chooseFolder() {
  await openMenu();
  fireEvent.click(screen.getByRole("menuitem", { name: OPEN_FOLDER_LABEL }));
  await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
}

describe("task folder action", () => {
  it("opens without any editor configuration", async () => {
    render(view("s1"));
    expect((screen.getByTestId("editors-menu-list") as HTMLButtonElement).disabled).toBe(false);
    await chooseFolder();
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
    expect((screen.getByTestId("editors-menu-list") as HTMLButtonElement).disabled).toBe(true);
  });
  it("sends the only worktree explicitly", async () => {
    fixtures.worktrees = [{ id: "wt-1", repositoryId: "r1", path: "/repo-one", branch: "main" }];
    render(view("s1"));
    await chooseFolder();
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
    await chooseFolder();
    expect(fixtures.openSessionFolder).not.toHaveBeenCalled();
    fireEvent.click(await screen.findByRole("button", { name: /repo-two/ }));
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
    await chooseFolder();
    expect(await screen.findByRole("dialog")).toBeTruthy();
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
    await chooseFolder();
    fireEvent.click(await screen.findByRole("button", { name: "Cancel" }));
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
    await chooseFolder();
    await openMenu();
    expect(
      screen.getByRole("menuitem", { name: OPEN_FOLDER_LABEL }).getAttribute("aria-disabled"),
    ).toBe("true");
    await act(async () => {
      finish({ success: true });
    });
    expect(
      screen.getByRole("menuitem", { name: OPEN_FOLDER_LABEL }).getAttribute("aria-disabled"),
    ).not.toBe("true");
  });
});

it("blocks the picker and requests when the host opener is missing", async () => {
  fixtures.folderOpeningAvailable = false;
  fixtures.worktrees = [
    { id: "wt-1", repositoryId: "r1", path: "/one", branch: "main" },
    { id: "wt-2", repositoryId: "r2", path: "/two", branch: "main" },
  ];
  render(view("s1"));
  await openMenu();
  const button = screen.getByRole("menuitem", { name: OPEN_FOLDER_LABEL });
  expect(button.getAttribute("aria-disabled")).toBe("true");
  fireEvent.click(button);
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(fixtures.openSessionFolder).not.toHaveBeenCalled();
});

it("reports folder errors and permits retry without launching an editor", async () => {
  fixtures.openSessionFolder.mockRejectedValueOnce(new Error("host failure"));
  render(view("s1"));
  await chooseFolder();
  await waitFor(() =>
    expect(fixtures.toast).toHaveBeenCalledWith(
      expect.objectContaining({ title: "Failed to open folder", variant: "error" }),
    ),
  );
  await chooseFolder();
  await waitFor(() => expect(fixtures.openSessionFolder).toHaveBeenCalledTimes(2));
  expect(fixtures.openEditor).not.toHaveBeenCalled();
});

it("keeps editor launching available when the folder opener is unavailable", async () => {
  fixtures.folderOpeningAvailable = false;
  fixtures.editors = [
    {
      id: "editor-1",
      type: "editor",
      name: "Test editor",
      kind: "custom",
      installed: true,
      enabled: true,
    },
  ];
  render(view("s1"));
  fireEvent.click(screen.getByTestId("editors-menu-open"));
  expect(fixtures.openEditor).toHaveBeenCalledWith({ editorId: "editor-1", worktreeId: undefined });
  await openMenu();
  fireEvent.click(screen.getByRole("menuitem", { name: "Test editor" }));
  expect(fixtures.openEditor).toHaveBeenCalledTimes(2);
  expect(fixtures.openSessionFolder).not.toHaveBeenCalled();
});
