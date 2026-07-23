import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useEffect, useRef, useState } from "react";
import { AddWorkspaceSourcesDialog } from "./add-workspace-sources-dialog";
import { StateProvider, useAppStore, useAppStoreApi } from "@/components/state-provider";
import { sessionId as toSessionId, taskId as toTaskId } from "@/lib/types/http";
import { TooltipProvider } from "@kandev/ui/tooltip";

let isMobile = false;

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile }),
}));
vi.mock("@/hooks/domains/workspace/use-repositories", () => ({
  useRepositories: () => ({ repositories: [] }),
}));
vi.mock("@/components/folder-picker", () => ({
  FolderPicker: ({ onChange }: { onChange: (path: string) => void }) => (
    <button type="button" onClick={() => onChange("/sources/docs")}>
      Choose local folder
    </button>
  ),
}));
const { attachTaskWorkspaceSources } = vi.hoisted(() => ({ attachTaskWorkspaceSources: vi.fn() }));
vi.mock("@/lib/api/domains/kanban-api", () => ({ attachTaskWorkspaceSources }));

async function finishClose(surface: HTMLElement, isDrawer: boolean) {
  await waitFor(() => expect(surface.getAttribute("data-state")).not.toBe("open"));
  if (isDrawer) fireEvent.animationEnd(surface);
  await waitFor(() => expect(surface.isConnected).toBe(false));
}

function Harness({ makeTurnActive = false }: { makeTurnActive?: boolean }) {
  return (
    <StateProvider>
      <HarnessContent makeTurnActive={makeTurnActive} />
    </StateProvider>
  );
}

function HarnessContent({ makeTurnActive }: { makeTurnActive: boolean }) {
  const [open, setOpen] = useState(false);
  const [opener, setOpener] = useState<HTMLElement | null>(null);
  const store = useAppStoreApi();
  const isBusy = useAppStore((state) => Boolean(state.turns.activeBySession["session-1"]));
  const activated = useRef(false);
  useEffect(() => {
    if (!open || !makeTurnActive || activated.current) return;
    activated.current = true;
    store.getState().addTurn({
      id: "turn-stale",
      session_id: toSessionId("session-1"),
      task_id: toTaskId("task-1"),
      started_at: "2026-07-23T00:00:00Z",
      created_at: "2026-07-23T00:00:00Z",
      updated_at: "2026-07-23T00:00:00Z",
    });
    store.getState().setActiveTurn("session-1", "turn-stale");
  }, [makeTurnActive, open, store]);
  return (
    <>
      <button
        type="button"
        disabled={isBusy}
        onClick={(event) => {
          setOpener(event.currentTarget);
          setOpen(true);
        }}
      >
        Add sources
      </button>
      <AddWorkspaceSourcesDialog
        open={open}
        onOpenChange={setOpen}
        taskId="task-1"
        executorType="worktree"
        workspaceId="workspace-1"
        opener={opener}
      />
    </>
  );
}

afterEach(() => {
  cleanup();
  isMobile = false;
  attachTaskWorkspaceSources.mockReset();
});

describe("AddWorkspaceSourcesDialog", () => {
  it("uses touch-sized Local and Remote modes without discarding mixed source rows", async () => {
    isMobile = true;
    render(
      <TooltipProvider>
        <Harness />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Add sources" }));
    const local = await screen.findByTestId("source-mode-local");
    expect(local.className).toContain("min-h-11");
    fireEvent.click(screen.getByRole("button", { name: "Local folder" }));
    fireEvent.click(screen.getByTestId("source-mode-remote"));
    fireEvent.click(screen.getByRole("button", { name: "Remote Git repository" }));
    fireEvent.click(local);

    expect(screen.getByText("Folder")).toBeTruthy();
    expect(screen.getByText("Remote Repository")).toBeTruthy();
    expect(screen.getByText("Choose a repository and base branch.")).toBeTruthy();
  });

  it.each([
    ["desktop", false, "add-workspace-sources-dialog"],
    ["mobile", true, "add-workspace-sources-drawer"],
  ])("returns focus to the external %s opener after Cancel", async (_, mobile, surfaceTestId) => {
    isMobile = mobile;
    render(<Harness />);

    const opener = screen.getByRole("button", { name: "Add sources" });
    fireEvent.click(opener);
    const surface = await screen.findByTestId(surfaceTestId);
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    await finishClose(surface, mobile);
    await waitFor(() => expect(document.activeElement).toBe(opener));
  });

  it.each([
    ["desktop", false, "add-workspace-sources-dialog"],
    ["mobile", true, "add-workspace-sources-drawer"],
  ])(
    "reconciles adopted stale work before returning focus to the enabled %s opener",
    async (_, mobile, surfaceTestId) => {
      isMobile = mobile;
      attachTaskWorkspaceSources.mockResolvedValueOnce({
        task_id: "task-1",
        repositories: [],
        workspace_folders: [],
        workspace_path: "/workspace/task-1",
        adopted_session_ids: ["session-1"],
        session_ids: [],
      });
      render(<Harness makeTurnActive />);

      const opener = screen.getByRole("button", { name: "Add sources" }) as HTMLButtonElement;
      fireEvent.click(opener);
      const surface = await screen.findByTestId(surfaceTestId);
      await waitFor(() => expect(opener.disabled).toBe(true));
      fireEvent.click(screen.getByRole("button", { name: "Local folder" }));
      fireEvent.click(screen.getByRole("button", { name: "Choose local folder" }));
      fireEvent.click(screen.getByTestId("add-workspace-sources-submit"));

      await waitFor(() => expect(opener.disabled).toBe(false));
      await waitFor(() => expect(surface.getAttribute("data-state")).not.toBe("open"));
      if (mobile) await waitFor(() => expect(document.activeElement).toBe(opener));
      await finishClose(surface, mobile);
      expect(screen.queryByTestId(surfaceTestId)).toBeNull();
      await waitFor(() => expect(document.activeElement).toBe(opener));
      expect(attachTaskWorkspaceSources).toHaveBeenCalledOnce();
    },
  );
});
