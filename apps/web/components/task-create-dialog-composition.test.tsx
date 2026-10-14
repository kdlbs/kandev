import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { TaskCreateDialogProps } from "./task-create-dialog";

const mocks = vi.hoisted(() => ({
  blocked: false,
  toast: vi.fn(),
  close: vi.fn(),
  creating: vi.fn(),
  clearDraft: vi.fn(),
  createTask: vi.fn(),
}));
const TITLE = "Composed title";
const PROMPT = "Entered composition draft";

vi.mock("@/hooks/use-is-utility-configured", () => ({ useIsUtilityConfigured: () => false }));
vi.mock("@/hooks/use-utility-agent-generator", () => ({
  useUtilityAgentGenerator: () => ({ enhancePrompt: vi.fn(), isEnhancingPrompt: false }),
}));
vi.mock("@/hooks/domains/workspace/use-repository-sets", () => ({
  useRepositorySets: () => ({ sets: [] }),
}));
vi.mock("@/hooks/domains/task/use-task-edit-dialog-dependencies", () => ({
  useTaskEditDialogDependencies: () => undefined,
  isTaskDependencyUpdateFailure: () => false,
}));
vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: vi.fn() }),
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: mocks.toast }) }));
vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({ getState: () => ({}) }),
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      userSettings: { agentGeneratedTaskTitles: false, autoFocusNewTasks: false },
      repositoryBranches: { itemsByRepositoryId: {}, loadedByRepositoryId: {} },
      setActiveDocument: vi.fn(),
      setPlanMode: vi.fn(),
      applyAgentProfileRecentUse: vi.fn(),
    }),
}));
vi.mock("@/components/task-create-dialog-workflow-context", () => ({
  useResolvedTaskCreateWorkflowContext: (props: TaskCreateDialogProps) => props,
}));
vi.mock("@/components/task-create-dialog-workflow-agent-override-validation", () => ({
  buildWorkflowAgentOverrideValidation: () => ({ rows: [], options: [], invalid: false }),
}));
vi.mock("@/components/task-create-dialog-state", () => ({
  computeIsTaskStarted: () => false,
  useDialogFormState: () => ({
    taskName: TITLE,
    hasTitle: true,
    hasDescription: true,
    descriptionInputRef: {
      current: {
        getValue: () => PROMPT,
        getAttachments: () => [],
        setValue: vi.fn(),
      },
    },
    openCycle: 1,
    repositories: [],
    discoveredRepositories: [],
    remoteRepos: [],
    prInfoByUrl: {},
    useRemote: false,
    noRepository: true,
    workspacePath: "",
    executorId: "executor-1",
    executorProfileId: "executor-profile-1",
    seededExecutorProfileId: null,
    freshBranchEnabled: false,
    priority: "medium",
    workflowAgentOverrides: {},
    blockedBy: [],
    setIsCreatingTask: mocks.creating,
    clearDraft: mocks.clearDraft,
    setRepositories: vi.fn(),
    setRepositoriesDirty: vi.fn(),
  }),
  useTaskCreateDialogData: () => ({
    workflows: [],
    agentProfiles: [],
    executors: [],
    snapshots: {},
    workspaceSnapshotRead: {},
    repositories: [],
    repositoriesLoading: false,
    taskCreateLastUsed: {},
    userSettingsLoaded: true,
    computed: {
      effectiveWorkflowId: "workflow-1",
      effectiveAgentProfileId: "agent-1",
      compatibleAgentProfiles: [],
      workspaceDefaults: {},
      isLocalExecutor: false,
      isPassthroughProfile: false,
      agentProfileOptions: [],
      noCompatibleAgent: mocks.blocked,
    },
  }),
  useTaskCreateDialogEffects: vi.fn(),
  useLockedFieldSync: vi.fn(),
  useDialogHandlers: () => ({}),
  useSessionRepoName: () => null,
}));

import { useTaskCreateDialogSetup } from "./task-create-dialog-setup";
import { resetTaskCreateLastUsedSync } from "./task-create-dialog-handlers";

function TaskCreateInput() {
  const setup = useTaskCreateDialogSetup({
    mode: "create",
    open: true,
    workspaceId: "workspace-1",
    workflowId: "workflow-1",
    defaultStepId: null,
    steps: [],
    onOpenChange: mocks.close,
    createTask: mocks.createTask,
  });
  return <input aria-label="Task title" defaultValue={TITLE} onKeyDown={setup.handleKeyDown} />;
}

async function dispatch(input: HTMLElement, init: KeyboardEventInit = {}) {
  const event = new KeyboardEvent("keydown", {
    key: "Enter",
    ctrlKey: true,
    bubbles: true,
    cancelable: true,
    ...init,
  });
  await act(async () => {
    input.dispatchEvent(event);
  });
  return event;
}

// @covers AC-UI-KEYBOARD-SHORTCUT-COMPOSITION-001.2, .4
describe("Task Create composition with real setup and submit wiring", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.blocked = false;
    mocks.createTask.mockResolvedValue({ id: "created-task-1" });
    resetTaskCreateLastUsedSync({ clearQueued: true });
  });
  afterEach(() => {
    cleanup();
    resetTaskCreateLastUsedSync({ clearQueued: true });
  });

  it.each([
    { isComposing: true, keyCode: 13 },
    { isComposing: false, keyCode: 229 },
  ])("does not create for %j, then creates once for an ordinary chord", async (signal) => {
    render(<TaskCreateInput />);
    const input = screen.getByRole("textbox");
    const propagated = vi.fn();
    document.addEventListener("keydown", propagated);
    try {
      const event = await dispatch(input, signal);
      expect(mocks.createTask).not.toHaveBeenCalled();
      expect(event.defaultPrevented).toBe(false);
      expect(propagated).toHaveBeenCalledTimes(1);
      expect(mocks.close).not.toHaveBeenCalled();
      expect(mocks.clearDraft).not.toHaveBeenCalled();
      expect((input as HTMLInputElement).value).toBe(TITLE);

      const ordinary = await dispatch(input);
      expect(mocks.createTask).toHaveBeenCalledExactlyOnceWith(
        expect.objectContaining({
          title: TITLE,
          description: PROMPT,
          workspace_id: "workspace-1",
          workflow_id: "workflow-1",
        }),
      );
      expect(ordinary.defaultPrevented).toBe(true);
      expect(mocks.close).toHaveBeenCalledWith(false);
      expect(mocks.toast).not.toHaveBeenCalled();
    } finally {
      document.removeEventListener("keydown", propagated);
    }
  });

  it("does not create for plain Enter or a compatibility-blocked ordinary chord", async () => {
    mocks.blocked = true;
    render(<TaskCreateInput />);
    const input = screen.getByRole("textbox");
    const plain = await dispatch(input, { ctrlKey: false });
    expect(plain.defaultPrevented).toBe(false);
    const ordinary = await dispatch(input);
    expect(ordinary.defaultPrevented).toBe(true);
    expect(mocks.createTask).not.toHaveBeenCalled();
    expect(mocks.close).not.toHaveBeenCalled();
  });

  it("creates once through the real submit boundary for an eligible ordinary chord", async () => {
    render(<TaskCreateInput />);
    await dispatch(screen.getByRole("textbox"));
    expect(mocks.createTask).toHaveBeenCalledExactlyOnceWith(
      expect.objectContaining({
        title: TITLE,
        description: PROMPT,
        start_agent: true,
      }),
    );
    expect(mocks.close).toHaveBeenCalledWith(false);
    expect(mocks.toast).not.toHaveBeenCalled();
  });
});
