import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { createRef } from "react";
import { ApiError } from "@/lib/api/client";

// All external module mocks must be declared with vi.mock before the import of
// the unit under test so vitest hoists them. The mocks below capture the
// arguments passed to the createTask / launchSession boundaries so we can
// assert that handleCreateSubmit honours CLI-mode parity: empty prompt → no
// create call; non-empty prompt → call with that prompt in the payload.

const pushMock = vi.fn();
const TASK_ID = "task-1";
const RENAMED_TITLE = "Renamed task";
const ORIGINAL_PROMPT = "Original prompt";
const UPDATED_PROMPT = "Updated prompt";
const ORIGINAL_TITLE = "Original title";
const RAW_REPOSITORY_PROVIDER_FAILURE = "raw repository provider failure";
const MAIN_BRANCH = "main";
const TODO_STATE = "TODO" as const;
vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: pushMock, replace: vi.fn(), back: vi.fn() }),
}));

const toastMock = vi.fn();
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: toastMock }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: unknown) => unknown) =>
    selector({
      setActiveDocument: vi.fn(),
      setPlanMode: vi.fn(),
      applyAgentProfileRecentUse: vi.fn(),
    }),
}));

const recordRecentUseMock = vi.fn();
vi.mock("@/lib/agent-profile-recent-use", () => ({
  recordAgentProfileRecentUseBestEffort: (...args: unknown[]) => recordRecentUseMock(...args),
}));

const updateTaskMock = vi.fn();
vi.mock("@/lib/api", () => ({
  updateTask: (...args: unknown[]) => updateTaskMock(...args),
}));

const launchSessionMock = vi.fn(async (..._args: unknown[]) => ({ session_id: "session-1" }));
vi.mock("@/lib/services/session-launch-service", () => ({
  launchSession: (...args: unknown[]) => launchSessionMock(...args),
}));

vi.mock("@/lib/services/session-launch-helpers", () => ({
  buildStartRequest: () => ({ request: { taskId: "t", agentProfileId: "a" } }),
}));

type BuildCreateTaskPayloadCall = {
  repositoriesPayload?: Array<{
    repository_id?: string;
    base_branch?: string;
    checkout_branch?: string;
    fresh_branch?: boolean;
  }>;
  agentProfileId: string;
  executorId: string;
  executorProfileId: string;
  withAgent: boolean;
  trimmedDescription: string;
};

const buildCreateTaskPayloadMock = vi.fn((args: BuildCreateTaskPayloadCall) => ({
  repositories: args.repositoriesPayload,
  agent_profile_id: args.agentProfileId || undefined,
  executor_id: args.executorId || undefined,
  executor_profile_id: args.executorProfileId || undefined,
}));
const validateCreateInputsMock = vi.fn((..._args: unknown[]) => true);
const buildRepositoriesPayloadMock = vi.fn(
  (_args: unknown) => [] as Array<{ repository_id: string; base_branch?: string }>,
);
vi.mock("@/components/task-create-dialog-helpers", () => ({
  activatePlanMode: vi.fn(),
  buildCreateTaskPayload: (args: BuildCreateTaskPayloadCall) => buildCreateTaskPayloadMock(args),
  buildRepositoriesPayload: (args: unknown) => buildRepositoriesPayloadMock(args),
  computeIsTaskStarted: (isEditMode: boolean, editingTask?: { state?: string } | null) =>
    Boolean(
      isEditMode &&
      editingTask?.state &&
      editingTask.state !== TODO_STATE &&
      editingTask.state !== "CREATED",
    ),
  findDuplicateRemoteRepo: () => null,
  hasPendingAttachmentUploads: () => false,
  validateCreateInputs: (...args: unknown[]) => validateCreateInputsMock(...args),
  toMessageAttachments: () => [],
}));

const createTaskRetryMock = vi.fn(async (buildPayload: (consented: string[]) => unknown) => {
  // Invoke the build function so payload-construction side effects (and
  // assertions on it) run as they would in production.
  buildPayload([]);
  return { id: TASK_ID, session_id: "session-1" };
});
vi.mock("@/components/task-create-dialog-fresh-branch-consent", () => ({
  useFreshBranchConsent: (options: {
    createTask?: (payload: unknown) => Promise<{ id: string; session_id?: string }>;
  }) => ({
    pendingDiscard: null,
    ensureFreshBranchConsent: vi.fn(async () => []),
    createTaskWithFreshBranchRetry: (...args: unknown[]) => {
      const buildPayload = args[0] as (consented: string[]) => unknown;
      return options.createTask
        ? options.createTask(buildPayload([]))
        : createTaskRetryMock(buildPayload);
    },
  }),
}));

import { taskSubmitErrorMessage, useTaskSubmitHandlers } from "./task-create-dialog-submit";
import {
  readQueuedTaskCreateLastUsedState,
  resetTaskCreateLastUsedSync,
  syncTaskCreateLastUsed,
} from "./task-create-dialog-handlers";
import type { SubmitHandlersDeps, TaskFormInputsHandle } from "./task-create-dialog-types";

function makeRef(value: string): React.RefObject<TaskFormInputsHandle | null> {
  const ref = createRef<TaskFormInputsHandle>();
  ref.current = {
    getValue: () => value,
    setValue: () => {},
    getAttachments: () => [],
  };
  return ref;
}

function makeDeps(overrides: Partial<SubmitHandlersDeps>): SubmitHandlersDeps {
  return {
    isSessionMode: false,
    isEditMode: false,
    autopilot: false,
    isPassthroughProfile: false,
    taskName: "My CLI task",
    workspaceId: "ws-1",
    workflowId: "wf-1",
    effectiveWorkflowId: "wf-1",
    repositories: [],
    repositoriesDirty: false,
    discoveredRepositories: [],
    workspaceRepositories: [],
    useRemote: false,
    remoteRepos: [],
    prInfoByUrl: {
      info: () => undefined,
      loading: () => false,
      settled: () => true,
      error: () => undefined,
      ensure: () => undefined,
      clear: () => undefined,
    },
    agentProfileId: "agent-1",
    executorId: "exec-1",
    executorProfileId: "execp-1",
    editingTask: null,
    onSuccess: vi.fn(),
    onOpenChange: vi.fn(),
    refreshBranchPolicies: vi.fn(async () => undefined),
    taskId: null,
    descriptionInputRef: makeRef(""),
    setIsCreatingSession: vi.fn(),
    setIsCreatingTask: vi.fn(),
    setHasTitle: vi.fn(),
    setHasDescription: vi.fn(),
    setTaskName: vi.fn(),
    setRepositories: vi.fn(),
    setRemoteRepos: vi.fn(),
    setAgentProfileId: vi.fn(),
    setExecutorId: vi.fn(),
    setMcpServerIds: vi.fn(),
    setMcpServerIdsDirty: vi.fn(),
    setSelectedWorkflowId: vi.fn(),
    setFetchedSteps: vi.fn(),
    clearDraft: vi.fn(),
    freshBranchEnabled: false,
    isLocalExecutor: false,
    repositoryLocalPath: "",
    noRepository: true,
    workspacePath: "",
    mcpServerIds: [],
    mcpServerIdsDirty: false,
    ...overrides,
  };
}

beforeEach(() => {
  resetTaskCreateLastUsedSync({ clearQueued: true });
  buildCreateTaskPayloadMock.mockClear();
  buildRepositoriesPayloadMock.mockReset();
  buildRepositoriesPayloadMock.mockReturnValue([]);
  validateCreateInputsMock.mockClear();
  createTaskRetryMock.mockClear();
  updateTaskMock.mockReset();
  updateTaskMock.mockResolvedValue({ id: TASK_ID, title: RENAMED_TITLE });
  launchSessionMock.mockClear();
  pushMock.mockClear();
  toastMock.mockClear();
  recordRecentUseMock.mockClear();
});

// eslint-disable-next-line max-lines-per-function -- grouped edit regressions share one fixture.
describe("useTaskSubmitHandlers — started task edits", () => {
  it("preserves a legacy overlong title when it was not edited", async () => {
    const legacyTitle = "x".repeat(80);
    const deps = makeDeps({
      isEditMode: true,
      taskName: legacyTitle,
      editingTask: {
        id: TASK_ID,
        title: legacyTitle,
        description: ORIGINAL_PROMPT,
        workflowStepId: "step-1",
        state: "IN_PROGRESS",
      },
      descriptionInputRef: makeRef(""),
    });
    const { result } = renderHook(() => useTaskSubmitHandlers(deps));

    await act(async () => {
      await result.current.handleUpdateWithoutAgent();
    });

    expect(updateTaskMock).toHaveBeenCalledWith(TASK_ID, {});
  });

  it("updates only the title so a locked prompt cannot be cleared", async () => {
    buildRepositoriesPayloadMock.mockReturnValue([
      { repository_id: "repo-1", base_branch: MAIN_BRANCH },
    ]);
    const deps = makeDeps({
      isEditMode: true,
      taskName: RENAMED_TITLE,
      editingTask: {
        id: TASK_ID,
        title: ORIGINAL_TITLE,
        description: ORIGINAL_PROMPT,
        workflowStepId: "step-1",
        state: "IN_PROGRESS",
      },
      descriptionInputRef: makeRef(""),
      noRepository: false,
    });
    const { result } = renderHook(() => useTaskSubmitHandlers(deps));

    await act(async () => {
      await result.current.handleUpdateWithoutAgent();
    });

    expect(updateTaskMock).toHaveBeenCalledWith(TASK_ID, { title: RENAMED_TITLE });
    expect(buildRepositoriesPayloadMock).not.toHaveBeenCalled();
  });

  it("keeps repository updates for tasks that have not started", async () => {
    const repositories = [{ repository_id: "repo-1", base_branch: MAIN_BRANCH }];
    buildRepositoriesPayloadMock.mockReturnValue(repositories);
    const deps = makeDeps({
      isEditMode: true,
      taskName: RENAMED_TITLE,
      editingTask: {
        id: TASK_ID,
        title: ORIGINAL_TITLE,
        description: ORIGINAL_PROMPT,
        workflowStepId: "step-1",
        state: TODO_STATE,
      },
      descriptionInputRef: makeRef(UPDATED_PROMPT),
      noRepository: false,
      repositoriesDirty: true,
    });
    const { result } = renderHook(() => useTaskSubmitHandlers(deps));

    await act(async () => {
      await result.current.handleUpdateWithoutAgent();
    });

    expect(updateTaskMock).toHaveBeenCalledWith(TASK_ID, {
      title: RENAMED_TITLE,
      description: UPDATED_PROMPT,
      repositories,
    });
  });

  it("preserves a policy snapshot during an ordinary unstarted-task edit", async () => {
    const deps = makeDeps({
      isEditMode: true,
      taskName: RENAMED_TITLE,
      editingTask: {
        id: TASK_ID,
        title: ORIGINAL_TITLE,
        description: ORIGINAL_PROMPT,
        workflowStepId: "step-1",
        state: TODO_STATE,
      },
      repositories: [
        {
          key: "row-0",
          repositoryId: "repo-1",
          branch: MAIN_BRANCH,
          branchPolicyId: "policy-1",
        },
      ],
      descriptionInputRef: makeRef(UPDATED_PROMPT),
      noRepository: false,
      repositoriesDirty: false,
    });
    const { result } = renderHook(() => useTaskSubmitHandlers(deps));

    await act(async () => {
      await result.current.handleUpdateWithoutAgent();
    });

    expect(updateTaskMock).toHaveBeenCalledWith(TASK_ID, {
      title: RENAMED_TITLE,
      description: UPDATED_PROMPT,
    });
    expect(buildRepositoriesPayloadMock).not.toHaveBeenCalled();
  });

  it("sends an explicit empty repository list when a policy-backed row is removed", async () => {
    const deps = makeDeps({
      isEditMode: true,
      taskName: ORIGINAL_TITLE,
      editingTask: {
        id: TASK_ID,
        title: ORIGINAL_TITLE,
        description: ORIGINAL_PROMPT,
        workflowStepId: "step-1",
        state: TODO_STATE,
      },
      repositories: [],
      descriptionInputRef: makeRef(ORIGINAL_PROMPT),
      noRepository: false,
      repositoriesDirty: true,
    });
    const { result } = renderHook(() => useTaskSubmitHandlers(deps));

    await act(async () => {
      await result.current.handleUpdateWithoutAgent();
    });

    expect(updateTaskMock).toHaveBeenCalledWith(TASK_ID, {
      description: ORIGINAL_PROMPT,
      repositories: [],
    });
    expect(buildRepositoriesPayloadMock).toHaveBeenCalled();
  });

  it("uses the update-only path when the started edit form is submitted", async () => {
    const onSuccess = vi.fn();
    const deps = makeDeps({
      isEditMode: true,
      taskName: RENAMED_TITLE,
      editingTask: {
        id: TASK_ID,
        title: ORIGINAL_TITLE,
        description: ORIGINAL_PROMPT,
        workflowStepId: "step-1",
        state: "IN_PROGRESS",
      },
      descriptionInputRef: makeRef(ORIGINAL_PROMPT),
      onSuccess,
    });
    const { result } = renderHook(() => useTaskSubmitHandlers(deps));

    await act(async () => {
      await result.current.handleSubmit({ preventDefault: vi.fn() } as never);
    });

    expect(updateTaskMock).toHaveBeenCalledWith(TASK_ID, { title: RENAMED_TITLE });
    expect(launchSessionMock).not.toHaveBeenCalled();
    expect(onSuccess).toHaveBeenCalledWith({ id: TASK_ID, title: RENAMED_TITLE }, "edit");
  });
});

describe("useTaskSubmitHandlers — repository selection edit failures", () => {
  const failures = [
    {
      code: "repository_selection_invalid",
      description: "The selected repository could not be verified.",
    },
    {
      code: "repository_selection_not_found",
      description: "The selected repository was not found.",
    },
    {
      code: "repository_selection_unavailable",
      description: "The repository provider is unavailable. Check the connection and try again.",
    },
  ] as const;

  for (const failure of failures) {
    it(`keeps the dialog open for ${failure.code} in the edit submit handler`, async () => {
      const onOpenChange = vi.fn();
      updateTaskMock.mockRejectedValueOnce(
        new ApiError(RAW_REPOSITORY_PROVIDER_FAILURE, 400, {
          error: RAW_REPOSITORY_PROVIDER_FAILURE,
          error_code: failure.code,
        }),
      );
      const deps = makeDeps({
        isEditMode: true,
        taskName: ORIGINAL_TITLE,
        editingTask: {
          id: TASK_ID,
          title: ORIGINAL_TITLE,
          description: ORIGINAL_PROMPT,
          workflowStepId: "step-1",
          state: TODO_STATE,
        },
        descriptionInputRef: makeRef(ORIGINAL_PROMPT),
        onOpenChange,
      });
      const { result } = renderHook(() => useTaskSubmitHandlers(deps));

      await act(async () => {
        await result.current.handleSubmit({ preventDefault: () => {} } as never);
      });

      expect(onOpenChange).not.toHaveBeenCalled();
      expect(toastMock).toHaveBeenCalledWith(
        expect.objectContaining({ description: failure.description }),
      );
    });

    it(`keeps the dialog open for ${failure.code} in the update-only handler`, async () => {
      const onOpenChange = vi.fn();
      updateTaskMock.mockRejectedValueOnce(
        new ApiError(RAW_REPOSITORY_PROVIDER_FAILURE, 400, {
          error: RAW_REPOSITORY_PROVIDER_FAILURE,
          error_code: failure.code,
        }),
      );
      const deps = makeDeps({
        isEditMode: true,
        taskName: ORIGINAL_TITLE,
        editingTask: {
          id: TASK_ID,
          title: ORIGINAL_TITLE,
          description: ORIGINAL_PROMPT,
          workflowStepId: "step-1",
          state: TODO_STATE,
        },
        descriptionInputRef: makeRef(ORIGINAL_PROMPT),
        onOpenChange,
      });
      const { result } = renderHook(() => useTaskSubmitHandlers(deps));

      await act(async () => {
        await result.current.handleUpdateWithoutAgent();
      });

      expect(onOpenChange).not.toHaveBeenCalled();
      expect(toastMock).toHaveBeenCalledWith(
        expect.objectContaining({ description: failure.description }),
      );
    });
  }
});

// eslint-disable-next-line max-lines-per-function -- create-mode parity cases share transport setup.
describe("useTaskSubmitHandlers — handleCreateSubmit (CLI-mode parity)", () => {
  it("refreshes stale policy options and keeps the dialog open", async () => {
    const refreshBranchPolicies = vi.fn(async () => undefined);
    const onOpenChange = vi.fn();
    const createTask = vi.fn().mockRejectedValue(
      new ApiError("invalid repository branch policy", 400, {
        error: "invalid repository branch policy",
        error_code: "branch_policy_stale",
      }),
    );
    const deps = makeDeps({
      createTask,
      refreshBranchPolicies,
      onOpenChange,
      descriptionInputRef: makeRef("create with a policy"),
    });
    const { result } = renderHook(() => useTaskSubmitHandlers(deps));

    await act(async () => {
      await result.current.handleSubmit({ preventDefault: () => {} } as never);
    });

    expect(refreshBranchPolicies).toHaveBeenCalledTimes(1);
    expect(onOpenChange).not.toHaveBeenCalled();
  });

  it("uses the create-mode transport override", async () => {
    const createTask = vi.fn().mockResolvedValue({
      id: TASK_ID,
      session_id: "session-plugin",
      agent_profile_id: "agent-effective",
    });
    const deps = makeDeps({
      createTask,
      descriptionInputRef: makeRef("inspect the pull request"),
    });
    const { result } = renderHook(() => useTaskSubmitHandlers(deps));

    await act(async () => {
      await result.current.handleSubmit({ preventDefault: () => {} } as never);
    });

    expect(createTask).toHaveBeenCalledWith(
      expect.objectContaining({
        agent_profile_id: "agent-1",
        executor_id: "exec-1",
      }),
    );
    expect(createTaskRetryMock).not.toHaveBeenCalled();
    expect(recordRecentUseMock).not.toHaveBeenCalledWith(
      "task_create",
      expect.anything(),
      expect.any(Function),
    );
  });

  it("skips create when prompt is empty even with cli_passthrough=true (prompt is now required)", async () => {
    const deps = makeDeps({
      isPassthroughProfile: true,
      descriptionInputRef: makeRef(""),
    });
    const { result } = renderHook(() => useTaskSubmitHandlers(deps));

    await act(async () => {
      await result.current.handleSubmit({ preventDefault: () => {} } as never);
    });

    // The plan-mode fallback (handleCreatePlanMode) is what runs when there's
    // no description; verify it was the only path exercised by inspecting the
    // build payload — handleCreatePlanMode builds with withAgent:false, while
    // a passthrough-with-prompt path would build with withAgent:true.
    const calls = buildCreateTaskPayloadMock.mock.calls;
    expect(calls.length).toBe(1);
    expect((calls[0]![0] as { withAgent: boolean }).withAgent).toBe(false);
  });

  it("creates the task with the user's prompt when cli_passthrough=true and prompt is provided", async () => {
    const preserveLastUsed = vi.fn();
    const onOpenChange = vi.fn();
    const deps = makeDeps({
      isPassthroughProfile: true,
      descriptionInputRef: makeRef("run npm test"),
      onOpenChange,
      preserveTaskCreateLastUsedOnClose: preserveLastUsed,
    });
    const { result } = renderHook(() => useTaskSubmitHandlers(deps));

    await act(async () => {
      await result.current.handleSubmit({ preventDefault: () => {} } as never);
    });

    expect(buildCreateTaskPayloadMock).toHaveBeenCalledTimes(1);
    const payloadArg = buildCreateTaskPayloadMock.mock.calls[0]![0] as {
      withAgent: boolean;
      trimmedDescription: string;
    };
    expect(payloadArg.withAgent).toBe(true);
    expect(payloadArg.trimmedDescription).toBe("run npm test");
    expect(preserveLastUsed).toHaveBeenCalledTimes(1);
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(preserveLastUsed.mock.invocationCallOrder[0]).toBeLessThan(
      onOpenChange.mock.invocationCallOrder[0]!,
    );
  });

  it("still creates the task in ACP mode when prompt is provided", async () => {
    const deps = makeDeps({
      isPassthroughProfile: false,
      descriptionInputRef: makeRef("refactor module"),
    });
    const { result } = renderHook(() => useTaskSubmitHandlers(deps));

    await act(async () => {
      await result.current.handleSubmit({ preventDefault: () => {} } as never);
    });

    const payloadArg = buildCreateTaskPayloadMock.mock.calls[0]![0] as {
      withAgent: boolean;
      trimmedDescription: string;
    };
    expect(payloadArg.withAgent).toBe(true);
    expect(payloadArg.trimmedDescription).toBe("refactor module");
  });

  it("replaces the queued last-used overlay with the final create payload", async () => {
    syncTaskCreateLastUsed({
      repository_id: null,
      branch: null,
      agent_profile_id: "agent-before-workflow",
      executor_profile_id: null,
    });
    const deps = makeDeps({
      agentProfileId: "agent-from-workflow",
      executorProfileId: "execp-autopick",
      descriptionInputRef: makeRef("run tests"),
      noRepository: true,
    });
    const { result } = renderHook(() => useTaskSubmitHandlers(deps));

    await act(async () => {
      await result.current.handleSubmit({ preventDefault: () => {} } as never);
    });

    expect(readQueuedTaskCreateLastUsedState()).toEqual({
      agentProfileId: "agent-from-workflow",
      executorProfileId: "execp-autopick",
    });
  });
});

describe("useTaskSubmitHandlers — handleCreateWithoutAgent", () => {
  // "Create without starting agent" must leave the destination to the backend,
  // which parks the task in the workflow's start step. Sending a step from the
  // dialog pinned it to whatever the caller happened to pass as defaultStepId
  // (uniformly "first step by position"), so a workflow whose start step was
  // moved elsewhere was ignored.
  it("sends no workflow_step_id, leaving the start step to the backend", async () => {
    const createTask = vi.fn().mockResolvedValue({ id: TASK_ID });
    const deps = makeDeps({
      createTask,
      descriptionInputRef: makeRef("park this for later"),
    });
    const { result } = renderHook(() => useTaskSubmitHandlers(deps));

    await act(async () => {
      await result.current.handleCreateWithoutAgent();
    });

    expect(createTask).toHaveBeenCalledTimes(1);
    expect(createTask.mock.calls[0][0]).not.toHaveProperty("workflow_step_id");
    expect(buildCreateTaskPayloadMock).toHaveBeenCalledWith(
      expect.objectContaining({ withAgent: false }),
    );
  });
});

describe("taskSubmitErrorMessage", () => {
  it("uses localized copy for repository selection error codes", () => {
    expect(
      taskSubmitErrorMessage(
        new ApiError("raw upstream provider failure", 503, {
          error: "raw upstream provider failure",
          error_code: "repository_selection_unavailable",
        }),
      ),
    ).toBe("The repository provider is unavailable. Check the connection and try again.");
  });

  it("preserves non-repository error messages", () => {
    expect(taskSubmitErrorMessage(new Error("ordinary failure"))).toBe("ordinary failure");
  });
});
