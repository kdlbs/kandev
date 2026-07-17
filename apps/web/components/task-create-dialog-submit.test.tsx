import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { createRef } from "react";

// All external module mocks must be declared with vi.mock before the import of
// the unit under test so vitest hoists them. The mocks below capture the
// arguments passed to the createTask / launchSession boundaries so we can
// assert that handleCreateSubmit honours CLI-mode parity: empty prompt → no
// create call; non-empty prompt → call with that prompt in the payload.

const pushMock = vi.fn();
const TASK_ID = "task-1";
const RENAMED_TITLE = "Renamed task";
const ORIGINAL_PROMPT = "Original prompt";
vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: pushMock, replace: vi.fn(), back: vi.fn() }),
}));

const toastMock = vi.fn();
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: toastMock }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: unknown) => unknown) =>
    selector({ setActiveDocument: vi.fn(), setPlanMode: vi.fn() }),
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
  workflow_step_id: "step-1",
  repositories: args.repositoriesPayload,
  agent_profile_id: args.agentProfileId || undefined,
  executor_id: args.executorId || undefined,
  executor_profile_id: args.executorProfileId || undefined,
}));
const validateCreateInputsMock = vi.fn((..._args: unknown[]) => true);
vi.mock("@/components/task-create-dialog-helpers", () => ({
  activatePlanMode: vi.fn(),
  buildCreateTaskPayload: (args: BuildCreateTaskPayloadCall) => buildCreateTaskPayloadMock(args),
  buildRepositoriesPayload: () => [],
  computeIsTaskStarted: (isEditMode: boolean, editingTask?: { state?: string } | null) =>
    Boolean(
      isEditMode &&
      editingTask?.state &&
      editingTask.state !== "TODO" &&
      editingTask.state !== "CREATED",
    ),
  findDuplicateRemoteRepo: () => null,
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
  useFreshBranchConsent: () => ({
    pendingDiscard: null,
    ensureFreshBranchConsent: vi.fn(async () => []),
    createTaskWithFreshBranchRetry: (...args: unknown[]) =>
      createTaskRetryMock(args[0] as (consented: string[]) => unknown),
  }),
}));

import { useTaskSubmitHandlers } from "./task-create-dialog-submit";
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
    isPassthroughProfile: false,
    taskName: "My CLI task",
    workspaceId: "ws-1",
    workflowId: "wf-1",
    effectiveWorkflowId: "wf-1",
    effectiveDefaultStepId: "step-1",
    repositories: [],
    discoveredRepositories: [],
    workspaceRepositories: [],
    useRemote: false,
    remoteRepos: [],
    prInfoByUrl: {
      info: () => undefined,
      loading: () => false,
      ensure: () => undefined,
      clear: () => undefined,
    },
    agentProfileId: "agent-1",
    executorId: "exec-1",
    executorProfileId: "execp-1",
    editingTask: null,
    onSuccess: vi.fn(),
    onOpenChange: vi.fn(),
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
    setSelectedWorkflowId: vi.fn(),
    setFetchedSteps: vi.fn(),
    clearDraft: vi.fn(),
    freshBranchEnabled: false,
    isLocalExecutor: false,
    repositoryLocalPath: "",
    noRepository: true,
    workspacePath: "",
    ...overrides,
  };
}

beforeEach(() => {
  resetTaskCreateLastUsedSync({ clearQueued: true });
  buildCreateTaskPayloadMock.mockClear();
  validateCreateInputsMock.mockClear();
  createTaskRetryMock.mockClear();
  updateTaskMock.mockReset();
  updateTaskMock.mockResolvedValue({ id: TASK_ID, title: RENAMED_TITLE });
  launchSessionMock.mockClear();
  pushMock.mockClear();
  toastMock.mockClear();
});

describe("useTaskSubmitHandlers — started task edits", () => {
  it("updates only the title so a locked prompt cannot be cleared", async () => {
    const deps = makeDeps({
      isEditMode: true,
      taskName: RENAMED_TITLE,
      editingTask: {
        id: TASK_ID,
        title: "Original title",
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

    expect(updateTaskMock).toHaveBeenCalledWith(TASK_ID, { title: RENAMED_TITLE });
  });

  it("uses the update-only path when the started edit form is submitted", async () => {
    const onSuccess = vi.fn();
    const deps = makeDeps({
      isEditMode: true,
      taskName: RENAMED_TITLE,
      editingTask: {
        id: TASK_ID,
        title: "Original title",
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

describe("useTaskSubmitHandlers — handleCreateSubmit (CLI-mode parity)", () => {
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
