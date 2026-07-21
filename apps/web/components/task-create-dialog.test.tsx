import { createRef, type ReactNode, useEffect, useImperativeHandle, useRef } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { TaskCreateDialog } from "./task-create-dialog";
import type { DialogFormState, TaskFormInputsHandle } from "./task-create-dialog-types";

const enhancePromptMock = vi.fn();
const toastMock = vi.fn();
const setHasDescriptionMock = vi.fn();
const ORIGINAL_PROMPT = "Original prompt";
const IMPROVED_PROMPT = "Improved prompt";
const USER_EDIT = "User edit";
const PROMPT_RESULT_RECOVERY_TEST_ID = "prompt-result-recovery";
const ENHANCE_PROMPT_BUTTON_TEST_ID = "enhance-prompt-button";

let allowProgrammaticSet = true;
let mockFs: DialogFormState;

vi.mock("@kandev/ui/dialog", () => ({
  Dialog: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogContent: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogHeader: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogFooter: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock("@kandev/ui/button", () => ({
  Button: ({ children, ...rest }: { children: ReactNode } & Record<string, unknown>) => (
    <button {...rest}>{children}</button>
  ),
}));

vi.mock("@/components/routing/app-link", () => ({
  default: ({ children, href }: { children: ReactNode; href: string }) => (
    <a href={href}>{children}</a>
  ),
}));

vi.mock("@/components/workflow-selector-row", () => ({
  WorkflowSelectorRow: () => null,
}));

vi.mock("@/components/agent-logo", () => ({
  AgentLogo: () => null,
}));

vi.mock("@/hooks/use-is-utility-configured", () => ({
  useIsUtilityConfigured: () => true,
}));

vi.mock("@/hooks/use-utility-agent-generator", () => ({
  useUtilityAgentGenerator: () => ({
    enhancePrompt: enhancePromptMock,
    isEnhancingPrompt: false,
  }),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: toastMock }),
}));

vi.mock("@/hooks/use-keyboard-shortcut", () => ({
  useKeyboardShortcutHandler: () => () => undefined,
}));

vi.mock("@/components/task-create-dialog-footer", () => ({
  TaskCreateDialogFooter: () => null,
}));

vi.mock("@/components/discard-local-changes-dialog", () => ({
  DiscardLocalChangesDialog: () => null,
}));

vi.mock("@/components/task-create-dialog-header", () => ({
  DialogHeaderContent: () => null,
}));

vi.mock("@/components/task-create-dialog-create-mode-selectors", () => ({
  CreateModeSelectors: () => null,
}));

vi.mock("@/components/task-create-dialog-repo-chips", () => ({
  RepoChipsRow: () => null,
}));

vi.mock("@/hooks/use-task-create-dialog-popover-container", () => ({
  TaskCreateDialogPopoverContainerProvider: ({ children }: { children: ReactNode }) => (
    <>{children}</>
  ),
}));

vi.mock("@/components/task-create-dialog-handlers", () => ({
  resetTaskCreateLastUsedSync: () => undefined,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: { userSettings: { taskCreateLastUsed: null } }) => unknown) =>
    selector({ userSettings: { taskCreateLastUsed: null } }),
}));

vi.mock("@/components/task-create-dialog-submit", () => ({
  useTaskSubmitHandlers: () => ({
    handleSubmit: () => undefined,
    handleCancel: () => undefined,
    handleUpdateWithoutAgent: () => undefined,
    handleCreateWithoutAgent: () => undefined,
    handleCreateWithPlanMode: () => undefined,
    pendingDiscard: null,
  }),
}));

vi.mock("@/components/task-create-dialog-selectors", () => ({
  InlineTaskName: () => null,
  AgentSelector: () => null,
  ExecutorProfileSelector: () => null,
  TaskFormInputs: ({
    initialDescription,
    descriptionValueRef,
    onDescriptionChange,
    onEnhancePrompt,
  }: {
    initialDescription: string;
    descriptionValueRef: React.RefObject<TaskFormInputsHandle | null>;
    onDescriptionChange: (hasDescription: boolean) => void;
    onEnhancePrompt?: () => void;
  }) => {
    const textareaRef = useRef<HTMLTextAreaElement>(null);
    const latestValueRef = useRef(initialDescription);

    useEffect(() => {
      latestValueRef.current = initialDescription;
      if (textareaRef.current) {
        textareaRef.current.value = initialDescription;
      }
    }, [initialDescription]);

    useImperativeHandle(
      descriptionValueRef,
      () => ({
        getValue: () => latestValueRef.current,
        setValue: (next: string) => {
          if (allowProgrammaticSet) {
            latestValueRef.current = next;
            if (textareaRef.current) {
              textareaRef.current.value = next;
            }
          }
        },
        getAttachments: () => [],
      }),
      [],
    );

    return (
      <div>
        <textarea
          ref={textareaRef}
          data-testid="task-description-input"
          defaultValue={initialDescription}
          onChange={(event) => {
            const next = event.target.value;
            latestValueRef.current = next;
            onDescriptionChange(next.trim().length > 0);
          }}
        />
        <button type="button" data-testid="enhance-prompt-button" onClick={onEnhancePrompt}>
          Enhance
        </button>
      </div>
    );
  },
}));

vi.mock("@/components/task-create-dialog-state", () => ({
  useDialogFormState: () => mockFs,
  useTaskCreateDialogEffects: () => undefined,
  useDialogHandlers: () => ({
    handleTaskNameChange: () => undefined,
    handleRowRepositoryChange: () => undefined,
    handleRowBranchChange: () => undefined,
    handleAgentProfileChange: () => undefined,
    handleExecutorProfileChange: () => undefined,
    handleWorkflowChange: () => undefined,
    handleToggleRemote: () => undefined,
    handleToggleFreshBranch: () => undefined,
    handleToggleNoRepository: () => undefined,
    handleWorkspacePathChange: () => undefined,
  }),
  useLockedFieldSync: () => undefined,
  useSessionRepoName: () => "",
  useTaskCreateDialogData: () => ({
    workflows: [],
    agentProfiles: [],
    executors: [],
    snapshots: {},
    repositories: [],
    repositoriesLoading: false,
    taskCreateLastUsed: {
      branch: null,
      repositoryId: null,
      agentProfileId: null,
      executorProfileId: null,
    },
    userSettingsLoaded: true,
    computed: {
      isPassthroughProfile: false,
      effectiveWorkflowId: null,
      effectiveDefaultStepId: null,
      workspaceDefaults: null,
      hasRepositorySelection: true,
      branchOptions: [],
      agentProfileOptions: [],
      executorProfileOptions: [],
      executorHint: null,
      isLocalExecutor: false,
      headerRepositoryOptions: [],
      agentProfilesLoading: false,
      executorsLoading: false,
      workflowAgentLocked: false,
      workflowAgentProfileId: "",
      effectiveAgentProfileId: "agent-1",
      selectedExecutorProfileName: null,
      noCompatibleAgent: false,
      compatibleAgentProfiles: [],
      authLoaded: true,
    },
  }),
  computeIsTaskStarted: () => false,
}));

function buildMockFs(initialDescription = ORIGINAL_PROMPT): DialogFormState {
  return {
    taskName: "Task title",
    setTaskName: () => undefined,
    hasTitle: true,
    setHasTitle: () => undefined,
    hasDescription: true,
    setHasDescription: setHasDescriptionMock,
    draftDescription: initialDescription,
    openCycle: 0,
    currentDefaults: { name: "Task title", description: initialDescription },
    descriptionInputRef: createRef<TaskFormInputsHandle>(),
    repositories: [],
    setRepositories: () => undefined,
    addRepository: () => undefined,
    removeRepository: () => undefined,
    updateRepository: () => undefined,
    agentProfileId: "agent-1",
    setAgentProfileId: () => undefined,
    executorId: "executor-1",
    setExecutorId: () => undefined,
    executorProfileId: "executor-profile-1",
    setExecutorProfileId: () => undefined,
    discoveredRepositories: [],
    setDiscoveredRepositories: () => undefined,
    discoverReposLoading: false,
    setDiscoverReposLoading: () => undefined,
    discoverReposLoaded: true,
    setDiscoverReposLoaded: () => undefined,
    selectedWorkflowId: null,
    setSelectedWorkflowId: () => undefined,
    fetchedSteps: null,
    setFetchedSteps: () => undefined,
    isCreatingSession: false,
    setIsCreatingSession: () => undefined,
    isCreatingTask: false,
    setIsCreatingTask: () => undefined,
    useRemote: false,
    setUseRemote: () => undefined,
    remoteRepos: [],
    setRemoteRepos: () => undefined,
    addRemoteRepo: () => undefined,
    removeRemoteRepo: () => undefined,
    updateRemoteRepo: () => undefined,
    branchesByUrl: {
      branches: () => [],
      loading: () => false,
      ensure: () => undefined,
      clear: () => undefined,
    },
    prInfoByUrl: {
      info: () => undefined,
      loading: () => false,
      ensure: () => undefined,
      clear: () => undefined,
    },
    githubUrlError: null,
    setGitHubUrlError: () => undefined,
    workflowAgentProfileId: "",
    setWorkflowAgentProfileId: () => undefined,
    clearDraft: () => undefined,
    freshBranchEnabled: false,
    setFreshBranchEnabled: () => undefined,
    currentLocalBranch: "",
    setCurrentLocalBranch: () => undefined,
    currentLocalBranchLoading: false,
    setCurrentLocalBranchLoading: () => undefined,
    noRepository: false,
    setNoRepository: () => undefined,
    workspacePath: "",
    setWorkspacePath: () => undefined,
  };
}

function renderDialog() {
  return render(
    <TaskCreateDialog
      open
      onOpenChange={() => undefined}
      workspaceId="workspace-1"
      workflowId={null}
      defaultStepId={null}
      steps={[]}
    />,
  );
}

afterEach(() => {
  cleanup();
});

beforeEach(() => {
  allowProgrammaticSet = true;
  enhancePromptMock.mockReset();
  toastMock.mockReset();
  setHasDescriptionMock.mockReset();
  mockFs = buildMockFs();
});

describe("TaskCreateDialog prompt enhancement", () => {
  it("applies the enhanced prompt immediately when the description is unchanged", async () => {
    let deliver: ((result: { content: string }) => boolean | Promise<boolean>) | undefined;
    enhancePromptMock.mockImplementation(
      (_source: string, onSuccess: (result: { content: string }) => boolean | Promise<boolean>) => {
        deliver = onSuccess;
      },
    );

    renderDialog();

    const textarea = screen.getByTestId("task-description-input") as HTMLTextAreaElement;
    fireEvent.click(screen.getByTestId(ENHANCE_PROMPT_BUTTON_TEST_ID));

    expect(enhancePromptMock).toHaveBeenCalledWith(ORIGINAL_PROMPT, expect.any(Function));

    await act(async () => {
      await deliver?.({ content: IMPROVED_PROMPT });
    });

    await waitFor(() => expect(textarea.value).toBe(IMPROVED_PROMPT));
    expect(setHasDescriptionMock).toHaveBeenCalledWith(true);
    expect(screen.queryByTestId(PROMPT_RESULT_RECOVERY_TEST_ID)).toBeNull();
  });

  it("keeps the user's edited description and offers recovery", async () => {
    let deliver: ((result: { content: string }) => boolean | Promise<boolean>) | undefined;
    enhancePromptMock.mockImplementation(
      (_source: string, onSuccess: (result: { content: string }) => boolean | Promise<boolean>) => {
        deliver = onSuccess;
      },
    );

    renderDialog();

    const textarea = screen.getByTestId("task-description-input") as HTMLTextAreaElement;
    fireEvent.click(screen.getByTestId(ENHANCE_PROMPT_BUTTON_TEST_ID));
    fireEvent.change(textarea, { target: { value: USER_EDIT } });

    await act(async () => {
      await deliver?.({ content: IMPROVED_PROMPT });
    });

    await waitFor(() => expect(textarea.value).toBe(USER_EDIT));
    expect(screen.getByTestId(PROMPT_RESULT_RECOVERY_TEST_ID)).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Apply" }));

    await waitFor(() => expect(textarea.value).toBe(IMPROVED_PROMPT));
    expect(screen.queryByTestId(PROMPT_RESULT_RECOVERY_TEST_ID)).toBeNull();
  });

  it("keeps the generated result behind recovery when the editor handle is gone", async () => {
    let deliver: ((result: { content: string }) => boolean | Promise<boolean>) | undefined;
    enhancePromptMock.mockImplementation(
      (_source: string, onSuccess: (result: { content: string }) => boolean | Promise<boolean>) => {
        deliver = onSuccess;
      },
    );

    renderDialog();

    fireEvent.click(screen.getByTestId(ENHANCE_PROMPT_BUTTON_TEST_ID));
    mockFs.descriptionInputRef.current = null;

    await act(async () => {
      await deliver?.({ content: IMPROVED_PROMPT });
    });

    expect(screen.getByTestId(PROMPT_RESULT_RECOVERY_TEST_ID)).toBeTruthy();
  });

  it("keeps the generated result behind recovery when the editor rejects a write", async () => {
    let deliver: ((result: { content: string }) => boolean | Promise<boolean>) | undefined;
    enhancePromptMock.mockImplementation(
      (_source: string, onSuccess: (result: { content: string }) => boolean | Promise<boolean>) => {
        deliver = onSuccess;
      },
    );

    renderDialog();

    fireEvent.click(screen.getByTestId(ENHANCE_PROMPT_BUTTON_TEST_ID));
    allowProgrammaticSet = false;

    await act(async () => {
      await deliver?.({ content: IMPROVED_PROMPT });
    });

    expect(setHasDescriptionMock).not.toHaveBeenCalled();
    expect(screen.getByTestId(PROMPT_RESULT_RECOVERY_TEST_ID)).toBeTruthy();
  });
});
