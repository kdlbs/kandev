import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { FormEvent } from "react";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { FileAttachment } from "./chat/file-attachment";

const mockLaunchSession = vi.fn();
const mockBuildStartRequest = vi.fn();
const mockToMessageAttachments = vi.fn();

vi.mock("@/lib/services/session-launch-service", () => ({
  launchSession: (...args: Parameters<typeof mockLaunchSession>) => mockLaunchSession(...args),
}));

vi.mock("@/lib/services/session-launch-helpers", () => ({
  buildStartRequest: (...args: Parameters<typeof mockBuildStartRequest>) =>
    mockBuildStartRequest(...args),
}));

vi.mock("@/components/task-create-dialog-helpers", () => ({
  toMessageAttachments: (...args: Parameters<typeof mockToMessageAttachments>) =>
    mockToMessageAttachments(...args),
}));

import { useSessionContextChange, useSessionLaunchSubmit } from "./new-session-form-actions";

const mockToast = vi.fn();

const AGENT_PROFILE_A: AgentProfileOption = {
  id: "profile-a",
  label: "Profile A",
  agent_name: "agent-a",
  agent_id: "agent-id",
  cli_passthrough: false,
};

const ATTACHMENT: FileAttachment = {
  id: "attachment-1",
  data: "dGVzdA==",
  mimeType: "text/plain",
  fileName: "notes.txt",
  size: 42,
  isImage: false,
  deliveryMode: "path",
};
const TASK_ID = "task-1";
const PROFILE_ID = "profile-a";
const EXECUTOR_ID = "executor-1";
const SESSION_ID = "session-2";
const GROUP_ID = "group-1";
const SEED_PROMPT = "seed prompt";
const MESSAGE_ATTACHMENTS = ["message-attachment"];
const SUMMARY_ACTION = "summarize:session-1";
const SUMMARY_SESSION_ID = "session-1";

// eslint-disable-next-line max-lines-per-function
describe("useSessionContextChange", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("copies the initial prompt when copy_prompt is selected", async () => {
    const promptRef = { current: { value: "original" } as unknown as HTMLTextAreaElement };
    const setContextValue = vi.fn();
    const setHasPrompt = vi.fn();
    const { result } = renderHook(() =>
      useSessionContextChange({
        promptRef,
        initialPrompt: "starter",
        summarize: vi.fn(),
        toast: mockToast,
        setContextValue,
        setHasPrompt,
      }),
    );

    await act(async () => {
      await result.current("copy_prompt");
    });

    expect(promptRef.current.value).toBe("starter");
    expect(setContextValue).toHaveBeenCalledWith("copy_prompt");
    expect(setHasPrompt).toHaveBeenCalledWith(true);
    expect(mockToast).not.toHaveBeenCalled();
  });

  it("clears the prompt for blank", async () => {
    const promptRef = { current: { value: "original" } as unknown as HTMLTextAreaElement };
    const setContextValue = vi.fn();
    const setHasPrompt = vi.fn();
    const { result } = renderHook(() =>
      useSessionContextChange({
        promptRef,
        initialPrompt: null,
        summarize: vi.fn(),
        toast: mockToast,
        setContextValue,
        setHasPrompt,
      }),
    );

    await act(async () => {
      await result.current("blank");
    });

    expect(promptRef.current.value).toBe("");
    expect(setContextValue).toHaveBeenCalledWith("blank");
    expect(setHasPrompt).toHaveBeenCalledWith(false);
  });

  it("summarizes sessions into prompt text", async () => {
    const promptRef = { current: { value: "" } as unknown as HTMLTextAreaElement };
    const summarize = vi.fn().mockResolvedValue({ summary: "summary text" });
    const setContextValue = vi.fn();
    const setHasPrompt = vi.fn();
    const { result } = renderHook(() =>
      useSessionContextChange({
        promptRef,
        initialPrompt: null,
        summarize,
        toast: mockToast,
        setContextValue,
        setHasPrompt,
      }),
    );

    await act(async () => {
      await result.current(SUMMARY_ACTION);
    });

    expect(summarize).toHaveBeenCalledWith(SUMMARY_SESSION_ID);
    expect(promptRef.current.value).toBe("summary text");
    expect(setHasPrompt).toHaveBeenCalledWith(true);
    expect(mockToast).not.toHaveBeenCalled();
  });

  it("handles summarize failures by clearing context and showing toast", async () => {
    const promptRef = { current: { value: "" } as unknown as HTMLTextAreaElement };
    const summarize = vi.fn().mockResolvedValue({ summary: null, error: "connection refused" });
    const setContextValue = vi.fn();
    const setHasPrompt = vi.fn();
    const { result } = renderHook(() =>
      useSessionContextChange({
        promptRef,
        initialPrompt: null,
        summarize,
        toast: mockToast,
        setContextValue,
        setHasPrompt,
      }),
    );

    await act(async () => {
      await result.current(SUMMARY_ACTION);
    });

    expect(setContextValue).toHaveBeenCalledWith("blank");
    expect(mockToast).toHaveBeenCalledWith({
      title: "Summarize failed",
      description: "connection refused",
      variant: "error",
    });
    expect(setHasPrompt).not.toHaveBeenCalled();
  });

  it("does not toast when summarize succeeds after prompt ref unmounts", async () => {
    const promptRef = { current: null as HTMLTextAreaElement | null };
    const summarize = vi.fn().mockResolvedValue({ summary: "summary text" });
    const setContextValue = vi.fn();
    const setHasPrompt = vi.fn();
    const { result } = renderHook(() =>
      useSessionContextChange({
        promptRef,
        initialPrompt: null,
        summarize,
        toast: mockToast,
        setContextValue,
        setHasPrompt,
      }),
    );

    await act(async () => {
      await result.current(SUMMARY_ACTION);
    });

    expect(summarize).toHaveBeenCalledWith(SUMMARY_SESSION_ID);
    expect(setContextValue).toHaveBeenCalledWith(SUMMARY_ACTION);
    expect(setHasPrompt).not.toHaveBeenCalled();
    expect(mockToast).not.toHaveBeenCalled();
  });

  it("sanitizes unsafe characters from summarize result before setting prompt", async () => {
    const promptRef = { current: { value: "" } as unknown as HTMLTextAreaElement };
    const summarize = vi.fn().mockResolvedValue({ summary: "line1\r\n<unsafe>\nline2" });
    const setContextValue = vi.fn();
    const setHasPrompt = vi.fn();
    const { result } = renderHook(() =>
      useSessionContextChange({
        promptRef,
        initialPrompt: null,
        summarize,
        toast: mockToast,
        setContextValue,
        setHasPrompt,
      }),
    );

    await act(async () => {
      await result.current(SUMMARY_ACTION);
    });

    expect(promptRef.current.value).toBe("line1\n unsafe \nline2");
    expect(setHasPrompt).toHaveBeenCalledWith(true);
  });
});

// eslint-disable-next-line max-lines-per-function
describe("useSessionLaunchSubmit", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockBuildStartRequest.mockReturnValue({
      request: {
        task_id: TASK_ID,
        profile_id: PROFILE_ID,
        executor_id: EXECUTOR_ID,
      },
    });
    mockToMessageAttachments.mockReturnValue(MESSAGE_ATTACHMENTS);
    mockLaunchSession.mockResolvedValue({ session_id: SESSION_ID });
  });

  it("creates a session and activates it with the typed prompt", async () => {
    const promptRef = { current: { value: "  hello " } as unknown as HTMLTextAreaElement };
    const mockSetActiveSession = vi.fn();
    const mockActivateSession = vi.fn();
    const mockSetIsCreating = vi.fn();
    const mockOnClose = vi.fn();

    const { result } = renderHook(() =>
      useSessionLaunchSubmit({
        promptRef,
        taskId: TASK_ID,
        selectedProfileId: PROFILE_ID,
        executorId: EXECUTOR_ID,
        contextValue: "blank",
        initialPrompt: null,
        agentProfiles: [AGENT_PROFILE_A],
        attachments: [ATTACHMENT],
        groupId: GROUP_ID,
        onClose: mockOnClose,
        toast: mockToast,
        setActiveSession: mockSetActiveSession,
        activateSession: mockActivateSession,
        setIsCreating: mockSetIsCreating,
      }),
    );
    const event = {
      preventDefault: vi.fn(),
    } as unknown as FormEvent;

    await act(async () => {
      await result.current(event);
    });

    expect(mockBuildStartRequest).toHaveBeenCalledWith(TASK_ID, PROFILE_ID, {
      executorId: EXECUTOR_ID,
      prompt: "hello",
      attachments: MESSAGE_ATTACHMENTS,
    });
    expect(mockLaunchSession).toHaveBeenCalledWith({
      task_id: TASK_ID,
      profile_id: PROFILE_ID,
      executor_id: EXECUTOR_ID,
    });
    expect(mockActivateSession).toHaveBeenCalledWith(
      SESSION_ID,
      TASK_ID,
      "Profile A",
      GROUP_ID,
      mockSetActiveSession,
    );
    expect(mockOnClose).toHaveBeenCalled();
    expect(mockSetIsCreating).toHaveBeenNthCalledWith(1, true);
    expect(mockSetIsCreating).toHaveBeenLastCalledWith(false);
  });

  it("uses initial prompt when context is copy_prompt and user did not type anything", async () => {
    const promptRef = { current: { value: "   " } as unknown as HTMLTextAreaElement };
    const mockSetActiveSession = vi.fn();
    const mockActivateSession = vi.fn();
    const mockSetIsCreating = vi.fn();
    const mockOnClose = vi.fn();

    const { result } = renderHook(() =>
      useSessionLaunchSubmit({
        promptRef,
        taskId: TASK_ID,
        selectedProfileId: PROFILE_ID,
        executorId: EXECUTOR_ID,
        contextValue: "copy_prompt",
        initialPrompt: SEED_PROMPT,
        agentProfiles: [AGENT_PROFILE_A],
        attachments: [ATTACHMENT],
        onClose: mockOnClose,
        toast: mockToast,
        setActiveSession: mockSetActiveSession,
        activateSession: mockActivateSession,
        setIsCreating: mockSetIsCreating,
      }),
    );
    const event = {
      preventDefault: vi.fn(),
    } as unknown as FormEvent;

    await act(async () => {
      await result.current(event);
    });

    expect(mockBuildStartRequest).toHaveBeenCalledWith(
      TASK_ID,
      PROFILE_ID,
      expect.objectContaining({ prompt: SEED_PROMPT }),
    );
    expect(mockActivateSession).toHaveBeenCalledWith(
      SESSION_ID,
      TASK_ID,
      "Profile A",
      undefined,
      mockSetActiveSession,
    );
    expect(mockOnClose).toHaveBeenCalled();
  });

  it("does not call launch when prompt is empty", async () => {
    const promptRef = { current: { value: "   " } as unknown as HTMLTextAreaElement };
    const mockSetActiveSession = vi.fn();
    const mockActivateSession = vi.fn();
    const mockSetIsCreating = vi.fn();
    const mockOnClose = vi.fn();

    const { result } = renderHook(() =>
      useSessionLaunchSubmit({
        promptRef,
        taskId: TASK_ID,
        selectedProfileId: PROFILE_ID,
        executorId: EXECUTOR_ID,
        contextValue: "blank",
        initialPrompt: null,
        agentProfiles: [AGENT_PROFILE_A],
        attachments: [ATTACHMENT],
        onClose: mockOnClose,
        toast: mockToast,
        setActiveSession: mockSetActiveSession,
        activateSession: mockActivateSession,
        setIsCreating: mockSetIsCreating,
      }),
    );
    const event = {
      preventDefault: vi.fn(),
    } as unknown as FormEvent;

    await act(async () => {
      await result.current(event);
    });

    expect(mockBuildStartRequest).not.toHaveBeenCalled();
    expect(mockLaunchSession).not.toHaveBeenCalled();
    expect(mockSetIsCreating).not.toHaveBeenCalled();
    expect(mockOnClose).not.toHaveBeenCalled();
  });

  it("shows a toast when launching fails", async () => {
    mockLaunchSession.mockRejectedValueOnce(new Error("launch failed"));
    const promptRef = { current: { value: "hello" } as unknown as HTMLTextAreaElement };
    const mockSetActiveSession = vi.fn();
    const mockActivateSession = vi.fn();
    const mockSetIsCreating = vi.fn();
    const mockOnClose = vi.fn();

    const { result } = renderHook(() =>
      useSessionLaunchSubmit({
        promptRef,
        taskId: TASK_ID,
        selectedProfileId: PROFILE_ID,
        executorId: EXECUTOR_ID,
        contextValue: "blank",
        initialPrompt: null,
        agentProfiles: [AGENT_PROFILE_A],
        attachments: [ATTACHMENT],
        onClose: mockOnClose,
        toast: mockToast,
        setActiveSession: mockSetActiveSession,
        activateSession: mockActivateSession,
        setIsCreating: mockSetIsCreating,
      }),
    );
    const event = {
      preventDefault: vi.fn(),
    } as unknown as FormEvent;

    await act(async () => {
      await result.current(event);
    });

    expect(mockToast).toHaveBeenCalledWith({
      title: "Failed to create session",
      description: "launch failed",
      variant: "error",
    });
    expect(mockActivateSession).not.toHaveBeenCalled();
    expect(mockSetIsCreating).toHaveBeenLastCalledWith(false);
  });
});
