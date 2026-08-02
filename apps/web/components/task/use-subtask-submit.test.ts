import { act, renderHook } from "@testing-library/react";
import { useState } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { UtilityGenerationResult } from "@/hooks/use-utility-agent-generator";

const { mockCreateTask, mockReplaceTaskUrl, mockSetActiveTask, mockSetActiveSession } = vi.hoisted(
  () => ({
    mockCreateTask: vi.fn(),
    mockReplaceTaskUrl: vi.fn(),
    mockSetActiveTask: vi.fn(),
    mockSetActiveSession: vi.fn(),
  }),
);

const mockToast = vi.fn();
const mockEnhancePrompt = vi.fn();

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));

vi.mock("@/hooks/use-utility-agent-generator", () => ({
  useUtilityAgentGenerator: () => ({
    enhancePrompt: mockEnhancePrompt,
    isEnhancingPrompt: false,
  }),
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  createTask: mockCreateTask,
}));

vi.mock("@/lib/links", () => ({
  replaceTaskUrl: mockReplaceTaskUrl,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ setActiveTask: mockSetActiveTask, setActiveSession: mockSetActiveSession }),
}));

vi.mock("@/components/task-create-dialog-helpers", () => ({
  buildRepositoriesPayload: vi.fn(() => []),
  toMessageAttachments: vi.fn(() => []),
}));

import { useSubtaskPromptZone, useSubtaskSubmit } from "./use-subtask-submit";

const GENERATED_RESULT = {
  content: "improved prompt",
  callId: "call-123",
  durationMs: 1_200,
} satisfies UtilityGenerationResult;
const ORIGINAL_PROMPT = "original prompt";
const CREATED_TASK_ID = "created-task";
const CREATED_SESSION_ID = "created-session";

function useSubtaskPromptHarness(initialPrompt = ORIGINAL_PROMPT) {
  const [promptValue, setPromptValue] = useState(initialPrompt);
  const [hasPrompt, setHasPrompt] = useState(Boolean(initialPrompt.trim()));
  const promptZone = useSubtaskPromptZone({
    parentTaskId: "task-1",
    taskTitle: "Parent task",
    inputDisabled: false,
    contextValue: "blank",
    initialPrompt: null,
    promptValue,
    setPromptValue,
    setHasPrompt,
  });

  return {
    ...promptZone,
    promptValue,
    setPromptValue,
    hasPrompt,
  };
}

describe("useSubtaskPromptZone", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("applies an enhanced prompt immediately when the source text is unchanged", async () => {
    mockEnhancePrompt.mockImplementation(
      async (_source: string, deliver: (result: UtilityGenerationResult) => Promise<boolean>) =>
        deliver(GENERATED_RESULT),
    );

    const { result } = renderHook(() => useSubtaskPromptHarness());

    act(() => {
      result.current.promptRef.current = { value: ORIGINAL_PROMPT } as HTMLTextAreaElement;
    });

    await act(async () => {
      await result.current.handleEnhancePrompt();
    });

    expect(result.current.promptValue).toBe("improved prompt");
    expect(result.current.hasPrompt).toBe(true);
    expect(result.current.pendingResult).toBeNull();
    expect(mockToast).toHaveBeenCalledWith(
      expect.objectContaining({ description: "Enhanced prompt applied.", variant: "success" }),
    );
  });

  it("retains the enhanced prompt when the user changes the source text before delivery", async () => {
    let deliverResult: ((result: UtilityGenerationResult) => Promise<boolean>) | undefined;
    mockEnhancePrompt.mockImplementation(
      async (_source: string, deliver: (result: UtilityGenerationResult) => Promise<boolean>) => {
        deliverResult = deliver;
      },
    );

    const { result } = renderHook(() => useSubtaskPromptHarness());

    act(() => {
      result.current.promptRef.current = { value: ORIGINAL_PROMPT } as HTMLTextAreaElement;
    });

    await act(async () => {
      await result.current.handleEnhancePrompt();
    });

    act(() => {
      result.current.setPromptValue("edited prompt");
    });

    await act(async () => {
      await deliverResult?.(GENERATED_RESULT);
    });

    expect(result.current.promptValue).toBe("edited prompt");
    expect(result.current.pendingResult).toEqual(GENERATED_RESULT);

    act(() => {
      result.current.applyPending();
    });

    expect(result.current.promptValue).toBe("improved prompt");
    expect(result.current.pendingResult).toBeNull();
  });

  it("retains the enhanced prompt when the input target is unavailable", async () => {
    mockEnhancePrompt.mockImplementation(
      async (_source: string, deliver: (result: UtilityGenerationResult) => Promise<boolean>) =>
        deliver(GENERATED_RESULT),
    );

    const { result } = renderHook(() => useSubtaskPromptHarness());

    act(() => {
      result.current.promptRef.current = null;
    });

    await act(async () => {
      await result.current.handleEnhancePrompt();
    });

    expect(result.current.promptValue).toBe(ORIGINAL_PROMPT);
    expect(result.current.pendingResult).toEqual(GENERATED_RESULT);
  });

  it("preserves exact source text and retains the result after a whitespace-only edit", async () => {
    let deliverResult: ((result: UtilityGenerationResult) => Promise<boolean>) | undefined;
    mockEnhancePrompt.mockImplementation(
      async (_source: string, deliver: (result: UtilityGenerationResult) => Promise<boolean>) => {
        deliverResult = deliver;
      },
    );

    const initialPrompt = "  original prompt  ";
    const editedPrompt = "  original prompt   ";
    const { result } = renderHook(() => useSubtaskPromptHarness(initialPrompt));

    act(() => {
      result.current.promptRef.current = { value: initialPrompt } as HTMLTextAreaElement;
    });

    await act(async () => {
      await result.current.handleEnhancePrompt();
    });

    expect(mockEnhancePrompt).toHaveBeenCalledWith(initialPrompt, expect.any(Function));

    act(() => {
      result.current.setPromptValue(editedPrompt);
    });

    await act(async () => {
      await deliverResult?.(GENERATED_RESULT);
    });

    expect(result.current.promptValue).toBe(editedPrompt);
    expect(result.current.pendingResult).toEqual(GENERATED_RESULT);
  });

  it("resolves submission prompts from the same controlled prompt state", () => {
    const { result } = renderHook(() => useSubtaskPromptHarness());

    act(() => {
      result.current.setPromptValue("next prompt");
    });

    expect(result.current.resolvePrompt()).toBe("next prompt");
  });
});

function makeSubmitOptions(
  overrides: Partial<Parameters<typeof useSubtaskSubmit>[0]> = {},
): Parameters<typeof useSubtaskSubmit>[0] {
  return {
    fs: {
      useRemote: false,
      remoteRepos: [],
      prInfoByUrl: {},
      repositories: [],
      discoveredRepositories: [],
      agentProfileId: "",
      executorProfileId: "",
    } as unknown as Parameters<typeof useSubtaskSubmit>[0]["fs"],
    parentTaskId: "parent-task",
    defaultProfileId: "default-profile",
    workspaceId: "workspace-1",
    workflowId: "workflow-1",
    availableRepositories: [],
    attachments: [],
    resolvePrompt: () => "do the work",
    title: "Manual title",
    setIsCreating: vi.fn(),
    onClose: vi.fn(),
    workspaceMode: "new_workspace",
    ...overrides,
  };
}

describe("useSubtaskSubmit", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockCreateTask.mockResolvedValue({ id: CREATED_TASK_ID, session_id: CREATED_SESSION_ID });
  });

  it("sends the auto-title contract without a title", async () => {
    const opts = makeSubmitOptions({ autoTitle: true });
    const { result } = renderHook(() => useSubtaskSubmit(opts));

    await act(async () => {
      await result.current.handleSubmit({ preventDefault: vi.fn() } as never);
    });

    expect(mockCreateTask).toHaveBeenCalledWith(
      expect.objectContaining({ auto_title: true, description: "do the work" }),
    );
    expect(mockCreateTask.mock.calls[0][0]).not.toHaveProperty("title");
    expect(opts.onClose).toHaveBeenCalledOnce();
    expect(mockSetActiveTask).toHaveBeenCalledWith(CREATED_TASK_ID);
    expect(mockSetActiveSession).toHaveBeenCalledWith(CREATED_TASK_ID, CREATED_SESSION_ID);
    expect(mockReplaceTaskUrl).toHaveBeenCalledWith(CREATED_TASK_ID);
  });

  it("requires a title when auto-title mode is omitted", async () => {
    const opts = makeSubmitOptions({ title: "" });
    const { result } = renderHook(() => useSubtaskSubmit(opts));

    await act(async () => {
      await result.current.handleSubmit({ preventDefault: vi.fn() } as never);
    });

    expect(mockCreateTask).not.toHaveBeenCalled();
  });

  it("preserves the legacy title payload", async () => {
    const opts = makeSubmitOptions({ title: "Manual title" });
    const { result } = renderHook(() => useSubtaskSubmit(opts));

    await act(async () => {
      await result.current.handleSubmit({ preventDefault: vi.fn() } as never);
    });

    expect(mockCreateTask).toHaveBeenCalledWith(
      expect.objectContaining({ title: "Manual title", description: "do the work" }),
    );
    expect(mockCreateTask.mock.calls[0][0]).not.toHaveProperty("auto_title");
  });

  it("rejects an empty prompt before creating", async () => {
    const opts = makeSubmitOptions({ resolvePrompt: () => "  " });
    const { result } = renderHook(() => useSubtaskSubmit(opts));

    await act(async () => {
      await result.current.handleSubmit({ preventDefault: vi.fn() } as never);
    });

    expect(mockCreateTask).not.toHaveBeenCalled();
  });

  it("cleans up the creating state after a request failure", async () => {
    const opts = makeSubmitOptions();
    const error = new Error("request failed");
    mockCreateTask.mockRejectedValueOnce(error);
    const { result } = renderHook(() => useSubtaskSubmit(opts));

    await act(async () => {
      await result.current.handleSubmit({ preventDefault: vi.fn() } as never);
    });

    expect(opts.setIsCreating).toHaveBeenNthCalledWith(1, true);
    expect(opts.setIsCreating).toHaveBeenLastCalledWith(false);
    expect(opts.onClose).not.toHaveBeenCalled();
    expect(mockToast).toHaveBeenCalledWith(expect.objectContaining({ variant: "error" }));
  });
});
