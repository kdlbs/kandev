import { useLayoutEffect } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { HydrationState } from "@/lib/state/store";
import type { Repository, TaskSession } from "@/lib/types/http";
import type { TaskOverview } from "@/lib/state/slices/task-overview-types";
import { agentProfileId, sessionId } from "@/lib/types/ids";
import { createTask } from "@/lib/api/domains/kanban-api";
import { NewSubtaskDialog } from "./new-subtask-dialog";

vi.mock("@/lib/api/domains/kanban-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/kanban-api")>();
  return { ...actual, createTask: vi.fn().mockResolvedValue({ id: "created-child" }) };
});
vi.mock("@/hooks/domains/settings/use-settings-data", () => ({ useSettingsData: () => {} }));
vi.mock("@/hooks/use-is-utility-configured", () => ({ useIsUtilityConfigured: () => false }));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: vi.fn() }) }));
vi.mock("@/hooks/use-summarize-session", () => ({
  useSummarizeSession: () => ({ summarize: vi.fn(), isSummarizing: false }),
}));
vi.mock("@/hooks/use-utility-agent-generator", () => ({
  useUtilityAgentGenerator: () => ({ enhancePrompt: vi.fn(), isEnhancingPrompt: false }),
}));
vi.mock("@/components/task-create-dialog-effects", () => ({
  useDiscoverReposEffect: () => {},
  useGitHubUrlErrorEffect: () => {},
}));
vi.mock("@/hooks/domains/github/use-branches-by-url", () => ({
  useBranchesByURL: () => ({
    branches: () => [],
    loading: () => false,
    error: () => null,
    ensure: vi.fn(),
    clear: vi.fn(),
  }),
}));
vi.mock("@/hooks/domains/github/use-pr-info-by-url", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/hooks/domains/github/use-pr-info-by-url")>();
  return {
    ...actual,
    usePRInfoByURL: () => ({
      info: () => null,
      error: () => null,
      ensure: vi.fn(),
      clear: vi.fn(),
    }),
  };
});
vi.mock("@/hooks/domains/integrations/use-remote-repositories", () => ({
  useRemoteRepositories: () => ({
    repos: [],
    availableProviders: [],
    loading: false,
    unavailable: false,
    error: null,
    search: vi.fn(),
    matchesURL: () => false,
  }),
}));
vi.mock("@/hooks/domains/workspace/use-repository-branches", () => ({
  useBranches: () => ({ branches: [], isLoading: false }),
}));
vi.mock("@/hooks/domains/workspace/use-repository-branch-policies", () => ({
  useRepositoryBranchPolicies: () => ({ policies: [] }),
}));
vi.mock("@/hooks/domains/workspace/use-repository-sets", () => ({
  useRepositorySets: () => ({ sets: [], isLoading: false, refresh: vi.fn() }),
}));
vi.mock("@/hooks/use-task-sessions", async () => {
  const { useAppStore } = await import("@/components/state-provider");
  return {
    useTaskSessions: (taskId: string) => {
      const sessions = useAppStore((s) => s.taskSessionsByTask.itemsByTaskId[taskId]);
      const isLoaded = useAppStore((s) => s.taskSessionsByTask.loadedByTaskId[taskId]);
      const error = useAppStore((s) => s.taskSessionsByTask.errorByTaskId?.[taskId]);
      return {
        sessions: sessions ?? EMPTY_SESSIONS,
        isLoaded,
        error,
        isLoading: !isLoaded && !error,
        loadSessions: vi.fn(),
      };
    },
  };
});
vi.mock("@/components/task-create-dialog-options", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/components/task-create-dialog-options")>();
  return {
    ...actual,
    useAgentProfileOptions: () =>
      [PARENT_PROFILE_LABEL, "Active profile", CHOSEN_PROFILE_LABEL, "Secondary profile"].map(
        (label, index) => ({
          value: ["parent-profile", "active-profile", "chosen-profile", SECONDARY_PROFILE_ID][
            index
          ],
          label,
          renderLabel: () => label,
        }),
      ),
    useExecutorProfileOptions: () => [],
  };
});

const PARENT_PROFILE_LABEL = "Parent profile";
const CHOSEN_PROFILE_LABEL = "Chosen profile";
const SECONDARY_PROFILE_ID = agentProfileId("secondary-profile");
const PARENT_REPOSITORY_LABEL = "Parent repository";
const AGENT_SELECTOR_ID = "agent-profile-selector";
const REPO_CHIP_ID = "repo-chip-trigger";
const NEW_MODE_ID = "subtask-workspace-mode-new";
const CHECKED_ATTRIBUTE = "aria-checked";
const CREATE_SUBTASK_LABEL = "Create Subtask";
const PARENT_BRANCH = "parent-branch";
const EMPTY_SESSIONS: TaskSession[] = [];
const repositories = [PARENT_REPOSITORY_LABEL, "Chosen repository", "Active repository"].map(
  (name, index) => ({
    id: ["parent-repo", "chosen-repo", "active-repo"][index],
    name,
    workspace_id: "workspace",
    source_type: "local",
    local_path: `/repos/${index}`,
    default_branch: "main",
  }),
) as Repository[];
const parent = {
  id: "parent",
  title: "Selected parent",
  primarySessionId: "primary",
  workflowId: "workflow",
  repositories: [{ repository_id: "parent-repo", base_branch: "release", position: 0 }],
} as TaskOverview;
const primary = {
  id: "primary",
  task_id: "parent",
  is_primary: true,
  agent_profile_id: "parent-profile",
  repository_id: "parent-repo",
  base_branch: "main",
  started_at: "2026-10-14T12:00:00Z",
} as TaskSession;
const active = {
  ...parent,
  id: "active",
  title: "Active task",
  primarySessionId: "active-session",
  repositories: [{ repository_id: "active-repo", position: 0 }],
} as TaskOverview;
let store: ReturnType<typeof useAppStoreApi>;
function CaptureStore({ selectedParent }: { selectedParent: TaskOverview | null }) {
  store = useAppStoreApi();
  useLayoutEffect(() => {
    if (selectedParent) store.getState().retainTaskOverviews("sidebar:test", [selectedParent]);
  }, [selectedParent]);
  return null;
}
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function openDialog(
  options: {
    boardParent?: boolean;
    missing?: boolean;
    loaded?: boolean;
    sessions?: TaskSession[];
    noRepository?: boolean;
  } = {},
) {
  const initial = {
    userSettings: { agentGeneratedTaskTitles: false },
    workspaces: { activeId: "workspace" },
    kanban: { tasks: options.boardParent ? [parent] : [active], workflowId: "workflow" },
    tasks: {
      activeTaskId: options.boardParent ? "parent" : "active",
      activeSessionId: options.boardParent ? "primary" : "active-session",
    },
    repositories: {
      itemsByWorkspaceId: { workspace: repositories },
      loadedByWorkspaceId: { workspace: true },
    },
    taskSessionsByTask: {
      itemsByTaskId: { parent: options.sessions ?? [] },
      loadedByTaskId: { parent: options.loaded ?? false },
    },
    taskSessions: {
      items: {
        "active-session": {
          ...primary,
          id: "active-session",
          task_id: "active",
          agent_profile_id: "active-profile",
        },
      },
    },
  } as unknown as HydrationState;
  const selectedParent = options.noRepository ? { ...parent, repositories: [] } : parent;
  render(
    <StateProvider initialState={initial}>
      <TooltipProvider>
        <CaptureStore selectedParent={options.missing ? null : selectedParent} />
        <NewSubtaskDialog
          open
          onOpenChange={vi.fn()}
          parentTaskId="parent"
          parentTaskTitle="Selected parent"
        />
      </TooltipProvider>
    </StateProvider>,
  );
  if (!options.missing) expect(store.getState().taskOverview.byId.parent?.id).toBe("parent");
}
function deliver(sessions: TaskSession[], error: string | null = null) {
  act(() =>
    store.setState((s) => ({
      taskSessionsByTask: {
        ...s.taskSessionsByTask,
        itemsByTaskId: { ...s.taskSessionsByTask.itemsByTaskId, parent: sessions },
        loadedByTaskId: { ...s.taskSessionsByTask.loadedByTaskId, parent: !error },
        errorByTaskId: { ...s.taskSessionsByTask.errorByTaskId, parent: error },
      },
    })),
  );
}
function chooseProfile(name: string) {
  fireEvent.click(screen.getByTestId(AGENT_SELECTOR_ID));
  fireEvent.click(screen.getByRole("option", { name }));
}
function enterWork() {
  fireEvent.change(screen.getByTestId("subtask-title-input"), {
    target: { value: "Entered title" },
  });
  fireEvent.change(screen.getByTestId("subtask-prompt-input"), {
    target: { value: "Entered prompt" },
  });
}
function expectWork() {
  expect((screen.getByTestId("subtask-title-input") as HTMLInputElement).value).toBe(
    "Entered title",
  );
  expect((screen.getByTestId("subtask-prompt-input") as HTMLTextAreaElement).value).toBe(
    "Entered prompt",
  );
}

// @covers AC-TASKS-SUBTASK-DIALOG-PARENT-DEFAULTS-001.1
// @covers AC-TASKS-SUBTASK-DIALOG-PARENT-DEFAULTS-001.2
// @covers AC-TASKS-SUBTASK-DIALOG-PARENT-DEFAULTS-001.5
describe("selected parent initial defaults", () => {
  it("seeds canonical-only non-active parent and delayed primary without remounting entered work", () => {
    openDialog();
    enterWork();
    deliver([primary]);
    expect(screen.getByTestId(REPO_CHIP_ID).textContent).toContain(PARENT_REPOSITORY_LABEL);
    expect(screen.getByTestId(AGENT_SELECTOR_ID).textContent).toContain(PARENT_PROFILE_LABEL);
    expect(screen.getByTestId(NEW_MODE_ID).getAttribute(CHECKED_ATTRIBUTE)).toBe("true");
    expectWork();
  });
  it("seeds the first available parent repository from delayed primary context", () => {
    openDialog({ noRepository: true });
    deliver([primary]);
    expect(screen.getByTestId(REPO_CHIP_ID).textContent).toContain(PARENT_REPOSITORY_LABEL);
  });
  it("does not reseed initial defaults after a later primary refresh", () => {
    openDialog();
    deliver([primary]);
    deliver([
      { ...primary, agent_profile_id: SECONDARY_PROFILE_ID, worktree_branch: "later-branch" },
    ]);
    expect(screen.getByTestId(AGENT_SELECTOR_ID).textContent).toContain(PARENT_PROFILE_LABEL);
    expect(screen.getByTestId(NEW_MODE_ID).getAttribute(CHECKED_ATTRIBUTE)).toBe("true");
    expect(screen.getByTestId(REPO_CHIP_ID).textContent).toContain(PARENT_REPOSITORY_LABEL);
  });
  it("retains already-loaded active-parent defaults", async () => {
    openDialog({ boardParent: true, loaded: true, sessions: [primary] });
    expect(screen.getByTestId(REPO_CHIP_ID).textContent).toContain(PARENT_REPOSITORY_LABEL);
    expect(screen.getByTestId(AGENT_SELECTOR_ID).textContent).toContain(PARENT_PROFILE_LABEL);
    enterWork();
    fireEvent.click(screen.getByRole("button", { name: CREATE_SUBTASK_LABEL }));
    await waitFor(() =>
      expect(createTask).toHaveBeenCalledWith(
        expect.objectContaining({ agent_profile_id: "parent-profile" }),
      ),
    );
  });
  it("uses delayed primary worktree rather than differently configured secondary", () => {
    openDialog({ boardParent: true });
    enterWork();
    deliver([
      {
        ...primary,
        id: sessionId("secondary"),
        is_primary: false,
        agent_profile_id: SECONDARY_PROFILE_ID,
        worktree_branch: "secondary-branch",
      },
      { ...primary, worktree_branch: PARENT_BRANCH },
    ]);
    expect(screen.getByTestId(AGENT_SELECTOR_ID).textContent).toContain(PARENT_PROFILE_LABEL);
    expect(
      screen.getByTestId("subtask-workspace-mode-inherit").getAttribute(CHECKED_ATTRIBUTE),
    ).toBe("true");
    expect(screen.getByText(PARENT_BRANCH)).toBeTruthy();
    expectWork();
  });
});

// @covers AC-TASKS-SUBTASK-DIALOG-PARENT-DEFAULTS-001.3
// @covers AC-TASKS-SUBTASK-DIALOG-PARENT-DEFAULTS-001.4
describe("untouched defaults and usable missing context", () => {
  it("keeps repository removal and explicit profile through late branch context without a mode click", async () => {
    openDialog({ boardParent: true });
    enterWork();
    fireEvent.click(screen.getByRole("button", { name: "Remove repository" }));
    chooseProfile(CHOSEN_PROFILE_LABEL);
    deliver([{ ...primary, worktree_branch: PARENT_BRANCH }]);
    deliver([
      { ...primary, agent_profile_id: SECONDARY_PROFILE_ID, worktree_branch: "changed-branch" },
    ]);
    expect(screen.queryByTestId(REPO_CHIP_ID)).toBeNull();
    expect(screen.getByTestId(AGENT_SELECTOR_ID).textContent).toContain(CHOSEN_PROFILE_LABEL);
    expect(screen.getByTestId(NEW_MODE_ID).getAttribute(CHECKED_ATTRIBUTE)).toBe("true");
    expectWork();
    fireEvent.click(screen.getByRole("button", { name: CREATE_SUBTASK_LABEL }));
    await waitFor(() => expect(createTask).toHaveBeenCalledTimes(1));
    expect(vi.mocked(createTask).mock.calls[0][0]).toMatchObject({
      workspace_mode: "new_workspace",
      repositories: [],
    });
  });
  it("preserves an explicit repository selection before late parent repository context", async () => {
    openDialog({ noRepository: true });
    fireEvent.click(screen.getByTestId("add-repository"));
    fireEvent.click(screen.getByTestId(REPO_CHIP_ID));
    fireEvent.click(screen.getByRole("option", { name: /^Chosen repository/ }));
    enterWork();
    deliver([{ ...primary, worktree_branch: PARENT_BRANCH }]);
    fireEvent.click(screen.getByRole("button", { name: CREATE_SUBTASK_LABEL }));
    await waitFor(() => expect(createTask).toHaveBeenCalledTimes(1));
    expect(vi.mocked(createTask).mock.calls[0][0]).toMatchObject({
      workspace_mode: "new_workspace",
      repositories: [expect.objectContaining({ repository_id: "chosen-repo" })],
    });
    expect(screen.getByTestId(NEW_MODE_ID).getAttribute(CHECKED_ATTRIBUTE)).toBe("true");
    expect(screen.getByTestId(REPO_CHIP_ID).textContent).toContain("Chosen repository");
  });
  it("preserves an explicit new workspace choice through late branch context", () => {
    openDialog();
    fireEvent.click(screen.getByTestId(NEW_MODE_ID));
    deliver([{ ...primary, worktree_branch: PARENT_BRANCH }]);
    expect(screen.getByTestId(NEW_MODE_ID).getAttribute(CHECKED_ATTRIBUTE)).toBe("true");
  });
  it("keeps a deliberate cleared profile at the create boundary after late primary data", async () => {
    openDialog({ boardParent: true });
    enterWork();
    chooseProfile(CHOSEN_PROFILE_LABEL);
    chooseProfile(CHOSEN_PROFILE_LABEL);
    expect(screen.getByTestId(AGENT_SELECTOR_ID).textContent).toContain("Select agent profile");
    deliver([primary]);
    expect(screen.getByTestId(AGENT_SELECTOR_ID).textContent).toContain("Select agent profile");
    fireEvent.click(screen.getByRole("button", { name: CREATE_SUBTASK_LABEL }));
    await waitFor(() => expect(createTask).toHaveBeenCalledTimes(1));
    expect(vi.mocked(createTask).mock.calls[0][0]).toMatchObject({
      parent_id: "parent",
      agent_profile_id: undefined,
      title: "Entered title",
      description: "Entered prompt",
    });
  });
  it.each(["empty", "error", "missing"])(
    "keeps the form editable for %s parent context",
    (kind) => {
      openDialog({ missing: kind === "missing" });
      deliver([], kind === "error" ? "read failed" : null);
      enterWork();
      expectWork();
      expect(screen.getByTestId(NEW_MODE_ID).getAttribute(CHECKED_ATTRIBUTE)).toBe("true");
      expect((screen.getByRole("button", { name: "Cancel" }) as HTMLButtonElement).disabled).toBe(
        false,
      );
    },
  );
});

// @covers AC-TASKS-SUBTASK-DIALOG-PARENT-DEFAULTS-001.3
describe("explicit remote subtask source", () => {
  it("keeps a pasted remote source through late primary worktree data", async () => {
    openDialog();
    enterWork();
    fireEvent.click(screen.getByTestId("source-mode-remote"));
    fireEvent.click(screen.getByTestId("remote-repo-chip-trigger"));
    const input = screen.getByTestId("remote-repo-input");
    fireEvent.paste(input, { clipboardData: { getData: () => "https://github.com/acme/chosen" } });
    fireEvent.keyDown(input, { key: "Enter" });
    deliver([{ ...primary, worktree_branch: PARENT_BRANCH }]);
    fireEvent.click(screen.getByRole("button", { name: CREATE_SUBTASK_LABEL }));
    await waitFor(() => expect(createTask).toHaveBeenCalledTimes(1));
    expect(vi.mocked(createTask).mock.calls[0][0]).toMatchObject({
      workspace_mode: "new_workspace",
      repositories: [expect.objectContaining({ github_url: "https://github.com/acme/chosen" })],
    });
    expect(screen.getByTestId(NEW_MODE_ID).getAttribute(CHECKED_ATTRIBUTE)).toBe("true");
  });
});
