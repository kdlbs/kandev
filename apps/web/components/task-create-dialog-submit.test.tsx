import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { createRef } from "react";

// All external module mocks must be declared with vi.mock before the import of
// the unit under test so vitest hoists them. The mocks below capture the
// arguments passed to the createTask / launchSession boundaries so we can
// assert that handleCreateSubmit honours CLI-mode parity: empty prompt → no
// create call; non-empty prompt → call with that prompt in the payload.

const pushMock = vi.fn();
vi.mock("next/navigation", () => ({
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

vi.mock("@/lib/api", () => ({
  updateTask: vi.fn(),
}));

vi.mock("@/lib/services/session-launch-service", () => ({
  launchSession: vi.fn(async () => ({ session_id: "session-1" })),
}));

vi.mock("@/lib/services/session-launch-helpers", () => ({
  buildStartRequest: () => ({ request: { taskId: "t", agentProfileId: "a" } }),
}));

const buildCreateTaskPayloadMock = vi.fn((..._args: unknown[]) => ({
  workflow_step_id: "step-1",
}));
const validateCreateInputsMock = vi.fn((..._args: unknown[]) => true);
vi.mock("@/components/task-create-dialog-helpers", () => ({
  activatePlanMode: vi.fn(),
  buildCreateTaskPayload: (...args: unknown[]) => buildCreateTaskPayloadMock(...args),
  buildRepositoriesPayload: () => [],
  validateCreateInputs: (...args: unknown[]) => validateCreateInputsMock(...args),
  toMessageAttachments: () => [],
}));

const createTaskRetryMock = vi.fn(async (buildPayload: (consented: string[]) => unknown) => {
  // Invoke the build function so payload-construction side effects (and
  // assertions on it) run as they would in production.
  buildPayload([]);
  return { id: "task-1", session_id: "session-1" };
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
    useGitHubUrl: false,
    githubUrl: "",
    githubPrHeadBranch: null,
    githubBranch: "",
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
    setGitHubBranch: vi.fn(),
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
  buildCreateTaskPayloadMock.mockClear();
  validateCreateInputsMock.mockClear();
  createTaskRetryMock.mockClear();
  pushMock.mockClear();
  toastMock.mockClear();
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
    const deps = makeDeps({
      isPassthroughProfile: true,
      descriptionInputRef: makeRef("run npm test"),
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
});
