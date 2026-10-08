import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { useProjectFormData } from "./agent-project-form-fields";
import type { AgentProject } from "@/lib/types/http-agent-projects";
import type { RemoteRepository } from "@/hooks/domains/integrations/use-remote-repositories";
import type { Repository } from "@/lib/types/http";
import { ApiError } from "@/lib/api/client";
import { projectRemoteSelectionId } from "@/lib/agent-projects/repository-resolution";

const mocks = vi.hoisted(() => ({
  create: vi.fn(),
  update: vi.fn(),
  ensureTaskSession: vi.fn(),
  sendMessageRequest: vi.fn(),
  generateUUID: vi.fn(),
}));

vi.mock("@/hooks/domains/agent-projects/use-agent-projects", () => ({
  useAgentProjectMutations: () => ({ create: mocks.create, update: mocks.update }),
}));
vi.mock("@/lib/utils", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/utils")>()),
  generateUUID: mocks.generateUUID,
}));
vi.mock("@/lib/services/session-launch-service", () => ({
  ensureTaskSession: mocks.ensureTaskSession,
}));
vi.mock("@/hooks/message-request", () => ({ sendMessageRequest: mocks.sendMessageRequest }));

import { useAgentProjectFormController } from "./agent-project-form-controller";

const formData = {
  repositories: [{ id: "repo-1" }],
  profiles: [{ id: "coordinator" }, { id: "economy" }, { id: "frontier" }],
  executorReady: true,
  repositoriesLoaded: true,
} as ReturnType<typeof useProjectFormData>;

const matchingRemoteRepository: RemoteRepository = {
  provider: "github",
  id: "acme/runtime",
  owner: "acme",
  name: "runtime",
  fullName: "acme/runtime",
  url: "https://github.com/acme/runtime.git",
  providerHost: "https://github.com",
  private: true,
  defaultBranch: "stable",
};

const FIRST_CREATE_REQUEST_KEY = "request-key-1";
const LEGACY_REPOSITORY_ID = "repository-runtime";

function formDataWithLegacyRepository() {
  return {
    ...formData,
    repositories: [
      {
        id: LEGACY_REPOSITORY_ID,
        workspace_id: "ws-1",
        source_type: "provider",
        provider: "github",
        provider_repo_id: "",
        provider_host: "https://github.com",
        provider_scope: "",
        provider_owner: "acme",
        provider_name: "runtime",
        remote_url: "https://github.com/acme/runtime.git",
        default_branch: "stable",
      } as Repository,
    ],
  } as ReturnType<typeof useProjectFormData>;
}

beforeEach(() => {
  mocks.create.mockReset();
  mocks.update.mockReset();
  mocks.ensureTaskSession.mockReset();
  mocks.sendMessageRequest.mockReset();
  mocks.generateUUID.mockReset();
  mocks.generateUUID.mockReturnValueOnce(FIRST_CREATE_REQUEST_KEY).mockReturnValue("request-key-2");
});

type ControllerTestOptions = {
  open?: boolean;
  workspaceId?: string;
  project?: AgentProject;
  formData?: ReturnType<typeof useProjectFormData>;
  onOpenChange?: (open: boolean) => void;
  onOpenCoordinator?: (taskId: string) => void;
};

function useController(options: ControllerTestOptions = {}) {
  return useAgentProjectFormController({
    open: options.open ?? true,
    workspaceId: options.workspaceId ?? "ws-1",
    project: options.project,
    formData: options.formData ?? formData,
    onOpenChange: options.onOpenChange ?? vi.fn(),
    onOpenCoordinator: options.onOpenCoordinator,
  });
}

describe("Agent Project repository selection", () => {
  it("coalesces a matching provider row when a configured repository is selected first", () => {
    const { result } = renderHook(() =>
      useController({ formData: formDataWithLegacyRepository() }),
    );

    act(() => result.current.toggleRepository(LEGACY_REPOSITORY_ID));
    act(() => result.current.selectRemoteRepository(matchingRemoteRepository));

    expect(result.current.draft.repositoryIds).toEqual([LEGACY_REPOSITORY_ID]);
    expect(result.current.draft.remoteRepositories).toEqual([]);
  });

  it("replaces a matching pending provider row when its configured repository is selected", () => {
    const { result } = renderHook(() =>
      useController({ formData: formDataWithLegacyRepository() }),
    );

    act(() => result.current.selectRemoteRepository(matchingRemoteRepository));
    expect(result.current.draft.repositoryIds).toEqual([
      projectRemoteSelectionId(matchingRemoteRepository),
    ]);
    act(() => result.current.toggleRepository(LEGACY_REPOSITORY_ID));

    expect(result.current.draft.repositoryIds).toEqual([LEGACY_REPOSITORY_ID]);
    expect(result.current.draft.remoteRepositories).toEqual([]);
    expect(result.current.draft.primaryRepositoryId).toBe(LEGACY_REPOSITORY_ID);
  });
});

describe("Agent Project project persistence", () => {
  it("reuses the same create request key when retrying a draft after an error", async () => {
    mocks.create.mockRejectedValueOnce(new Error("response lost")).mockResolvedValue({ id: "p1" });
    const onOpenChange = vi.fn();
    const { result } = renderHook(() => useController({ onOpenChange }));
    act(() => {
      result.current.updateDraft({
        name: "Release migration",
        repositoryIds: ["repo-1"],
        primaryRepositoryId: "repo-1",
        coordinatorProfileId: "coordinator",
        economyProfileId: "economy",
        frontierProfileId: "frontier",
      });
    });

    const event = { preventDefault: vi.fn() } as never;
    await act(async () => result.current.submit(event));
    await act(async () => result.current.submit(event));

    expect(mocks.create).toHaveBeenCalledTimes(2);
    expect(mocks.create.mock.calls[0][1].requestKey).toBe(FIRST_CREATE_REQUEST_KEY);
    expect(mocks.create.mock.calls[1][1].requestKey).toBe(FIRST_CREATE_REQUEST_KEY);
    expect(mocks.generateUUID).toHaveBeenCalledTimes(1);
  });

  it("keeps the edited draft and exposes a stale-revision failure", async () => {
    mocks.update.mockRejectedValue(new Error("project was changed by another request"));
    const project = {
      id: "project-1",
      name: "Original name",
      revision: 1,
      repository_ids: ["repo-1"],
      primary_repository_id: "repo-1",
      coordinator_profile_id: "coordinator",
      economy_profile_id: "economy",
      frontier_profile_id: "frontier",
    } as AgentProject;
    const onOpenChange = vi.fn();
    const { result } = renderHook(() => useController({ project, onOpenChange }));
    act(() => result.current.updateDraft({ name: "Keep this edit" }));
    await act(async () => result.current.submit({ preventDefault: vi.fn() } as never));
    expect(result.current.draft.name).toBe("Keep this edit");
    expect(result.current.error).toBe("project was changed by another request");
    expect(result.current.saving).toBe(false);
    expect(onOpenChange).not.toHaveBeenCalled();
  });

  it("creates an idle project without ensuring or messaging a coordinator", async () => {
    const saved = { id: "project-1", main_task_id: "task-1" } as AgentProject;
    mocks.create.mockResolvedValue(saved);
    const onOpenChange = vi.fn();
    const onOpenCoordinator = vi.fn();
    const { result } = renderHook(() => useController({ onOpenChange, onOpenCoordinator }));
    act(() => {
      result.current.updateDraft({
        name: "Idle project",
        repositoryIds: ["repo-1"],
        primaryRepositoryId: "repo-1",
        coordinatorProfileId: "coordinator",
        economyProfileId: "economy",
        frontierProfileId: "frontier",
      });
    });

    await act(async () => result.current.submit({ preventDefault: vi.fn() } as never));

    expect(mocks.create).toHaveBeenCalledOnce();
    expect(mocks.ensureTaskSession).not.toHaveBeenCalled();
    expect(mocks.sendMessageRequest).not.toHaveBeenCalled();
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(onOpenCoordinator).not.toHaveBeenCalled();
  });
});

describe("Agent Project initial coordinator prompts", () => {
  it("sends the initial prompt once through the ensured coordinator session", async () => {
    const saved = { id: "project-1", main_task_id: "task-1" } as AgentProject;
    mocks.create.mockResolvedValue(saved);
    mocks.ensureTaskSession.mockResolvedValue({ success: true, session_id: "session-1" });
    mocks.sendMessageRequest.mockResolvedValue({ id: "message-1" });
    const onOpenChange = vi.fn();
    const onOpenCoordinator = vi.fn();
    const { result } = renderHook(() => useController({ onOpenChange, onOpenCoordinator }));
    act(() => {
      result.current.updateDraft({
        name: "Started project",
        initialPrompt: "Review the current API boundaries.",
        repositoryIds: ["repo-1"],
        primaryRepositoryId: "repo-1",
        coordinatorProfileId: "coordinator",
        economyProfileId: "economy",
        frontierProfileId: "frontier",
      });
    });

    await act(async () => result.current.submit({ preventDefault: vi.fn() } as never));

    expect(mocks.create).toHaveBeenCalledOnce();
    expect(mocks.ensureTaskSession).toHaveBeenCalledWith("task-1", expect.any(Object));
    expect(mocks.sendMessageRequest).toHaveBeenCalledWith(
      expect.objectContaining({
        taskId: "task-1",
        resolvedSessionId: "session-1",
        finalMessage: "Review the current API boundaries.",
        planMode: false,
        clientMessageId: expect.any(String),
      }),
    );
    expect(mocks.create.mock.invocationCallOrder[0]).toBeLessThan(
      mocks.ensureTaskSession.mock.invocationCallOrder[0],
    );
    expect(mocks.ensureTaskSession.mock.invocationCallOrder[0]).toBeLessThan(
      mocks.sendMessageRequest.mock.invocationCallOrder[0],
    );
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(onOpenCoordinator).toHaveBeenCalledWith("task-1");
  });

  it("guards duplicate submissions synchronously before the project request settles", async () => {
    let finishCreate!: (project: AgentProject) => void;
    mocks.create.mockImplementation(
      () => new Promise<AgentProject>((resolve) => (finishCreate = resolve)),
    );
    const { result } = renderHook(() => useController());
    act(() => {
      result.current.updateDraft({
        name: "No duplicate",
        repositoryIds: ["repo-1"],
        primaryRepositoryId: "repo-1",
        coordinatorProfileId: "coordinator",
        economyProfileId: "economy",
        frontierProfileId: "frontier",
      });
    });

    let first!: Promise<void>;
    let second!: Promise<void>;
    act(() => {
      first = result.current.submit({ preventDefault: vi.fn() } as never);
      second = result.current.submit({ preventDefault: vi.fn() } as never);
    });
    expect(mocks.create).toHaveBeenCalledOnce();
    await act(async () => {
      finishCreate({ id: "project-1" } as AgentProject);
      await Promise.all([first, second]);
    });
  });
});

describe("Agent Project prompt retry", () => {
  it("keeps a failed prompt on the saved project and retries with the same message identity", async () => {
    const saved = { id: "project-1", main_task_id: "task-1" } as AgentProject;
    mocks.create.mockResolvedValue(saved);
    mocks.ensureTaskSession.mockResolvedValue({ success: true, session_id: "session-1" });
    mocks.sendMessageRequest
      .mockRejectedValueOnce(new Error("websocket request timed out"))
      .mockResolvedValueOnce({ id: "message-1" });
    const onOpenChange = vi.fn();
    const onOpenCoordinator = vi.fn();
    const { result } = renderHook(() => useController({ onOpenChange, onOpenCoordinator }));
    act(() => {
      result.current.updateDraft({
        name: "Started project",
        initialPrompt: "Keep this exact prompt.",
        repositoryIds: ["repo-1"],
        primaryRepositoryId: "repo-1",
        coordinatorProfileId: "coordinator",
        economyProfileId: "economy",
        frontierProfileId: "frontier",
      });
    });

    await act(async () => result.current.submit({ preventDefault: vi.fn() } as never));
    expect(result.current.createdProject).toEqual(saved);
    expect(result.current.draft.initialPrompt).toBe("Keep this exact prompt.");
    expect(result.current.promptLocked).toBe(true);
    expect(onOpenChange).not.toHaveBeenCalled();

    const firstPayload = mocks.sendMessageRequest.mock.calls[0]![0];
    act(() => result.current.updateDraft({ initialPrompt: "Changed prompt" }));
    expect(result.current.draft.initialPrompt).toBe("Keep this exact prompt.");
    await act(async () => result.current.retryStart());

    expect(mocks.create).toHaveBeenCalledOnce();
    expect(mocks.ensureTaskSession).toHaveBeenCalledOnce();
    expect(mocks.sendMessageRequest).toHaveBeenCalledTimes(2);
    expect(mocks.sendMessageRequest.mock.calls[1]![0]).toEqual(firstPayload);
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(onOpenCoordinator).toHaveBeenCalledWith("task-1");
  });
});

describe("Agent Project stale submission guards", () => {
  it("does not continue an old submission after the dialog closes", async () => {
    let finishCreate!: (project: AgentProject) => void;
    mocks.create.mockImplementation(
      () => new Promise<AgentProject>((resolve) => (finishCreate = resolve)),
    );
    const onOpenChange = vi.fn();
    const { result, rerender } = renderHook(
      ({ open, workspaceId }: { open: boolean; workspaceId: string }) =>
        useController({ open, workspaceId, onOpenChange }),
      { initialProps: { open: true, workspaceId: "ws-1" } },
    );
    act(() => {
      result.current.updateDraft({
        name: "Late response",
        initialPrompt: "Do not send after dismissal.",
        repositoryIds: ["repo-1"],
        primaryRepositoryId: "repo-1",
        coordinatorProfileId: "coordinator",
        economyProfileId: "economy",
        frontierProfileId: "frontier",
      });
    });
    let pending!: Promise<void>;
    act(() => {
      pending = result.current.submit({ preventDefault: vi.fn() } as never);
    });
    rerender({ open: false, workspaceId: "ws-1" });
    await act(async () => {
      finishCreate({ id: "project-1", main_task_id: "task-1" } as AgentProject);
      await pending;
    });

    expect(mocks.ensureTaskSession).not.toHaveBeenCalled();
    expect(mocks.sendMessageRequest).not.toHaveBeenCalled();
    expect(onOpenChange).not.toHaveBeenCalled();
  });

  it("does not continue a pending prompt after workspace switch and reopen", async () => {
    let finishCreate!: (project: AgentProject) => void;
    mocks.create.mockImplementation(
      () => new Promise<AgentProject>((resolve) => (finishCreate = resolve)),
    );
    const onOpenChange = vi.fn();
    const onOpenCoordinator = vi.fn();
    const { result, rerender } = renderHook(
      ({ open, workspaceId }: { open: boolean; workspaceId: string }) =>
        useController({ open, workspaceId, onOpenChange, onOpenCoordinator }),
      { initialProps: { open: true, workspaceId: "ws-1" } },
    );
    act(() => {
      result.current.updateDraft({
        name: "Workspace switch",
        initialPrompt: "Do not send in a different workspace.",
        repositoryIds: ["repo-1"],
        primaryRepositoryId: "repo-1",
        coordinatorProfileId: "coordinator",
        economyProfileId: "economy",
        frontierProfileId: "frontier",
      });
    });
    let pending!: Promise<void>;
    act(() => {
      pending = result.current.submit({ preventDefault: vi.fn() } as never);
    });
    rerender({ open: false, workspaceId: "ws-1" });
    rerender({ open: true, workspaceId: "ws-2" });
    await act(async () => {
      finishCreate({ id: "project-1", main_task_id: "task-1" } as AgentProject);
      await pending;
    });

    expect(mocks.ensureTaskSession).not.toHaveBeenCalled();
    expect(mocks.sendMessageRequest).not.toHaveBeenCalled();
    expect(onOpenChange).not.toHaveBeenCalled();
    expect(onOpenCoordinator).not.toHaveBeenCalled();
  });
});

describe("Agent Project create retry identity", () => {
  it("retries an uncertain create with its original request payload", async () => {
    const saved = { id: "project-1" } as AgentProject;
    mocks.create.mockRejectedValueOnce(new Error("response lost")).mockResolvedValue(saved);
    const { result } = renderHook(() => useController());
    act(() => {
      result.current.updateDraft({
        name: "Keep this create payload",
        repositoryIds: ["repo-1"],
        primaryRepositoryId: "repo-1",
        coordinatorProfileId: "coordinator",
        economyProfileId: "economy",
        frontierProfileId: "frontier",
      });
    });
    await act(async () => result.current.submit({ preventDefault: vi.fn() } as never));
    act(() => result.current.updateDraft({ name: "Do not change the unresolved request" }));
    expect(result.current.draft.name).toBe("Keep this create payload");
    await act(async () => result.current.submit({ preventDefault: vi.fn() } as never));

    expect(mocks.create).toHaveBeenCalledTimes(2);
    expect(mocks.create.mock.calls[1]![1]).toEqual(mocks.create.mock.calls[0]![1]);
  });

  it("allows correction after a validation rejection without changing the request key", async () => {
    mocks.create
      .mockRejectedValueOnce(new ApiError("invalid project settings", 400, null))
      .mockResolvedValue({ id: "project-1" } as AgentProject);
    const { result } = renderHook(() => useController());
    act(() => {
      result.current.updateDraft({
        name: "Correct this project",
        repositoryIds: ["repo-1"],
        primaryRepositoryId: "repo-1",
        coordinatorProfileId: "coordinator",
        economyProfileId: "economy",
        frontierProfileId: "frontier",
      });
    });
    await act(async () => result.current.submit({ preventDefault: vi.fn() } as never));
    act(() => result.current.updateDraft({ name: "Corrected project" }));
    expect(result.current.draft.name).toBe("Corrected project");
    await act(async () => result.current.submit({ preventDefault: vi.fn() } as never));

    expect(mocks.create).toHaveBeenCalledTimes(2);
    expect(mocks.create.mock.calls[0]![1].requestKey).toBe(
      mocks.create.mock.calls[1]![1].requestKey,
    );
    expect(mocks.create.mock.calls[1]![1].name).toBe("Corrected project");
  });
});

describe("Agent Project profile errors and recovery", () => {
  it("allows correcting a profile rejected as unsupported and localizes the reason", async () => {
    mocks.create
      .mockRejectedValueOnce(
        new ApiError(
          "agent profile does not support project workspace access: OpenCode",
          422,
          null,
        ),
      )
      .mockResolvedValue({ id: "project-1" } as AgentProject);
    const { result } = renderHook(() => useController());
    act(() => {
      result.current.updateDraft({
        name: "Correct this profile",
        repositoryIds: ["repo-1"],
        primaryRepositoryId: "repo-1",
        coordinatorProfileId: "coordinator",
        economyProfileId: "economy",
        frontierProfileId: "frontier",
      });
    });
    await act(async () => result.current.submit({ preventDefault: vi.fn() } as never));
    expect(result.current.error).not.toContain("agent profile does not support");
    act(() => result.current.updateDraft({ coordinatorProfileId: "frontier" }));
    await act(async () => result.current.submit({ preventDefault: vi.fn() } as never));

    expect(mocks.create).toHaveBeenCalledTimes(2);
    expect(mocks.create.mock.calls[0]![1].requestKey).toBe(
      mocks.create.mock.calls[1]![1].requestKey,
    );
    expect(mocks.create.mock.calls[1]![1].coordinatorProfileId).toBe("frontier");
  });

  it("does not let an old validation rejection unlock a newer uncertain create", async () => {
    let rejectOld!: (error: Error) => void;
    let rejectNew!: (error: Error) => void;
    mocks.create
      .mockImplementationOnce(
        () => new Promise<AgentProject>((_resolve, reject) => (rejectOld = reject)),
      )
      .mockImplementationOnce(
        () => new Promise<AgentProject>((_resolve, reject) => (rejectNew = reject)),
      );
    const { result, rerender } = renderHook(
      ({ open }: { open: boolean }) => useController({ open }),
      { initialProps: { open: true } },
    );
    act(() => {
      result.current.updateDraft({
        name: "Old project",
        repositoryIds: ["repo-1"],
        primaryRepositoryId: "repo-1",
        coordinatorProfileId: "coordinator",
        economyProfileId: "economy",
        frontierProfileId: "frontier",
      });
    });
    let oldSubmission!: Promise<void>;
    act(() => {
      oldSubmission = result.current.submit({ preventDefault: vi.fn() } as never);
    });
    rerender({ open: false });
    rerender({ open: true });
    act(() => {
      result.current.updateDraft({
        name: "New project",
        repositoryIds: ["repo-1"],
        primaryRepositoryId: "repo-1",
        coordinatorProfileId: "coordinator",
        economyProfileId: "economy",
        frontierProfileId: "frontier",
      });
    });
    let newSubmission!: Promise<void>;
    act(() => {
      newSubmission = result.current.submit({ preventDefault: vi.fn() } as never);
    });
    await act(async () => {
      rejectOld(new ApiError("old validation rejection", 400, null));
      await oldSubmission;
    });
    await act(async () => {
      rejectNew(new Error("response lost"));
      await newSubmission;
    });
    act(() => result.current.updateDraft({ name: "Do not change new request" }));

    expect(result.current.draft.name).toBe("New project");
  });
});

describe("Agent Project coordinator and worker profiles", () => {
  it("does not send when the ensured coordinator has no session", async () => {
    mocks.create.mockResolvedValue({ id: "project-1", main_task_id: "task-1" } as AgentProject);
    mocks.ensureTaskSession.mockResolvedValue({ success: false });
    const { result } = renderHook(() => useController());
    act(() => {
      result.current.updateDraft({
        name: "No session",
        initialPrompt: "Do not send without a session.",
        repositoryIds: ["repo-1"],
        primaryRepositoryId: "repo-1",
        coordinatorProfileId: "coordinator",
        economyProfileId: "economy",
        frontierProfileId: "frontier",
      });
    });
    await act(async () => result.current.submit({ preventDefault: vi.fn() } as never));

    expect(mocks.sendMessageRequest).not.toHaveBeenCalled();
    expect(result.current.createdProject?.id).toBe("project-1");
    expect(result.current.error).toContain("session");
  });

  it("lets inherited worker profiles follow the coordinator until explicitly overridden", () => {
    const { result } = renderHook(() => useController());
    act(() => result.current.updateDraft({ coordinatorProfileId: "coordinator" }));
    expect(result.current.draft.economyProfileInherited).toBe(true);
    expect(result.current.effectiveEconomyProfileId).toBe("coordinator");
    expect(result.current.draft.frontierProfileInherited).toBe(true);

    act(() =>
      result.current.updateDraft({
        economyProfileId: "coordinator",
        economyProfileInherited: false,
        coordinatorProfileId: "frontier",
      }),
    );
    expect(result.current.effectiveEconomyProfileId).toBe("coordinator");
    expect(result.current.effectiveFrontierProfileId).toBe("frontier");
  });

  it("keeps edit profile choices explicit when the coordinator changes", () => {
    const project = {
      id: "project-1",
      name: "Existing",
      revision: 1,
      repository_ids: ["repo-1"],
      primary_repository_id: "repo-1",
      coordinator_profile_id: "coordinator",
      economy_profile_id: "economy",
      frontier_profile_id: "frontier",
    } as AgentProject;
    const { result } = renderHook(() => useController({ project }));
    act(() => result.current.updateDraft({ coordinatorProfileId: "new-coordinator" }));
    expect(result.current.draft.economyProfileInherited).toBe(false);
    expect(result.current.effectiveEconomyProfileId).toBe("economy");
    expect(result.current.effectiveFrontierProfileId).toBe("frontier");
  });
});
