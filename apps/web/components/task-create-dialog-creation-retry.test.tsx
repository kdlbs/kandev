/* eslint-disable max-lines, max-lines-per-function -- one recovery regression fixture covers start, create-only, and retry paths. */
import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import { loadTaskCreationReceipt } from "@/lib/task-creation-receipt";
import type { SubmitHandlersDeps, TaskFormInputsHandle } from "./task-create-dialog-types";

const mocks = vi.hoisted(() => ({
  createTask: vi.fn(),
  lookup: vi.fn(),
  fetchTask: vi.fn(),
  onSuccess: vi.fn(),
  onOpenChange: vi.fn(),
  clearDraft: vi.fn(),
  toast: vi.fn(),
  push: vi.fn(),
  freshConsent: vi.fn(),
  ensureConsent: vi.fn(),
  repositoriesPayload: vi.fn(),
}));

vi.mock("@/lib/api", () => ({
  createTask: (...args: unknown[]) => mocks.createTask(...args),
  updateTask: vi.fn(),
  fetchTask: (...args: unknown[]) => mocks.fetchTask(...args),
  fetchTaskByExternalId: (...args: unknown[]) => mocks.lookup(...args),
}));

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: mocks.push, replace: vi.fn(), back: vi.fn() }),
}));
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mocks.toast }),
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      auth: { user: { id: "user-1" } },
      userSettings: { autoFocusNewTasks: true },
      setActiveDocument: vi.fn(),
      setPlanMode: vi.fn(),
      applyAgentProfileRecentUse: vi.fn(),
    }),
}));
vi.mock("@/lib/services/session-launch-service", () => ({
  launchSession: vi.fn(),
}));
vi.mock("@/lib/services/session-launch-helpers", () => ({
  buildStartRequest: () => ({ request: {} }),
}));
vi.mock("@/lib/agent-profile-recent-use", () => ({
  recordAgentProfileRecentUseBestEffort: vi.fn(),
}));
vi.mock("@/lib/api/domains/task-runner-api", () => ({
  switchTaskRunner: vi.fn(),
}));
vi.mock("@/hooks/domains/task/use-task-edit-dialog-dependencies", () => ({
  isTaskDependencyUpdateFailure: () => false,
}));
vi.mock("@/components/task-create-dialog-handlers", () => ({
  queueTaskCreateLastUsedFromPayload: vi.fn(),
}));
vi.mock("@/components/task-create-dialog-fresh-branch-consent", () => ({
  dirtyFilesFromApiError: (error: unknown) => {
    if (!(error instanceof ApiError) || error.status !== 409) return null;
    const body = error.body as { dirty_files?: unknown } | null;
    return Array.isArray(body?.dirty_files) ? (body.dirty_files as string[]) : null;
  },
  useFreshBranchConsent: () => ({
    pendingDiscard: null,
    ensureFreshBranchConsent: (...args: unknown[]) => mocks.ensureConsent(...args),
    confirmForDirtyFiles: (...args: unknown[]) => mocks.freshConsent(...args),
    createTaskWithFreshBranchRetry: vi.fn(),
  }),
}));
vi.mock("@/components/task-create-dialog-helpers", () => ({
  activatePlanMode: vi.fn(),
  buildCreateTaskPayload: (args: Record<string, unknown>) => ({
    workspace_id: args.workspaceId,
    workflow_id: args.effectiveWorkflowId,
    ...(args.autoTitle ? { auto_title: true } : { title: args.trimmedTitle }),
    description: args.trimmedDescription,
    state: args.withAgent ? "IN_PROGRESS" : "CREATED",
    start_agent: args.withAgent || undefined,
    plan_mode: args.planMode || undefined,
    repositories: args.repositoriesPayload,
    attachments: args.attachments,
    agent_profile_id: args.agentProfileId,
    executor_id: args.executorId,
    executor_profile_id: args.executorProfileId,
  }),
  buildRepositoriesPayload: (...args: unknown[]) => mocks.repositoriesPayload(...args),
  computeIsTaskStarted: () => false,
  findDuplicateRemoteRepo: () => null,
  findUnresolvedProviderRemote: () => null,
  hasPendingAttachmentUploads: () => false,
  toMessageAttachments: (
    attachments: Array<{
      attachmentId?: string;
      data?: string;
      fileName: string;
      mimeType: string;
      size: number;
      isImage: boolean;
      deliveryMode: "prompt" | "path";
    }>,
  ) =>
    attachments.length === 0
      ? undefined
      : attachments.map((attachment) => ({
          type: attachment.isImage ? "image" : "resource",
          mime_type: attachment.mimeType,
          name: attachment.fileName,
          size_bytes: attachment.size,
          ...(attachment.attachmentId
            ? { attachment_id: attachment.attachmentId }
            : { data: attachment.data ?? "" }),
          delivery_mode: attachment.deliveryMode,
        })),
  validateCreateInputs: () => true,
  RUNNER_INELIGIBLE_REASON_KEYS: {},
}));

import { useTaskCreationRecovery } from "./task-create-dialog-recovery";
import { useTaskSubmitHandlers } from "./task-create-dialog-submit";

const ORIGINAL_TITLE = "Incident task";
const ORIGINAL_DESCRIPTION = "Task brief";
const LOST_RESPONSE = "Failed to fetch";
const REJECTION_MESSAGE = "invalid request";
const DIRTY_FILE = "src/app.ts";
const SUBMIT_EVENT = { preventDefault: () => {} } as never;

let currentTitle = ORIGINAL_TITLE;
let currentDescription = ORIGINAL_DESCRIPTION;
let currentAttachments: import("@/components/task/chat/file-attachment").FileAttachment[] = [];
const descriptionRef = {
  current: {
    getValue: () => currentDescription,
    setValue: (value: string) => {
      currentDescription = value;
    },
    getAttachments: () => currentAttachments,
  } as TaskFormInputsHandle,
} as React.RefObject<TaskFormInputsHandle | null>;

function attachment(attachmentId: string) {
  return {
    id: attachmentId,
    attachmentId,
    fileName: `${attachmentId}.txt`,
    mimeType: "text/plain",
    size: 4,
    isImage: false,
    deliveryMode: "prompt" as const,
  };
}

function baseDeps(overrides: Partial<SubmitHandlersDeps> = {}): SubmitHandlersDeps {
  return {
    isSessionMode: false,
    isEditMode: false,
    autopilot: false,
    priority: "medium",
    workflowAgentOverrides: {},
    isPassthroughProfile: false,
    taskName: currentTitle,
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
    seededExecutorProfileId: null,
    editingTask: null,
    onSuccess: mocks.onSuccess,
    onOpenChange: mocks.onOpenChange,
    refreshBranchPolicies: vi.fn(async () => undefined),
    taskId: null,
    descriptionInputRef: descriptionRef,
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
    setInitialWorkspaceLayout: vi.fn(),
    clearDraft: mocks.clearDraft,
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

type RecoveryArgs = Parameters<typeof useTaskCreationRecovery>[0];

function useWired(
  overrides: Partial<SubmitHandlersDeps> = {},
  recoveryOverrides: Partial<RecoveryArgs> = {},
) {
  const recovery = useTaskCreationRecovery({
    enabled: true,
    open: true,
    userId: "user-1",
    workspaceId: "ws-1",
    draftId: "task",
    ...recoveryOverrides,
  });
  const handlers = useTaskSubmitHandlers({ ...baseDeps(overrides), recovery });
  return { recovery, handlers };
}

function createdResponse(id = "task-1", sessionId: string | null = "session-1") {
  return {
    id,
    workspace_id: "ws-1",
    deduplicated: false,
    creation_complete: true,
    ...(sessionId ? { session_id: sessionId } : {}),
  };
}

function notFound() {
  return new ApiError("not found", 404, { error: "not found" });
}

beforeEach(() => {
  window.sessionStorage.clear();
  currentTitle = ORIGINAL_TITLE;
  currentDescription = ORIGINAL_DESCRIPTION;
  currentAttachments = [];
  mocks.createTask.mockReset();
  mocks.lookup.mockReset();
  mocks.fetchTask.mockReset();
  mocks.onSuccess.mockReset();
  mocks.onOpenChange.mockReset();
  mocks.clearDraft.mockReset();
  mocks.toast.mockReset();
  mocks.push.mockReset();
  mocks.freshConsent.mockReset();
  mocks.ensureConsent.mockReset();
  mocks.ensureConsent.mockResolvedValue([]);
  mocks.repositoriesPayload.mockReset();
  mocks.repositoriesPayload.mockReturnValue([]);
  mocks.lookup.mockRejectedValue(notFound());
});

afterEach(() => {
  window.sessionStorage.clear();
});

// @covers AC-TASKS-CREATE-RECOVERY-001.1, AC-TASKS-CREATE-RECOVERY-001.2
describe("native creation retry protection", () => {
  it("retains one native task when creation succeeds but the browser response fails", async () => {
    mocks.createTask
      .mockRejectedValueOnce(new Error(LOST_RESPONSE))
      .mockResolvedValueOnce(createdResponse());
    const { result } = renderHook(() => useWired());

    await act(async () => {
      await result.current.handlers.handleSubmit(SUBMIT_EVENT);
    });
    expect(result.current.recovery.receipt?.state).toBe("unknown");
    expect(result.current.recovery.receipt?.everUncertain).toBe(true);
    expect(mocks.onSuccess).not.toHaveBeenCalled();

    await act(async () => {
      await result.current.handlers.handleSubmit(SUBMIT_EVENT);
    });

    expect(mocks.createTask).toHaveBeenCalledTimes(2);
    const first = mocks.createTask.mock.calls[0]![0] as { external_id: string };
    const second = mocks.createTask.mock.calls[1]![0] as { external_id: string };
    expect(first.external_id).toBeTruthy();
    expect(second.external_id).toBe(first.external_id);
    expect(mocks.onSuccess).toHaveBeenCalledTimes(1);
    expect(mocks.onSuccess).toHaveBeenCalledWith(
      expect.objectContaining({ id: "task-1" }),
      "create",
      expect.objectContaining({ taskSessionId: "session-1" }),
    );
    expect(mocks.onOpenChange).toHaveBeenCalledWith(false);
  });

  it("admits one creation when submission actions overlap in the same tick", async () => {
    let resolveCreate!: (value: ReturnType<typeof createdResponse>) => void;
    mocks.createTask.mockReturnValue(
      new Promise((resolve) => {
        resolveCreate = resolve;
      }),
    );
    const { result } = renderHook(() => useWired());

    let first!: Promise<void>;
    let second!: Promise<void>;
    await act(async () => {
      first = result.current.handlers.handleSubmit(SUBMIT_EVENT);
      second = result.current.handlers.handleSubmit(SUBMIT_EVENT);
      resolveCreate(createdResponse());
      await Promise.all([first, second]);
    });

    expect(mocks.createTask).toHaveBeenCalledTimes(1);
  });

  it("retries a fresh-branch conflict with the same identity and renewed consent", async () => {
    mocks.repositoriesPayload.mockReturnValueOnce([
      { repository_id: "repo-1", base_branch: "main", fresh_branch: true },
    ]);
    mocks.freshConsent.mockResolvedValueOnce([DIRTY_FILE]);
    mocks.createTask
      .mockRejectedValueOnce(new ApiError("dirty worktree", 409, { dirty_files: [DIRTY_FILE] }))
      .mockResolvedValueOnce(createdResponse());
    const { result } = renderHook(() =>
      useWired({
        freshBranchEnabled: true,
        isLocalExecutor: true,
        repositoryLocalPath: "/repo",
        noRepository: false,
      }),
    );

    await act(async () => {
      await result.current.handlers.handleSubmit(SUBMIT_EVENT);
    });

    expect(mocks.freshConsent).toHaveBeenCalledWith([DIRTY_FILE]);
    expect(mocks.createTask).toHaveBeenCalledTimes(2);
    const first = mocks.createTask.mock.calls[0]![0] as { external_id: string };
    const second = mocks.createTask.mock.calls[1]![0] as {
      external_id: string;
      repositories?: Array<Record<string, unknown>>;
    };
    expect(second.external_id).toBe(first.external_id);
    expect(second.repositories?.[0]).toMatchObject({
      confirm_discard: true,
      consented_dirty_files: [DIRTY_FILE],
    });
  });

  it("uses a fresh identity for an independent submission after acceptance", async () => {
    mocks.createTask.mockResolvedValue(createdResponse());
    const { result } = renderHook(() => useWired());

    await act(async () => {
      await result.current.handlers.handleSubmit(SUBMIT_EVENT);
    });
    await act(async () => {
      await result.current.handlers.handleSubmit(SUBMIT_EVENT);
    });

    expect(mocks.createTask).toHaveBeenCalledTimes(2);
    const first = mocks.createTask.mock.calls[0]![0] as { external_id: string };
    const second = mocks.createTask.mock.calls[1]![0] as { external_id: string };
    expect(first.external_id).not.toBe(second.external_id);
  });

  it("keeps the captured submission values when a newer draft is present", async () => {
    mocks.createTask
      .mockRejectedValueOnce(new Error(LOST_RESPONSE))
      .mockResolvedValueOnce(createdResponse());
    const { result } = renderHook(() => useWired());

    await act(async () => {
      await result.current.handlers.handleSubmit(SUBMIT_EVENT);
    });
    currentDescription = "Newer draft edit";
    await act(async () => {
      await result.current.handlers.handleSubmit(SUBMIT_EVENT);
    });

    const retryPayload = mocks.createTask.mock.calls[1]![0] as { description: string };
    expect(retryPayload.description).toBe(ORIGINAL_DESCRIPTION);
    expect(currentDescription).toBe("Newer draft edit");
  });

  it("does not create again or close after an acceptance callback throws", async () => {
    mocks.createTask.mockResolvedValue(createdResponse());
    mocks.onSuccess.mockImplementation(() => {
      throw new Error("presentation failed");
    });
    const { result } = renderHook(() => useWired());

    await act(async () => {
      await result.current.handlers.handleSubmit(SUBMIT_EVENT);
    });
    expect(result.current.recovery.receipt?.state).toBe("accepted");
    expect(mocks.onOpenChange).not.toHaveBeenCalled();

    mocks.lookup.mockResolvedValue({
      id: "task-1",
      workspace_id: "ws-1",
      external_id: result.current.recovery.receipt!.externalId,
      creation_complete: true,
    });
    await act(async () => {
      await result.current.handlers.recovery.checkStatus();
    });
    expect(mocks.createTask).toHaveBeenCalledTimes(1);
    expect(mocks.onSuccess).toHaveBeenCalledTimes(1);
    expect(result.current.recovery.receipt?.taskId).toBe("task-1");
  });
});

// @covers AC-TASKS-CREATE-RECOVERY-001.5, AC-TASKS-CREATE-RECOVERY-001.6
describe("native creation outcome evidence", () => {
  it("keeps a proven first-attempt rejection editable for a corrected submission", async () => {
    mocks.createTask
      .mockRejectedValueOnce(
        new ApiError(REJECTION_MESSAGE, 400, {
          error: REJECTION_MESSAGE,
          creation_rejected: true,
        }),
      )
      .mockResolvedValueOnce(createdResponse());
    const { result } = renderHook(() => useWired());

    await act(async () => {
      await result.current.handlers.handleSubmit(SUBMIT_EVENT);
    });
    expect(result.current.recovery.receipt).toBeNull();
    expect(mocks.toast).toHaveBeenCalled();
    expect(mocks.onSuccess).not.toHaveBeenCalled();

    await act(async () => {
      await result.current.handlers.handleSubmit(SUBMIT_EVENT);
    });
    expect(mocks.createTask).toHaveBeenCalledTimes(2);
    const first = mocks.createTask.mock.calls[0]![0] as { external_id: string };
    const second = mocks.createTask.mock.calls[1]![0] as { external_id: string };
    expect(first.external_id).not.toBe(second.external_id);
  });

  it("keeps sticky uncertainty when a later request carries a rejection marker", async () => {
    mocks.createTask.mockRejectedValueOnce(new Error(LOST_RESPONSE)).mockRejectedValueOnce(
      new ApiError(REJECTION_MESSAGE, 400, {
        error: REJECTION_MESSAGE,
        creation_rejected: true,
      }),
    );
    const { result } = renderHook(() => useWired());

    await act(async () => {
      await result.current.handlers.handleSubmit(SUBMIT_EVENT);
    });
    await act(async () => {
      await result.current.handlers.handleSubmit(SUBMIT_EVENT);
    });

    expect(mocks.createTask).toHaveBeenCalledTimes(2);
    const first = mocks.createTask.mock.calls[0]![0] as { external_id: string };
    const second = mocks.createTask.mock.calls[1]![0] as { external_id: string };
    expect(first.external_id).toBe(second.external_id);
    expect(result.current.recovery.receipt?.state).toBe("unknown");
    expect(result.current.recovery.receipt?.everUncertain).toBe(true);
  });

  it("reports incomplete creation and settles it on a later status check without another POST", async () => {
    mocks.createTask.mockRejectedValueOnce(new Error(LOST_RESPONSE));
    const { result } = renderHook(() => useWired());
    await act(async () => {
      await result.current.handlers.handleSubmit(SUBMIT_EVENT);
    });
    const receipt = result.current.recovery.receipt!;
    mocks.lookup.mockResolvedValue({
      id: "task-1",
      workspace_id: "ws-1",
      external_id: receipt.externalId,
      creation_complete: false,
    });

    await act(async () => {
      await result.current.handlers.recovery.checkStatus();
    });
    expect(result.current.recovery.receipt).toMatchObject({
      state: "incomplete",
      taskId: "task-1",
    });
    expect(mocks.createTask).toHaveBeenCalledTimes(1);

    mocks.lookup.mockResolvedValue({
      id: "task-1",
      workspace_id: "ws-1",
      external_id: receipt.externalId,
      creation_complete: true,
      primary_session_id: "session-1",
    });
    mocks.fetchTask.mockResolvedValue({ id: "task-1", primary_session_id: "session-1" });
    await act(async () => {
      await result.current.handlers.recovery.checkStatus();
    });
    expect(mocks.createTask).toHaveBeenCalledTimes(1);
    expect(mocks.onSuccess).toHaveBeenCalledWith(
      expect.objectContaining({ id: "task-1" }),
      "create",
      expect.anything(),
    );
  });

  it("opens an incomplete task behind the creation-pending gate", async () => {
    mocks.createTask.mockRejectedValueOnce(new Error(LOST_RESPONSE));
    const { result } = renderHook(() => useWired());
    await act(async () => {
      await result.current.handlers.handleSubmit(SUBMIT_EVENT);
    });
    const receipt = result.current.recovery.receipt!;
    mocks.lookup.mockResolvedValue({
      id: "task-1",
      workspace_id: "ws-1",
      external_id: receipt.externalId,
      creation_complete: false,
    });
    await act(async () => {
      await result.current.handlers.recovery.checkStatus();
    });
    expect(result.current.recovery.receipt?.state).toBe("incomplete");

    mocks.fetchTask.mockResolvedValue({ id: "task-1", primary_session_id: null });
    await act(async () => {
      await result.current.handlers.recovery.openTask();
    });

    expect(mocks.onOpenChange).toHaveBeenCalledWith(false);
    expect(mocks.push).toHaveBeenCalledWith(expect.stringContaining("creation=pending"));
    expect(mocks.createTask).toHaveBeenCalledTimes(1);
  });

  it("opens a known accepted task without launching or creating again", async () => {
    mocks.createTask.mockResolvedValue(createdResponse());
    mocks.onSuccess.mockImplementation(() => {
      throw new Error("presentation failed");
    });
    mocks.fetchTask.mockResolvedValue({ id: "task-1", primary_session_id: "session-1" });
    const { result } = renderHook(() => useWired());
    await act(async () => {
      await result.current.handlers.handleSubmit(SUBMIT_EVENT);
    });

    await act(async () => {
      await result.current.handlers.recovery.openTask();
    });

    expect(mocks.createTask).toHaveBeenCalledTimes(1);
    expect(mocks.onOpenChange).toHaveBeenCalledWith(false);
    expect(mocks.push).toHaveBeenCalledTimes(1);
  });

  it("hydrates a plan-mode task without a session and never creates a second one", async () => {
    currentDescription = "";
    mocks.createTask.mockResolvedValue(createdResponse("task-1", null));
    mocks.fetchTask.mockResolvedValue({ id: "task-1", primary_session_id: null });
    const { result } = renderHook(() => useWired());

    await act(async () => {
      await result.current.handlers.handleSubmit(SUBMIT_EVENT);
    });
    await waitFor(() => expect(mocks.onSuccess).toHaveBeenCalled());

    const payload = mocks.createTask.mock.calls[0]![0] as { plan_mode?: boolean };
    expect(payload.plan_mode).toBe(true);
    expect(mocks.createTask).toHaveBeenCalledTimes(1);
  });
});

type WiredProps = {
  deps: Partial<SubmitHandlersDeps>;
  recovery: Partial<RecoveryArgs>;
};

// @covers AC-TASKS-CREATE-RECOVERY-001.8
describe("native recovery owner fencing", () => {
  it("does not hand a delayed create response to a reopened dialog", async () => {
    let resolveCreate!: (value: ReturnType<typeof createdResponse>) => void;
    mocks.createTask.mockReturnValue(
      new Promise((resolve) => {
        resolveCreate = resolve;
      }),
    );
    const { result, rerender } = renderHook(
      (props: WiredProps) => useWired(props.deps, props.recovery),
      { initialProps: { deps: {}, recovery: {} } },
    );

    let pending!: Promise<void>;
    await act(async () => {
      pending = result.current.handlers.handleSubmit(SUBMIT_EVENT);
      await Promise.resolve();
    });
    rerender({ deps: {}, recovery: { open: false } });
    rerender({ deps: {}, recovery: { open: true } });
    await act(async () => {
      resolveCreate(createdResponse());
      await pending;
    });

    expect(mocks.onSuccess).not.toHaveBeenCalled();
    expect(mocks.onOpenChange).not.toHaveBeenCalled();
    expect(mocks.push).not.toHaveBeenCalled();
    expect(result.current.recovery.receipt?.state).toBe("accepted");
    expect(result.current.recovery.receipt?.taskId).toBe("task-1");
  });

  it("does not publish a receipt after close/reopen during branch consent", async () => {
    let resolveConsent!: (value: string[]) => void;
    mocks.ensureConsent.mockReturnValue(
      new Promise((resolve) => {
        resolveConsent = resolve;
      }),
    );
    const { result, rerender } = renderHook(
      (props: WiredProps) => useWired(props.deps, props.recovery),
      { initialProps: { deps: {}, recovery: {} } },
    );

    let pending!: Promise<void>;
    await act(async () => {
      pending = result.current.handlers.handleSubmit(SUBMIT_EVENT);
      await Promise.resolve();
    });
    rerender({ deps: {}, recovery: { open: false } });
    rerender({ deps: {}, recovery: { open: true } });
    await act(async () => {
      resolveConsent([]);
      await pending;
    });

    expect(mocks.createTask).not.toHaveBeenCalled();
    expect(result.current.recovery.receipt).toBeNull();
  });

  it("does not POST a retry after the owner scope changes during lookup", async () => {
    mocks.createTask.mockRejectedValueOnce(new Error(LOST_RESPONSE));
    let resolveLookup!: (value: unknown) => void;
    mocks.lookup.mockReturnValue(
      new Promise((resolve) => {
        resolveLookup = resolve;
      }),
    );
    const { result, rerender } = renderHook(
      (props: WiredProps) => useWired(props.deps, props.recovery),
      { initialProps: { deps: {}, recovery: {} } },
    );

    await act(async () => {
      await result.current.handlers.handleSubmit(SUBMIT_EVENT);
    });
    let pending!: Promise<void>;
    await act(async () => {
      pending = result.current.handlers.handleSubmit(SUBMIT_EVENT);
      await Promise.resolve();
    });
    expect(mocks.lookup).toHaveBeenCalledTimes(1);

    rerender({ deps: {}, recovery: { workspaceId: "ws-2" } });
    await act(async () => {
      resolveLookup({ kind: "absent" });
      await pending;
    });

    expect(mocks.createTask).toHaveBeenCalledTimes(1);
    expect(mocks.onOpenChange).not.toHaveBeenCalled();
    expect(
      loadTaskCreationReceipt({ userId: "user-1", workspaceId: "ws-1", draftId: "task" }),
    ).toMatchObject({ state: "unknown", everUncertain: true });
  });

  it("does not present an accepted task after the owner scope changes during hydration", async () => {
    mocks.createTask.mockResolvedValue(createdResponse("task-1", null));
    let resolveTask!: (value: unknown) => void;
    mocks.fetchTask.mockReturnValue(
      new Promise((resolve) => {
        resolveTask = resolve;
      }),
    );
    const { result, rerender } = renderHook(
      (props: WiredProps) => useWired(props.deps, props.recovery),
      {
        initialProps: {
          deps: { isPassthroughProfile: true },
          recovery: {},
        },
      },
    );

    let pending!: Promise<void>;
    await act(async () => {
      pending = result.current.handlers.handleSubmit(SUBMIT_EVENT);
      await Promise.resolve();
    });
    await waitFor(() => expect(mocks.fetchTask).toHaveBeenCalled());

    rerender({ deps: { isPassthroughProfile: true }, recovery: { workspaceId: "ws-2" } });
    await act(async () => {
      resolveTask({ id: "task-1", primary_session_id: "session-1" });
      await pending;
    });

    expect(mocks.onSuccess).not.toHaveBeenCalled();
    expect(mocks.onOpenChange).not.toHaveBeenCalled();
    expect(mocks.push).not.toHaveBeenCalled();
  });
});

// @covers AC-TASKS-CREATE-RECOVERY-001.4, AC-TASKS-CREATE-RECOVERY-001.8
describe("newer draft preservation during accepted handoff", () => {
  async function submitWithNewerDraft(mutate: () => void) {
    let resolveCreate!: (value: ReturnType<typeof createdResponse>) => void;
    mocks.createTask.mockReturnValue(
      new Promise((resolve) => {
        resolveCreate = resolve;
      }),
    );
    const { result, rerender } = renderHook(() => useWired());
    let pending!: Promise<void>;
    await act(async () => {
      pending = result.current.handlers.handleSubmit(SUBMIT_EVENT);
      await Promise.resolve();
    });
    mutate();
    rerender();
    await act(async () => {
      resolveCreate(createdResponse());
      await pending;
    });
    return result;
  }

  it("treats an unchanged auto-generated title as matching the submission", async () => {
    mocks.createTask.mockResolvedValue(createdResponse());
    const { result } = renderHook(() => useWired({ autoTitle: true }));

    await act(async () => {
      await result.current.handlers.handleSubmit(SUBMIT_EVENT);
    });

    expect(mocks.onSuccess).toHaveBeenCalledTimes(1);
    expect(mocks.onOpenChange).toHaveBeenCalledWith(false);
    expect(mocks.clearDraft).toHaveBeenCalled();
  });

  it("keeps a title edit open when auto-generated titles are enabled", async () => {
    let resolveCreate!: (value: ReturnType<typeof createdResponse>) => void;
    mocks.createTask.mockReturnValue(
      new Promise((resolve) => {
        resolveCreate = resolve;
      }),
    );
    const { result, rerender } = renderHook(() => useWired({ autoTitle: true }));
    let pending!: Promise<void>;
    await act(async () => {
      pending = result.current.handlers.handleSubmit(SUBMIT_EVENT);
      await Promise.resolve();
    });
    currentTitle = "Edited auto title";
    rerender();
    await act(async () => {
      resolveCreate(createdResponse());
      await pending;
    });

    expect(mocks.onSuccess).not.toHaveBeenCalled();
    expect(mocks.onOpenChange).not.toHaveBeenCalled();
    expect(mocks.clearDraft).not.toHaveBeenCalled();
    expect(result.current.recovery.receipt?.state).toBe("accepted");
  });

  it("keeps a title-only edit open with Open task", async () => {
    const result = await submitWithNewerDraft(() => {
      currentTitle = "Edited title";
    });
    expect(mocks.clearDraft).not.toHaveBeenCalled();
    expect(mocks.onOpenChange).not.toHaveBeenCalled();
    expect(mocks.push).not.toHaveBeenCalled();
    expect(mocks.onSuccess).not.toHaveBeenCalled();
    expect(result.current.recovery.receipt?.state).toBe("accepted");
  });

  it("keeps an attachment-only edit open with Open task", async () => {
    const result = await submitWithNewerDraft(() => {
      currentAttachments = [attachment("upload-2")];
    });
    expect(mocks.clearDraft).not.toHaveBeenCalled();
    expect(mocks.onOpenChange).not.toHaveBeenCalled();
    expect(mocks.push).not.toHaveBeenCalled();
    expect(mocks.onSuccess).not.toHaveBeenCalled();
    expect(result.current.recovery.receipt?.state).toBe("accepted");
  });

  it("keeps a description edit open with Open task", async () => {
    const result = await submitWithNewerDraft(() => {
      currentDescription = "Newer description";
    });
    expect(mocks.clearDraft).not.toHaveBeenCalled();
    expect(mocks.onOpenChange).not.toHaveBeenCalled();
    expect(mocks.push).not.toHaveBeenCalled();
    expect(mocks.onSuccess).not.toHaveBeenCalled();
    expect(result.current.recovery.receipt?.state).toBe("accepted");
  });
});

// @covers AC-TASKS-CREATE-RECOVERY-001.6, AC-TASKS-CREATE-RECOVERY-001.14
describe("stale branch policy recovery", () => {
  function stalePolicyError() {
    return new ApiError("branch policy is stale", 400, {
      error: "branch policy is stale",
      error_code: "branch_policy_stale",
      creation_rejected: true,
    });
  }

  it("refreshes policy options and retires a first-attempt stale rejection", async () => {
    const refresh = vi.fn(async () => undefined);
    mocks.createTask.mockRejectedValueOnce(stalePolicyError());
    const { result } = renderHook(() => useWired({ refreshBranchPolicies: refresh }));

    await act(async () => {
      await result.current.handlers.handleSubmit(SUBMIT_EVENT);
    });

    expect(refresh).toHaveBeenCalledTimes(1);
    expect(result.current.recovery.receipt).toBeNull();
    expect(mocks.toast).toHaveBeenCalled();
  });

  it("refreshes and keeps sticky uncertainty for a later stale rejection", async () => {
    const refresh = vi.fn(async () => undefined);
    mocks.createTask
      .mockRejectedValueOnce(new Error(LOST_RESPONSE))
      .mockRejectedValueOnce(stalePolicyError());
    const { result } = renderHook(() => useWired({ refreshBranchPolicies: refresh }));

    await act(async () => {
      await result.current.handlers.handleSubmit(SUBMIT_EVENT);
    });
    await act(async () => {
      await result.current.handlers.handleSubmit(SUBMIT_EVENT);
    });

    expect(refresh).toHaveBeenCalledTimes(1);
    expect(mocks.createTask).toHaveBeenCalledTimes(2);
    const first = mocks.createTask.mock.calls[0]![0] as { external_id: string };
    const second = mocks.createTask.mock.calls[1]![0] as { external_id: string };
    expect(second.external_id).toBe(first.external_id);
    expect(result.current.recovery.receipt?.state).toBe("unknown");
    expect(result.current.recovery.receipt?.everUncertain).toBe(true);
  });
});
