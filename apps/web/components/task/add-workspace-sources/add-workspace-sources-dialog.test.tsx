import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useEffect, useRef, useState } from "react";
import { AddWorkspaceSourcesDialog } from "./add-workspace-sources-dialog";
import { StateProvider, useAppStore, useAppStoreApi } from "@/components/state-provider";
import { sessionId as toSessionId, taskId as toTaskId } from "@/lib/types/http";
import type { Repository } from "@/lib/types/http";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { repositoryDiscoveryCoordinator } from "@/hooks/domains/workspace/use-repository-discovery";

let isMobile = false;
const ADD_SOURCES_LABEL = "Add sources";
const WORKSPACE_REPOSITORY_LABEL = "Workspace repository";
const REPO_CHIP_TRIGGER_TEST_ID = "repo-chip-trigger";
const ADD_WORKSPACE_SOURCES_SUBMIT_TEST_ID = "add-workspace-sources-submit";
const TEST_TIMESTAMP = "2026-01-01T00:00:00Z";
const SURFACE_CASES = [
  ["desktop", false, "add-workspace-sources-dialog"],
  ["mobile", true, "add-workspace-sources-drawer"],
] as const;
const {
  attachTaskWorkspaceSources,
  discoverRepositoriesAction,
  getRepositoryDiscoveryAction,
  previewTaskWorkspaceSources,
  refreshRepositoryDiscoveryAction,
  refreshRepositories,
  savedRepositories,
} = vi.hoisted(() => {
  const discover = vi.fn().mockResolvedValue({ repositories: [] });
  return {
    attachTaskWorkspaceSources: vi.fn(),
    discoverRepositoriesAction: discover,
    getRepositoryDiscoveryAction: discover,
    previewTaskWorkspaceSources: vi.fn().mockResolvedValue(null),
    refreshRepositoryDiscoveryAction: discover,
    refreshRepositories: vi.fn().mockResolvedValue(undefined),
    savedRepositories: [] as Repository[],
  };
});

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile }),
}));
vi.mock("@/hooks/domains/workspace/use-repository-branches", () => ({
  useBranches: () => ({
    branches: [{ name: "main", type: "local" }],
    isLoaded: true,
    isLoading: false,
  }),
}));
vi.mock("@/hooks/domains/workspace/use-repositories", () => ({
  useRepositories: () => ({
    repositories: savedRepositories,
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
vi.mock("@/components/repository-discovery-controls", () => ({
  RepositoryDiscoveryControls: ({ enabled }: { enabled?: boolean }) =>
    enabled !== false ? <div data-testid="repository-discovery-controls" /> : null,
}));
vi.mock("@/lib/api/domains/kanban-api", () => ({
  attachTaskWorkspaceSources,
  previewTaskWorkspaceSources,
}));
vi.mock("@/app/actions/workspaces", () => ({
  discoverRepositoriesAction,
  getRepositoryDiscoveryAction,
  refreshRepositoryDiscoveryAction,
}));

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

async function selectRepositoryMenuItem(label: string) {
  fireEvent.click(await screen.findByRole("menuitem", { name: label }));
  await waitFor(() => expect(screen.queryByRole("menuitem", { name: label })).toBeNull());
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
  repositoryDiscoveryCoordinator.dispose();
  isMobile = false;
  attachTaskWorkspaceSources.mockReset();
  previewTaskWorkspaceSources.mockReset().mockResolvedValue(null);
  discoverRepositoriesAction.mockClear();
  refreshRepositories.mockClear();
  savedRepositories.length = 0;
});

describe("AddWorkspaceSourcesDialog consequences", () => {
  it.each(SURFACE_CASES)(
    "does not warn before selecting expansion on %s",
    async (_, mobile, surfaceTestId) => {
      isMobile = mobile;
      render(<Harness />);
      fireEvent.click(screen.getByRole("button", { name: ADD_SOURCES_LABEL }));
      await screen.findByTestId(surfaceTestId);
      expect(screen.queryByTestId("workspace-change-consequences")).toBeNull();
      expect(screen.getByTestId("workspace-source-continuity")).toBeTruthy();
    },
  );
  it.each(["ssh", "k8s", "local"])(
    "does not warn for unchanged %s workspaces",
    async (executorType) => {
      render(<Harness executorType={executorType} />);
      fireEvent.click(screen.getByRole("button", { name: ADD_SOURCES_LABEL }));
      await screen.findByTestId("workspace-source-continuity");
      expect(screen.queryByTestId("workspace-change-consequences")).toBeNull();
    },
  );
});

describe("AddWorkspaceSourcesDialog repository discovery", () => {
  it("mounts discovery controls only inside a saved repository selector", async () => {
    render(
      <TooltipProvider>
        <Harness />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: ADD_SOURCES_LABEL }));
    expect(screen.queryByTestId("repository-discovery-controls")).toBeNull();
    openRepositoryMenu();
    await selectRepositoryMenuItem(WORKSPACE_REPOSITORY_LABEL);
    fireEvent.click(screen.getByTestId(REPO_CHIP_TRIGGER_TEST_ID));

    expect(screen.getByTestId("repository-discovery-controls")).toBeTruthy();
  });
});

describe("AddWorkspaceSourcesDialog touch activation", () => {
  it("opens the repository menu after touch release without selecting a source", async () => {
    isMobile = true;
    render(<Harness />);
    fireEvent.click(screen.getByRole("button", { name: ADD_SOURCES_LABEL }));
    const trigger = screen.getByRole("button", { name: "Add repository" });
    const workspaceRepository = { name: "Workspace repository" };
    fireEvent.pointerDown(trigger, { button: 0, pointerType: "touch" });
    expect(screen.queryByRole("menuitem", workspaceRepository)).toBeNull();
    fireEvent.pointerUp(trigger, { button: 0, pointerType: "touch" });
    expect(screen.queryByRole("menuitem", workspaceRepository)).toBeNull();
    fireEvent.click(trigger);
    expect(await screen.findByRole("menuitem", workspaceRepository)).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Remove source" })).toBeNull();
    fireEvent.click(await screen.findByRole("menuitem", { name: "Local Git repository" }));
    expect(screen.getByRole("button", { name: "Remove source" })).toBeTruthy();
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
    const submit = screen.getByTestId(ADD_WORKSPACE_SOURCES_SUBMIT_TEST_ID);
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

  it.each(SURFACE_CASES)(
    "returns focus to the external %s opener after Cancel",
    async (_, mobile, surfaceTestId) => {
      isMobile = mobile;
      render(<Harness />);

      const opener = screen.getByRole("button", { name: ADD_SOURCES_LABEL });
      fireEvent.click(opener);
      const surface = await screen.findByTestId(surfaceTestId);
      fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

      await finishClose(surface, mobile);
      expect(attachTaskWorkspaceSources).not.toHaveBeenCalled();
      await waitFor(() => expect(document.activeElement).toBe(opener));
    },
  );

  it.each(SURFACE_CASES)(
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
      fireEvent.click(screen.getByTestId(ADD_WORKSPACE_SOURCES_SUBMIT_TEST_ID));

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
    await selectRepositoryMenuItem(WORKSPACE_REPOSITORY_LABEL);
    fireEvent.click(screen.getByTestId(REPO_CHIP_TRIGGER_TEST_ID));

    expect(await screen.findByText("discovered-project")).toBeTruthy();
    expect(await screen.findByText("on disk")).toBeTruthy();
    expect(await screen.findByTestId("create-local-repository-button")).toBeTruthy();
    fireEvent.click(screen.getByTestId("repo-refresh-button"));
    await waitFor(() => expect(refreshRepositories).toHaveBeenCalledOnce());
  });

  it.each(SURFACE_CASES)(
    "shows the required placement choice on %s",
    async (_, mobile, surfaceTestId) => {
      isMobile = mobile;
      savedRepositories.push({
        id: "repo-1" as Repository["id"],
        workspace_id: "workspace-1" as Repository["workspace_id"],
        name: "payments",
        source_type: "local",
        local_path: "/projects/payments",
        provider: "",
        provider_repo_id: "",
        provider_owner: "",
        provider_name: "",
        default_branch: "main",
        worktree_branch_prefix: "task",
        pull_before_worktree: false,
        setup_script: "",
        cleanup_script: "",
        dev_script: "",
        copy_files: "",
        created_at: TEST_TIMESTAMP,
        updated_at: TEST_TIMESTAMP,
      });
      render(
        <TooltipProvider>
          <Harness />
        </TooltipProvider>,
      );

      fireEvent.click(screen.getByRole("button", { name: ADD_SOURCES_LABEL }));
      await screen.findByTestId(surfaceTestId);
      await openRepositoryMenu();
      await selectRepositoryMenuItem(WORKSPACE_REPOSITORY_LABEL);
      fireEvent.click(screen.getByTestId(REPO_CHIP_TRIGGER_TEST_ID));
      fireEvent.click(await screen.findByRole("option", { name: /payments/ }));

      expect(screen.getByTestId("workspace-source-placement")).toBeTruthy();
      expect(screen.getByRole("alert").textContent).toContain("Choose where to add the sources.");
      expect(
        (screen.getByTestId(ADD_WORKSPACE_SOURCES_SUBMIT_TEST_ID) as HTMLButtonElement).disabled,
      ).toBe(true);
    },
  );
});

describe("AddWorkspaceSourcesDialog placement validation", () => {
  it("keeps submit disabled when the selected placement becomes unsupported", async () => {
    savedRepositories.push({
      id: "repo-1" as Repository["id"],
      workspace_id: "workspace-1" as Repository["workspace_id"],
      name: "payments",
      source_type: "local",
      local_path: "/projects/payments",
      provider: "",
      provider_repo_id: "",
      provider_owner: "",
      provider_name: "",
      default_branch: "main",
      worktree_branch_prefix: "task",
      pull_before_worktree: false,
      setup_script: "",
      cleanup_script: "",
      dev_script: "",
      copy_files: "",
      created_at: TEST_TIMESTAMP,
      updated_at: TEST_TIMESTAMP,
    });
    previewTaskWorkspaceSources.mockImplementation((_taskId, payload) =>
      Promise.resolve({
        task_id: "task-1",
        revision: "revision-1",
        workspace_path: "/workspace/task-1",
        placement: payload.repository_placement ?? "",
        sources: [
          {
            repository_id: "repo-1",
            repository_name: "payments",
            workspace_relative_path: "payments",
          },
        ],
        supported_placements: [
          {
            placement: "kandev_directory",
            enabled: payload.repository_placement !== "kandev_directory",
            reason: "destination is occupied",
          },
          { placement: "current_root", enabled: true },
          { placement: "expand_root", enabled: false, reason: "recovery is required" },
        ],
      }),
    );

    render(
      <TooltipProvider>
        <Harness />
      </TooltipProvider>,
    );
    fireEvent.click(screen.getByRole("button", { name: ADD_SOURCES_LABEL }));
    await openRepositoryMenu();
    await selectRepositoryMenuItem(WORKSPACE_REPOSITORY_LABEL);
    fireEvent.click(screen.getByTestId(REPO_CHIP_TRIGGER_TEST_ID));
    fireEvent.click(await screen.findByRole("option", { name: /payments/ }));
    await waitFor(() => expect(previewTaskWorkspaceSources).toHaveBeenCalledOnce());

    fireEvent.click(screen.getAllByRole("radio")[0]);
    await waitFor(() => expect(previewTaskWorkspaceSources).toHaveBeenCalledTimes(2));
    await waitFor(() => {
      expect(
        (screen.getByTestId(ADD_WORKSPACE_SOURCES_SUBMIT_TEST_ID) as HTMLButtonElement).disabled,
      ).toBe(true);
    });
  });
});
