import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type {
  ChatSubmitPayload,
  ChatSubmitResult,
} from "@/components/task/chat/chat-input-container";
import { useQuickChatInitialPrompt } from "./use-quick-chat-initial-prompt";

const LAUNCH_PROMPT = "Start here";

describe("useQuickChatInitialPrompt", () => {
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
