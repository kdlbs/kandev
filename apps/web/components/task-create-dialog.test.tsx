/* eslint-disable max-lines -- dialog lifecycle cases share one provider and submit harness. */
import {
  createRef,
  type ComponentProps,
  type ReactNode,
  useContext,
  useEffect,
  useImperativeHandle,
  useRef,
} from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { TaskCreateDialog } from "./task-create-dialog";
import type { DialogFormState, TaskFormInputsHandle } from "./task-create-dialog-types";
import {
  TaskCreateDialogTaskCreatedContext,
  type RegisterTaskCreatedHandler,
} from "./task-create-dialog-task-created";

const enhancePromptMock = vi.fn();
const toastMock = vi.fn();
const setHasDescriptionMock = vi.fn();
const formInitializationSpy = vi.fn();
const ORIGINAL_PROMPT = "Original prompt";
const IMPROVED_PROMPT = "Improved prompt";
const USER_EDIT = "User edit";
const PROMPT_RESULT_RECOVERY_TEST_ID = "prompt-result-recovery";
const ENHANCE_PROMPT_BUTTON_TEST_ID = "enhance-prompt-button";
const DEFAULT_WORKSPACE_ID = "workspace-1";
const SECOND_WORKSPACE_ID = "workspace-second";

const taskCreatedHandlerByWorkspace = new Map<
  string,
  (task: { id: string; workspace_id: string }) => void
>();
const taskCreatedRegistrationByWorkspace = new Map<string, RegisterTaskCreatedHandler | null>();
const taskSubmitHarness = {
  succeeds: false,
  onSuccessByWorkspace: new Map<
    string,
    (task: { id: string; workspace_id: string }, mode: "create" | "edit" | "session") => void
  >(),
};
type EscapeEvent = { preventDefault: () => void };

type CloseAutoFocusEvent = { preventDefault: () => void };

let allowProgrammaticSet = true;
let mockFs: DialogFormState;
let dialogEscapeHandler: ((event: EscapeEvent) => void) | undefined;
let autoFocusNewTasks = true;
let dialogCloseAutoFocusHandler: ((event: CloseAutoFocusEvent) => void) | undefined;

vi.mock("@kandev/ui/dialog", () => ({
  Dialog: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogContent: ({
    children,
    onEscapeKeyDown,
    onCloseAutoFocus,
  }: {
    children: ReactNode;
    onEscapeKeyDown?: (event: EscapeEvent) => void;
    onCloseAutoFocus?: (event: CloseAutoFocusEvent) => void;
  }) => {
    dialogEscapeHandler = onEscapeKeyDown;
    dialogCloseAutoFocusHandler = onCloseAutoFocus;
    return <div>{children}</div>;
  },
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
  isNativeSubmitDisabled: () => false,
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

vi.mock("./task-edit-dialog-dependencies", () => ({
  TaskEditDialogDependencies: () => null,
}));

vi.mock("@/hooks/use-task-create-dialog-popover-container", () => ({
  useTaskCreateDialogPopoverContainer: () => null,
  TaskCreateDialogPopoverContainerProvider: ({ children }: { children: ReactNode }) => (
    <>{children}</>
  ),
}));

vi.mock("@/components/task-create-dialog-handlers", () => ({
  resetTaskCreateLastUsedSync: () => undefined,
}));

vi.mock("@/components/state-provider", () => ({
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  useAppStore: (selector: (state: any) => unknown) =>
    selector({
      userSettings: { taskCreateLastUsed: null, autoFocusNewTasks },
      repositorySets: {
        itemsByWorkspaceId: {},
        loadingByWorkspaceId: {},
        loadedByWorkspaceId: {},
        revisionByWorkspaceId: {},
      },
      setRepositorySets: () => undefined,
      setRepositorySetsLoading: () => undefined,
    }),
  useAppStoreApi: () => ({
    getState: () => ({
      repositoryBranchPolicies: { revisionByRepositoryId: {} },
    }),
  }),
}));

vi.mock("@/components/task-create-dialog-submit", () => ({
  useTaskSubmitHandlers: (deps: {
    isEditMode: boolean;
    isSessionMode: boolean;
    workspaceId: string;
    onSuccess: (
      task: { id: string; workspace_id: string },
      mode: "create" | "edit" | "session",
    ) => void;
  }) => {
    taskSubmitHarness.onSuccessByWorkspace.set(deps.workspaceId, deps.onSuccess);
    return {
      handleSubmit: () => {
        if (!taskSubmitHarness.succeeds) return;
        let mode: "create" | "edit" | "session" = "create";
        if (deps.isSessionMode) mode = "session";
        else if (deps.isEditMode) mode = "edit";
        deps.onSuccess({ id: `task-${deps.workspaceId}`, workspace_id: deps.workspaceId }, mode);
      },
      handleCancel: () => undefined,
      handleUpdateWithoutAgent: () => undefined,
      handleCreateWithoutAgent: () => undefined,
      handleCreateWithPlanMode: () => undefined,
      pendingDiscard: null,
    };
  },
}));

vi.mock("@/components/task-create-dialog-selectors", () => ({
  InlineTaskName: () => null,
  AgentSelector: () => null,
  ExecutorProfileSelector: () => null,
  TaskFormInputs: ({
    workspaceId,
    initialDescription,
    descriptionValueRef,
    onDescriptionChange,
    onEnhancePrompt,
  }: {
    workspaceId: string;
    initialDescription: string;
    descriptionValueRef: React.RefObject<TaskFormInputsHandle | null>;
    onDescriptionChange: (hasDescription: boolean) => void;
    onEnhancePrompt?: () => void;
  }) => {
    const registerTaskCreatedHandler = useContext(TaskCreateDialogTaskCreatedContext);
    const handler = taskCreatedHandlerByWorkspace.get(workspaceId);
    taskCreatedRegistrationByWorkspace.set(workspaceId, registerTaskCreatedHandler);
    useEffect(() => {
      if (!handler || !registerTaskCreatedHandler) return;
      return registerTaskCreatedHandler(handler);
    }, [handler, registerTaskCreatedHandler]);
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
  useDialogFormState: () => {
    formInitializationSpy();
    return mockFs;
  },
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
    blockedBy: [],
    setBlockedBy: () => undefined,
    taskName: "Task title",
    autopilot: false,
    setAutopilot: () => undefined,
    autoCreatePR: false,
    setAutoCreatePR: () => undefined,
    priority: "medium",
    setPriority: () => undefined,
    setTaskName: () => undefined,
    hasTitle: true,
    setHasTitle: () => undefined,
    hasDescription: true,
    setHasDescription: setHasDescriptionMock,
    hasPendingAttachmentUploads: false,
    setHasPendingAttachmentUploads: () => undefined,
    draftDescription: initialDescription,
    openCycle: 0,
    currentDefaults: { name: "Task title", description: initialDescription },
    descriptionInputRef: createRef<TaskFormInputsHandle>(),
    repositories: [],
    repositoriesDirty: false,
    setRepositories: () => undefined,
    setRepositoriesDirty: () => undefined,
    addRepository: () => undefined,
    removeRepository: () => undefined,
    updateRepository: () => undefined,
    agentProfileId: "agent-1",
    setAgentProfileId: () => undefined,
    executorId: "executor-1",
    setExecutorId: () => undefined,
    executorProfileId: "executor-profile-1",
    setExecutorProfileId: () => undefined,
    setExecutorProfileIdFromSeed: () => undefined,
    seededExecutorProfileId: null,
    discoveredRepositories: [],
    setDiscoveredRepositories: () => undefined,
    discoverReposLoading: false,
    setDiscoverReposLoading: () => undefined,
    discoverReposLoaded: true,
    setDiscoverReposLoaded: () => undefined,
    selectedWorkflowId: null,
    setSelectedWorkflowId: () => undefined,
    workflowAgentOverrides: {},
    setWorkflowAgentOverrides: () => undefined,
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
      error: () => undefined,
      ensure: () => undefined,
      clear: () => undefined,
    },
    prInfoByUrl: {
      info: () => undefined,
      loading: () => false,
      settled: () => true,
      error: () => undefined,
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
    preferLocalExecutor: false,
    setPreferLocalExecutor: () => undefined,
    workspacePath: "",
    setWorkspacePath: () => undefined,
  };
}

function renderDialog(
  mode: "create" | "edit" | "session" = "create",
  extraProps: Partial<ComponentProps<typeof TaskCreateDialog>> = {},
) {
  return render(
    <TaskCreateDialog
      open
      mode={mode}
      onOpenChange={() => undefined}
      workspaceId={DEFAULT_WORKSPACE_ID}
      workflowId={null}
      defaultStepId={null}
      steps={[]}
      {...extraProps}
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
  formInitializationSpy.mockReset();
  dialogEscapeHandler = undefined;
  dialogCloseAutoFocusHandler = undefined;
  autoFocusNewTasks = true;
  mockFs = buildMockFs();
  taskCreatedHandlerByWorkspace.clear();
  taskSubmitHarness.succeeds = false;
  taskSubmitHarness.onSuccessByWorkspace.clear();
  taskCreatedRegistrationByWorkspace.clear();
});

it("defers form initialization until first opening and retains the close lifecycle", () => {
  const props = {
    mode: "create" as const,
    onOpenChange: () => undefined,
    workspaceId: DEFAULT_WORKSPACE_ID,
    workflowId: null,
    defaultStepId: null,
    steps: [],
  };
  const view = render(<TaskCreateDialog {...props} open={false} />);
  expect(formInitializationSpy).not.toHaveBeenCalled();

  view.rerender(<TaskCreateDialog {...props} open />);
  expect(formInitializationSpy).toHaveBeenCalled();
  expect(dialogCloseAutoFocusHandler).toBeTypeOf("function");
  formInitializationSpy.mockClear();
  view.rerender(<TaskCreateDialog {...props} open={false} />);
  expect(formInitializationSpy).toHaveBeenCalled();
  expect(dialogCloseAutoFocusHandler).toBeTypeOf("function");
});

describe("TaskCreateDialog focus return (AC-TASKS-TASK-ACTIONS-MENU-001.12)", () => {
  it("returns focus to the given focusReturnRef target on close, overriding Radix's default", () => {
    const target = document.createElement("button");
    document.body.appendChild(target);
    target.focus = vi.fn();
    const focusReturnRef = { current: target };

    renderDialog("edit", { focusReturnRef });
    expect(dialogCloseAutoFocusHandler).toBeTypeOf("function");

    const event = { preventDefault: vi.fn() };
    dialogCloseAutoFocusHandler?.(event);

    expect(event.preventDefault).toHaveBeenCalled();
    expect(target.focus).toHaveBeenCalled();

    document.body.removeChild(target);
  });

  it("leaves Radix's default close-focus behavior alone when the target has left the document", () => {
    const target = document.createElement("button");
    target.focus = vi.fn();
    const focusReturnRef = { current: target };

    renderDialog("edit", { focusReturnRef });
    const event = { preventDefault: vi.fn() };
    dialogCloseAutoFocusHandler?.(event);

    expect(event.preventDefault).not.toHaveBeenCalled();
    expect(target.focus).not.toHaveBeenCalled();
  });

  it("leaves Radix's default close-focus behavior alone when no focusReturnRef is given", () => {
    renderDialog("edit");
    const event = { preventDefault: vi.fn() };
    dialogCloseAutoFocusHandler?.(event);

    expect(event.preventDefault).not.toHaveBeenCalled();
  });
});

describe("TaskCreateDialog Escape dismissal", () => {
  it("prevents Escape from dismissing create mode", () => {
    renderDialog();
    const event = { preventDefault: vi.fn() };

    expect(dialogEscapeHandler).toBeTypeOf("function");
    dialogEscapeHandler?.(event);

    expect(event.preventDefault).toHaveBeenCalledOnce();
  });

  it.each(["edit", "session"] as const)("keeps Escape dismissal available in %s mode", (mode) => {
    renderDialog(mode);
    const event = { preventDefault: vi.fn() };

    expect(dialogEscapeHandler).toBeTypeOf("function");
    dialogEscapeHandler?.(event);

    expect(event.preventDefault).not.toHaveBeenCalled();
  });
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

it("returns to the opening control after background task creation", () => {
  autoFocusNewTasks = false;
  const target = document.createElement("button");
  document.body.appendChild(target);
  try {
    target.focus();
    renderDialog();
    target.blur();
    const event = { preventDefault: vi.fn() };
    dialogCloseAutoFocusHandler?.(event);
    expect(event.preventDefault).toHaveBeenCalled();
    expect(document.activeElement).toBe(target);
  } finally {
    target.remove();
  }
});

describe("TaskCreateDialog task-created plugin callback", () => {
  it("scopes callbacks to a successful create dialog", () => {
    const first = vi.fn();
    const second = vi.fn();
    taskCreatedHandlerByWorkspace.set(DEFAULT_WORKSPACE_ID, first);
    taskCreatedHandlerByWorkspace.set(SECOND_WORKSPACE_ID, second);
    taskSubmitHarness.succeeds = true;
    const firstDialog = renderDialog();
    const completeFirstOpening = taskSubmitHarness.onSuccessByWorkspace.get(DEFAULT_WORKSPACE_ID);
    const secondDialog = renderDialog("create", { workspaceId: SECOND_WORKSPACE_ID });
    fireEvent.submit(firstDialog.container.querySelector("form")!);
    fireEvent.submit(secondDialog.container.querySelector("form")!);
    expect(second).toHaveBeenCalledTimes(1);

    taskSubmitHarness.succeeds = false;
    fireEvent.submit(firstDialog.container.querySelector("form")!);
    expect(first).toHaveBeenCalledTimes(1);
    renderDialog("edit", { workspaceId: "workspace-edit" });
    expect(taskCreatedRegistrationByWorkspace.get("workspace-edit")).toBeNull();

    const firstDialogProps = {
      mode: "create" as const,
      onOpenChange: () => undefined,
      workspaceId: DEFAULT_WORKSPACE_ID,
      workflowId: null,
      defaultStepId: null,
      steps: [],
    };
    firstDialog.rerender(<TaskCreateDialog {...firstDialogProps} open={false} />);
    firstDialog.rerender(<TaskCreateDialog {...firstDialogProps} open />);
    completeFirstOpening?.({ id: "late-task", workspace_id: DEFAULT_WORKSPACE_ID }, "create");
    expect(first).toHaveBeenCalledTimes(1);
    taskSubmitHarness.onSuccessByWorkspace.get(DEFAULT_WORKSPACE_ID)?.(
      { id: "current-task", workspace_id: DEFAULT_WORKSPACE_ID },
      "create",
    );
    expect(first).toHaveBeenCalledTimes(2);

    const completeBeforeModeChange =
      taskSubmitHarness.onSuccessByWorkspace.get(DEFAULT_WORKSPACE_ID);
    firstDialog.rerender(<TaskCreateDialog {...firstDialogProps} mode="edit" open />);
    firstDialog.rerender(<TaskCreateDialog {...firstDialogProps} open />);
    completeBeforeModeChange?.(
      { id: "late-edit-task", workspace_id: DEFAULT_WORKSPACE_ID },
      "create",
    );
    expect(first).toHaveBeenCalledTimes(2);

    cleanup();
    const notifyAfterUnmount = taskSubmitHarness.onSuccessByWorkspace.get(DEFAULT_WORKSPACE_ID);
    notifyAfterUnmount?.({ id: "late-task", workspace_id: DEFAULT_WORKSPACE_ID }, "create");
    expect(first).toHaveBeenCalledTimes(2);
  });
});
