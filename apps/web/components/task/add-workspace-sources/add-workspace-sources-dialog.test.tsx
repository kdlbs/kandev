import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useEffect, useRef, useState } from "react";
import { AddWorkspaceSourcesDialog } from "./add-workspace-sources-dialog";
import { StateProvider, useAppStore, useAppStoreApi } from "@/components/state-provider";
import { sessionId as toSessionId, taskId as toTaskId } from "@/lib/types/http";
import { TooltipProvider } from "@kandev/ui/tooltip";

let isMobile = false;
const ADD_SOURCES_LABEL = "Add sources";
const { attachTaskWorkspaceSources, discoverRepositoriesAction, refreshRepositories } = vi.hoisted(
  () => ({
    attachTaskWorkspaceSources: vi.fn(),
    discoverRepositoriesAction: vi.fn().mockResolvedValue({ repositories: [] }),
    refreshRepositories: vi.fn().mockResolvedValue(undefined),
  }),
);

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile }),
}));
vi.mock("@/hooks/domains/workspace/use-repositories", () => ({
  useRepositories: () => ({
    repositories: [],
    isLoading: false,
    refresh: refreshRepositories,
  }),
}));
vi.mock("@/components/folder-picker", () => ({
  FolderPicker: ({ onChange }: { onChange: (path: string) => void }) => (
    <button type="button" onClick={() => onChange("/sources/docs")}>
      Choose local folder
    </button>
  ),
}));
vi.mock("@/lib/api/domains/kanban-api", () => ({ attachTaskWorkspaceSources }));
vi.mock("@/app/actions/workspaces", () => ({ discoverRepositoriesAction }));

async function finishClose(surface: HTMLElement, isDrawer: boolean) {
  await waitFor(() => expect(surface.getAttribute("data-state")).not.toBe("open"));
  if (isDrawer) fireEvent.animationEnd(surface);
  await waitFor(() => expect(surface.isConnected).toBe(false));
}

function openRepositoryMenu() {
  const trigger = screen.getByRole("button", { name: "Add repository" });
  fireEvent.pointerDown(trigger, { button: 0, ctrlKey: false });
  fireEvent.click(trigger);
  return trigger;
}

function Harness(props: { makeTurnActive?: boolean; executorType?: string | null }) {
  const makeTurnActive = props.makeTurnActive ?? false;
  const executorType = Object.hasOwn(props, "executorType") ? props.executorType : "worktree";
  return (
    <StateProvider>
      <HarnessContent makeTurnActive={makeTurnActive} executorType={executorType} />
    </StateProvider>
  );
}

function HarnessContent({
  makeTurnActive,
  executorType,
}: {
  makeTurnActive: boolean;
  executorType: string | null | undefined;
}) {
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
        {ADD_SOURCES_LABEL}
      </button>
      <AddWorkspaceSourcesDialog
        open={open}
        onOpenChange={setOpen}
        taskId="task-1"
        executorType={executorType}
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
  discoverRepositoriesAction.mockClear();
  refreshRepositories.mockClear();
});

describe("AddWorkspaceSourcesDialog consequences", () => {
  it.each([
    ["desktop", false, "add-workspace-sources-dialog"],
    ["mobile", true, "add-workspace-sources-drawer"],
  ])("explains the workspace and session consequences on %s", async (_, mobile, surfaceTestId) => {
    isMobile = mobile;
    render(<Harness />);

    fireEvent.click(screen.getByRole("button", { name: ADD_SOURCES_LABEL }));
    const surface = await screen.findByTestId(surfaceTestId);
    const consequences = screen.getByTestId("workspace-change-consequences");

    expect(surface.contains(consequences)).toBe(true);
    expect(screen.getByText("This restarts the task workspace")).toBeTruthy();
    expect(consequences.textContent).toMatch(/task root becomes the agent's working directory/i);
    expect(consequences.textContent).toMatch(
      /terminals, dev servers, and other workspace processes stop/i,
    );
    expect(consequences.textContent).toMatch(
      /provider-private context that Kandev did not record may not carry over/i,
    );
    expect(screen.getByText(/Cancel leaves the workspace unchanged/i)).toBeTruthy();
  });

  it("explains that remote executor sources are attached without restarting the agent", async () => {
    render(<Harness executorType="ssh" />);

    fireEvent.click(screen.getByRole("button", { name: ADD_SOURCES_LABEL }));
    const consequences = await screen.findByTestId("workspace-change-consequences");

    expect(consequences.textContent).toContain("This updates the live task workspace");
    expect(consequences.textContent).toContain(
      "The agent and running workspace processes continue",
    );
    expect(consequences.textContent).not.toContain("This restarts the task workspace");
  });

  it("treats Kubernetes as a live remote workspace", async () => {
    render(<Harness executorType="k8s" />);

    fireEvent.click(screen.getByRole("button", { name: ADD_SOURCES_LABEL }));
    const consequences = await screen.findByTestId("workspace-change-consequences");

    expect(consequences.textContent).toContain("This updates the live task workspace");
    expect(consequences.textContent).not.toContain("This restarts the task workspace");
  });
});

describe("AddWorkspaceSourcesDialog", () => {
  it("uses a touch-sized repository menu without tabs or discarded mixed source rows", async () => {
    isMobile = true;
    render(
      <TooltipProvider>
        <Harness />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: ADD_SOURCES_LABEL }));
    expect(screen.queryByTestId("source-mode-local")).toBeNull();
    expect(screen.queryByTestId("source-mode-remote")).toBeNull();
    const addRepository = screen.getByRole("button", { name: "Add repository" });
    const submit = screen.getByTestId("add-workspace-sources-submit");
    expect(addRepository.className).toContain("min-h-11");
    expect((submit as HTMLButtonElement).disabled).toBe(true);

    fireEvent.click(screen.getByRole("button", { name: "Add folder" }));
    openRepositoryMenu();
    const remoteRepository = await screen.findByRole("menuitem", { name: "Remote repository" });
    expect(remoteRepository.className).toContain("min-h-11");
    fireEvent.click(remoteRepository);

    expect(screen.getByText("Folder")).toBeTruthy();
    expect(screen.getByText("Remote repository")).toBeTruthy();
    expect(screen.queryByText("Choose a repository and base branch.")).toBeNull();
    expect(screen.queryByRole("textbox", { name: "Checkout branch" })).toBeNull();
    expect((submit as HTMLButtonElement).disabled).toBe(false);

    fireEvent.click(submit);

    const validationError = screen.getByText("Choose a repository and base branch.");
    expect(validationError.className).toContain("text-xs");
    const form = screen.getByTestId("add-workspace-sources-form");
    expect(form.querySelectorAll('[role="alert"]')).toHaveLength(2);

    openRepositoryMenu();
    fireEvent.click(await screen.findByRole("menuitem", { name: "Local Git repository" }));
    expect(form.querySelectorAll('[role="alert"]')).toHaveLength(2);
  });

  it.each([null, undefined])(
    "shows Add folder disabled with a touch-visible reason while the executor is unresolved (%s)",
    async (executorType) => {
      render(<Harness executorType={executorType} />);

      fireEvent.click(screen.getByRole("button", { name: ADD_SOURCES_LABEL }));
      const addFolder = screen.getByRole("button", { name: /Add folder/ }) as HTMLButtonElement;
      expect(addFolder.disabled).toBe(true);
      const hint = screen.getByText("Waiting to confirm this task's executor supports folders");
      expect(hint.className).toContain("text-xs");
    },
  );

  it("keeps Add folder absent once the executor is known not to support folders", async () => {
    render(<Harness executorType="local_docker" />);

    fireEvent.click(screen.getByRole("button", { name: ADD_SOURCES_LABEL }));
    expect(screen.queryByRole("button", { name: /Add folder/ })).toBeNull();
  });

  it.each([
    ["desktop", false, "add-workspace-sources-dialog"],
    ["mobile", true, "add-workspace-sources-drawer"],
  ])("returns focus to the external %s opener after Cancel", async (_, mobile, surfaceTestId) => {
    isMobile = mobile;
    render(<Harness />);

    const opener = screen.getByRole("button", { name: ADD_SOURCES_LABEL });
    fireEvent.click(opener);
    const surface = await screen.findByTestId(surfaceTestId);
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    await finishClose(surface, mobile);
    expect(attachTaskWorkspaceSources).not.toHaveBeenCalled();
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

      const opener = screen.getByRole("button", {
        name: ADD_SOURCES_LABEL,
      }) as HTMLButtonElement;
      fireEvent.click(opener);
      const surface = await screen.findByTestId(surfaceTestId);
      await waitFor(() => expect(opener.disabled).toBe(true));
      fireEvent.click(screen.getByRole("button", { name: "Add folder" }));
      fireEvent.click(screen.getByRole("button", { name: "Choose local folder" }));
      fireEvent.click(screen.getByTestId("add-workspace-sources-submit"));

      await waitFor(() => expect(opener.disabled).toBe(false));
      await waitFor(() => expect(surface.getAttribute("data-state")).not.toBe("open"));
      await finishClose(surface, mobile);
      expect(screen.queryByTestId(surfaceTestId)).toBeNull();
      await waitFor(() => expect(document.activeElement).toBe(opener));
      expect(attachTaskWorkspaceSources).toHaveBeenCalledOnce();
    },
  );
});

describe("AddWorkspaceSourcesDialog saved repository picker", () => {
  it("reuses discovered repository, refresh, and create controls from task creation", async () => {
    discoverRepositoriesAction.mockResolvedValueOnce({
      repositories: [
        {
          path: "/projects/discovered-project",
          name: "discovered-project",
          default_branch: "main",
        },
      ],
    });
    render(
      <TooltipProvider>
        <Harness />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: ADD_SOURCES_LABEL }));
    await waitFor(() => expect(discoverRepositoriesAction).toHaveBeenCalledWith("workspace-1"));
    openRepositoryMenu();
    fireEvent.click(await screen.findByRole("menuitem", { name: "Workspace repository" }));
    fireEvent.click(screen.getByTestId("repo-chip-trigger"));

    expect(await screen.findByText("discovered-project")).toBeTruthy();
    expect(screen.getByText("on disk")).toBeTruthy();
    expect(screen.getByTestId("create-local-repository-button")).toBeTruthy();
    fireEvent.click(screen.getByTestId("repo-refresh-button"));
    await waitFor(() => expect(refreshRepositories).toHaveBeenCalledOnce());
  });
});
