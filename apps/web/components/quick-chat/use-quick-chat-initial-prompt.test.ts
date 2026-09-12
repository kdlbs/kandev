import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { getChatDraftText, setChatDraftText } from "@/lib/local-storage";
import type {
  ChatSubmitPayload,
  ChatSubmitResult,
} from "@/components/task/chat/chat-input-container";
import { useQuickChatInitialPrompt } from "./use-quick-chat-initial-prompt";

const LAUNCH_PROMPT = "Start here";

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});

describe("useQuickChatInitialPrompt draft recovery", () => {
  it.each([false, true])(
    "keeps a rejected launch as a manual draft without resending on remount (throws=%s)",
    async (throws) => {
      let pending: string | undefined = LAUNCH_PROMPT;
      const submit = throws
        ? vi.fn().mockRejectedValue(new Error("connection lost"))
        : vi.fn().mockResolvedValue(false);
      const onAttempted = () => {
        pending = undefined;
      };
      const onRejected = vi.fn();
      const mount = () =>
        renderHook(() =>
          useQuickChatInitialPrompt({
            sessionId: "session-1",
            taskId: "task-1",
            prompt: pending,
            blocked: false,
            submit,
            onAttempted,
            onRejected,
          }),
        );
      const first = mount();
      await act(async () => {});
      first.unmount();
      const second = mount();
      await act(async () => {});

      expect(submit).toHaveBeenCalledTimes(1);
      expect(getChatDraftText("session-1")).toBe(LAUNCH_PROMPT);
      expect(onRejected).toHaveBeenCalledWith("session-1", LAUNCH_PROMPT);
      second.unmount();
    },
  );

  it("preserves a newer manual draft when automatic delivery settles", async () => {
    let accept!: (value: boolean) => void;
    const submit = vi.fn(
      () =>
        new Promise<boolean>((resolve) => {
          accept = resolve;
        }),
    );
    renderHook(() =>
      useQuickChatInitialPrompt({
        sessionId: "session-1",
        taskId: "task-1",
        prompt: LAUNCH_PROMPT,
        blocked: false,
        submit,
      }),
    );
    await act(async () => {});
    const recoveryDraft = getChatDraftText("session-1");
    setChatDraftText("session-1", "manual follow-up");
    await act(async () => accept(true));
    expect(recoveryDraft).toBe(LAUNCH_PROMPT);
    expect(getChatDraftText("session-1")).toBe("manual follow-up");
  });

  it("keeps an attempted prompt bound to its original session submitter", async () => {
    const firstSubmit = vi.fn().mockResolvedValue(true);
    const nextSubmit = vi.fn().mockResolvedValue(true);
    const view = renderHook(
      ({ sessionId, submit, blocked }) =>
        useQuickChatInitialPrompt({
          sessionId,
          taskId: "task-1",
          prompt: LAUNCH_PROMPT,
          submit,
          blocked,
        }),
      { initialProps: { sessionId: "session-1", submit: firstSubmit, blocked: false } },
    );
    view.rerender({ sessionId: "session-2", submit: nextSubmit, blocked: true });
    await act(async () => {});
    expect(firstSubmit).toHaveBeenCalledOnce();
    expect(nextSubmit).not.toHaveBeenCalled();
  });
});

describe("useQuickChatInitialPrompt admission", () => {
  it("waits for migration and clears the launch prompt only after acceptance", async () => {
    const submit = vi.fn().mockResolvedValue(true);
    const onAccepted = vi.fn();
    const view = renderHook(
      ({ blocked }) =>
        useQuickChatInitialPrompt({
          sessionId: "session-1",
          taskId: "task-1",
          prompt: LAUNCH_PROMPT,
          blocked,
          submit,
          onAccepted,
        }),
      { initialProps: { blocked: true } },
    );

    expect(submit).not.toHaveBeenCalled();
    view.rerender({ blocked: false });
    await act(async () => {});

    expect(submit).toHaveBeenCalledWith({ message: LAUNCH_PROMPT });
    expect(onAccepted).toHaveBeenCalledTimes(1);
  });

  it("preserves the launch prompt when delivery is rejected", async () => {
    const submit = vi.fn().mockResolvedValue(false);
    const onAccepted = vi.fn();
    renderHook(() =>
      useQuickChatInitialPrompt({
        sessionId: "session-1",
        taskId: "task-1",
        prompt: LAUNCH_PROMPT,
        blocked: false,
        submit,
        onAccepted,
      }),
    );
    await act(async () => {});

    expect(onAccepted).not.toHaveBeenCalled();
  });

  it("does not retry a rejected prompt when callback identities change", async () => {
    const firstSubmit = vi.fn().mockResolvedValue(false);
    const secondSubmit = vi.fn().mockResolvedValue(false);
    const view = renderHook(
      ({ submit }: { submit: (payload: ChatSubmitPayload) => ChatSubmitResult }) =>
        useQuickChatInitialPrompt({
          sessionId: "session-1",
          taskId: "task-1",
          prompt: LAUNCH_PROMPT,
          blocked: false,
          submit,
        }),
      { initialProps: { submit: firstSubmit } },
    );
    await act(async () => {});

    view.rerender({ submit: secondSubmit });
    await act(async () => {});

    expect(firstSubmit).toHaveBeenCalledTimes(1);
    expect(secondSubmit).not.toHaveBeenCalled();
  });

  it("does not retry a synchronously rejected prompt", async () => {
    const firstSubmit: (payload: ChatSubmitPayload) => ChatSubmitResult = vi.fn(
      (_payload: ChatSubmitPayload) => {
        throw new Error("rejected before returning a promise");
      },
    );
    const secondSubmit = vi.fn().mockResolvedValue(false);
    const view = renderHook(
      ({ submit }: { submit: (payload: ChatSubmitPayload) => ChatSubmitResult }) =>
        useQuickChatInitialPrompt({
          sessionId: "session-1",
          taskId: "task-1",
          prompt: LAUNCH_PROMPT,
          blocked: false,
          submit,
        }),
      { initialProps: { submit: firstSubmit } },
    );
    await act(async () => {});

    view.rerender({ submit: secondSubmit });
    await act(async () => {});

    expect(firstSubmit).toHaveBeenCalledTimes(1);
    expect(secondSubmit).not.toHaveBeenCalled();
  });
});
