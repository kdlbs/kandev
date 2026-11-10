import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { defaultState } from "@/lib/state/default-state";
import { repositoryId, sessionId, taskId, type TaskSession } from "@/lib/types/http";

const fixtures = vi.hoisted(() => ({ open: vi.fn(), mobile: false }));
vi.mock("@/lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api")>()),
  openSessionInEditor: fixtures.open,
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: fixtures.mobile }),
}));
vi.mock("@/hooks/use-global-view-mode", () => ({ useGlobalViewMode: () => ["split", vi.fn()] }));
vi.mock("@/components/editors/external-vcs-file-link", () => ({
  ExternalVcsFileLink: () => null,
  ExternalVcsFileMenuItem: () => null,
}));

import { FileDiffToolbar } from "./review-diff-toolbar";

const sid = sessionId("review-session");
const secondaryId = "secondary-repo";
const secondaryWorktree = "secondary-worktree";
const filePath = "src/shared.ts";
const session: TaskSession = {
  id: sid,
  task_id: taskId("review-task"),
  state: "WAITING_FOR_INPUT",
  repository_id: repositoryId("primary-repo"),
  worktree_id: "primary-worktree",
  worktree_path: "/task/primary",
  worktrees: [
    {
      id: "association-1",
      session_id: sid,
      worktree_id: "primary-worktree",
      repository_id: repositoryId("primary-repo"),
      worktree_path: "/task/primary",
      position: 0,
    },
    {
      id: "association-2",
      session_id: sid,
      worktree_id: secondaryWorktree,
      repository_id: repositoryId(secondaryId),
      worktree_path: "/task/secondary",
      position: 1,
    },
  ],
} as TaskSession;

function renderToolbar(id?: string, repo?: string) {
  const initialState = {
    ...defaultState,
    taskSessions: { ...defaultState.taskSessions, items: { [sid]: session } },
    editors: {
      ...defaultState.editors,
      loaded: true,
      folderOpeningAvailable: true,
      items: [
        {
          id: "vscode",
          name: "VS Code",
          type: "vscode",
          kind: "built_in",
          enabled: true,
          installed: true,
        },
      ],
    },
    userSettings: { ...defaultState.userSettings, loaded: true },
  };
  const view = render(
    <StateProvider initialState={initialState}>
      <ToastProvider>
        <TooltipProvider>
          <FileDiffToolbar
            filePath={filePath}
            sessionId={sid}
            repositoryId={id}
            repo={repo}
            source="pr"
            wordWrap={false}
            expandUnchanged={false}
            onDiscard={vi.fn()}
            onToggleExpandUnchanged={vi.fn()}
            onToggleWordWrap={vi.fn()}
          />
        </TooltipProvider>
      </ToastProvider>
    </StateProvider>,
  );
  const trigger = fixtures.mobile
    ? screen.getByRole("button", { name: "More actions for src/shared.ts" })
    : view.container.querySelector(".tabler-icon-external-link")?.closest("button");
  if (!trigger) throw new Error("File actions trigger not found");
  fireEvent.pointerDown(trigger, { button: 0, ctrlKey: false });
  if (fixtures.mobile) fireEvent.click(trigger);
  fireEvent.click(screen.getByRole("menuitem", { name: /VS Code/ }));
}

afterEach(() => {
  cleanup();
  fixtures.open.mockReset();
  fixtures.mobile = false;
});

describe("Review external file target", () => {
  // @covers AC-UI-EXTERNAL-FILE-TARGET-001.1
  it("secondary repository routes the selected file", async () => {
    fixtures.open.mockResolvedValue({});
    renderToolbar(secondaryId, "secondary");
    await waitFor(() => expect(fixtures.open).toHaveBeenCalledOnce());
    expect(fixtures.open.mock.calls[0][1]).toMatchObject({
      file_path: filePath,
      worktree_id: secondaryWorktree,
    });
  });

  it("primary repository control", async () => {
    fixtures.open.mockResolvedValue({});
    renderToolbar("primary-repo", "primary");
    await waitFor(() => expect(fixtures.open).toHaveBeenCalledOnce());
    expect(fixtures.open.mock.calls[0][1]).toMatchObject({ file_path: filePath });
    expect([undefined, "primary-worktree"]).toContain(fixtures.open.mock.calls[0][1].worktree_id);
  });
  it("phone overflow routes the secondary file", async () => {
    fixtures.mobile = true;
    fixtures.open.mockResolvedValue({});
    renderToolbar(secondaryId, "secondary");
    await waitFor(() => expect(fixtures.open).toHaveBeenCalledOnce());
    expect(fixtures.open.mock.calls[0][1]).toMatchObject({
      file_path: filePath,
      worktree_id: secondaryWorktree,
    });
  });

  it("routes an explicit secondary ID without a name", async () => {
    fixtures.open.mockResolvedValue({});
    renderToolbar(secondaryId);
    await waitFor(() => expect(fixtures.open).toHaveBeenCalledOnce());
    expect(fixtures.open.mock.calls[0][1]).toMatchObject({
      file_path: filePath,
      worktree_id: secondaryWorktree,
    });
  });
});
